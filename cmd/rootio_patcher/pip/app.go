package pip

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/cmd/rootio_patcher/config"
	"rootio_patcher/pkg/rootio"
)

// App handles pip package remediation (post-install patching)
type App struct {
	cfg        *config.Config
	pythonPath string
	dryRun     bool
	useAlias   bool
	logger     *slog.Logger

	pipService Service
	apiClient  common.APIClient
	reporter   *common.Reporter
}

// NewApp creates a new pip application instance
func NewApp(cfg *config.Config, pythonPath string, dryRun, useAlias bool, logger *slog.Logger) *App {
	pipService := NewService(pythonPath, cfg.PKGURL, cfg.APIKey, useAlias, logger)
	apiClient := rootio.NewClient(cfg.APIURL, cfg.APIKey)
	reporter := common.NewReporter(cfg.PKGURL, logger)

	return NewAppWithServices(cfg, pythonPath, dryRun, useAlias, logger, pipService, apiClient, reporter)
}

// NewAppWithServices creates a new pip application with injected services (for testing)
func NewAppWithServices(
	cfg *config.Config,
	pythonPath string,
	dryRun, useAlias bool,
	logger *slog.Logger,
	pipService Service,
	apiClient common.APIClient,
	reporter *common.Reporter,
) *App {
	return &App{
		cfg:        cfg,
		pythonPath: pythonPath,
		dryRun:     dryRun,
		useAlias:   useAlias,
		logger:     logger,
		pipService: pipService,
		apiClient:  apiClient,
		reporter:   reporter,
	}
}

// Run executes the pip remediation workflow
func (a *App) Run(ctx context.Context) error {
	a.logger.DebugContext(ctx, "Starting pip remediation", slog.Bool("dry_run", a.dryRun))

	// 1. Collect installed packages
	a.logger.DebugContext(ctx, "Collecting installed packages")
	packages, err := a.pipService.ListPackages(ctx)
	if err != nil {
		return fmt.Errorf("failed to collect packages: %w", err)
	}
	a.logger.DebugContext(ctx, "Collected packages", slog.Int("count", len(packages)))

	// 2. Convert to SDK format
	sdkPackages := make([]rootio.Package, len(packages))
	for i, pkg := range packages {
		sdkPackages[i] = rootio.Package{
			Name:    pkg.Name,
			Version: pkg.Version,
		}
	}

	// 3. Call backend API to analyze vulnerabilities
	a.logger.DebugContext(ctx, "Analyzing packages for vulnerabilities")
	response, err := a.apiClient.AnalyzePackages(ctx, sdkPackages)
	if err != nil {
		return fmt.Errorf("failed to analyze packages: %w", err)
	}

	// 4. Log analysis results
	a.logger.DebugContext(ctx, "Vulnerability analysis complete",
		slog.Int("patches_available", len(response.Patches)),
		slog.Int("packages_skipped", len(response.Skipped)))

	if len(response.Patches) == 0 {
		fmt.Println("\nNo patches needed - all packages are up to date!")
		return nil
	}

	// 5. Execute or dry-run patches
	if a.dryRun {
		a.logger.DebugContext(ctx, "DRY-RUN MODE: No changes will be made")
		a.reporter.ReportDryRun(response.Patches, a.useAlias)
		return nil
	}

	// 6. Execute patches
	fmt.Printf("\nApplying %d patches...\n\n", len(response.Patches))
	if err := a.applyPatches(ctx, response.Patches); err != nil {
		return err
	}

	fmt.Printf("\n✓ Successfully patched %d packages!\n", len(response.Patches))

	return nil
}

// applyPatches applies patches sequentially, exits on first failure
func (a *App) applyPatches(ctx context.Context, patches []rootio.PackagePatch) error {
	for i, patch := range patches {
		// Select patch info based on config
		var patchName, patchVersion string
		if a.useAlias {
			patchName = patch.PatchAlias.Name
			patchVersion = patch.PatchAlias.Version
		} else {
			patchName = patch.Patch.Name
			patchVersion = patch.Patch.Version
		}

		fmt.Printf("[%d/%d] Patching %s (%s → %s)...\n",
			i+1, len(patches),
			patch.PackageName,
			patch.Version,
			patchVersion)

		a.logger.DebugContext(ctx, "Patch details",
			slog.String("patch_name", patchName),
			slog.Bool("use_alias", a.useAlias))

		// Use special handling for pip package - upgrade instead of uninstall+install
		var err error
		if strings.EqualFold(patch.PackageName, "pip") {
			err = a.pipService.ApplyPatchForPip(ctx, patch)
		} else {
			err = a.pipService.ApplyPatch(ctx, patch)
		}

		if err != nil {
			fmt.Printf("✗ Patch failed: %v\n", err)
			return fmt.Errorf("patch failed: %w", err)
		}

		fmt.Printf("  ✓ Successfully patched %s\n\n", patch.PackageName)
	}

	return nil
}
