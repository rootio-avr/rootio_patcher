package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"rootio_patcher/pkg/rootio"
)

// App is the main application for the PyPI patcher
type App struct {
	cfg    *Config
	logger *slog.Logger

	collector   *PackageCollector
	apiClient   *rootio.Client
	pipExecutor *PipExecutor
	reporter    *Reporter
}

// NewApp creates a new application instance
func NewApp(cfg *Config, logger *slog.Logger) *App {
	collector := NewPackageCollector(cfg.PythonPath, logger)
	apiClient := rootio.NewClient(cfg.APIURL, cfg.APIKey)
	pipExecutor := NewPipExecutor(cfg.PythonPath, cfg.PKGURL, cfg.APIKey, cfg.UseAlias, logger)
	reporter := NewReporter(cfg.PKGURL, logger)

	return &App{
		cfg:         cfg,
		logger:      logger,
		collector:   collector,
		apiClient:   apiClient,
		pipExecutor: pipExecutor,
		reporter:    reporter,
	}
}

// Run executes the main application workflow
func (a *App) Run(ctx context.Context) error {
	a.logger.DebugContext(ctx, "Starting PyPI Patcher", slog.Bool("dry_run", a.cfg.DryRun))

	// 1. Collect installed packages
	a.logger.DebugContext(ctx, "Collecting installed packages")
	packages, err := a.collector.CollectPackages(ctx)
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
	if a.cfg.DryRun {
		a.logger.DebugContext(ctx, "DRY-RUN MODE: No changes will be made")
		a.reporter.ReportDryRun(response.Patches, a.cfg.UseAlias)
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
		if a.cfg.UseAlias {
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
			slog.Bool("use_alias", a.cfg.UseAlias))

		// Use special handling for pip package - upgrade instead of uninstall+install
		var err error
		if strings.EqualFold(patch.PackageName, "pip") {
			err = a.pipExecutor.ApplyPatchForPip(ctx, patch)
		} else {
			err = a.pipExecutor.ApplyPatch(ctx, patch)
		}

		if err != nil {
			fmt.Printf("✗ Patch failed: %v\n", err)
			return fmt.Errorf("patch failed: %w", err)
		}

		fmt.Printf("  ✓ Successfully patched %s\n\n", patch.PackageName)
	}

	return nil
}
