package apt

import (
	"context"
	"fmt"
	"log/slog"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/pkg/rootio"
)

// APIClient is an alias for common.OsAPIClient
type APIClient = common.OsAPIClient

// App orchestrates the apt remediate workflow
type App struct {
	apiKey  string
	pkgURL  string
	dryRun  bool
	verbose bool
	logger  *slog.Logger

	scanner   Scanner
	apiClient APIClient
	executor  *Executor
}

func NewApp(apiKey, apiURL, pkgURL string, dryRun, verbose bool, logger *slog.Logger) *App {
	client := rootio.NewClient(apiURL, apiKey)
	executor := NewExecutor(apiKey, pkgURL, verbose, NewRealRunner())
	return NewAppWithServices(apiKey, pkgURL, dryRun, verbose, logger, NewScanner(), client, executor)
}

func NewAppWithServices(
	apiKey, pkgURL string,
	dryRun, verbose bool,
	logger *slog.Logger,
	scanner Scanner,
	apiClient APIClient,
	executor *Executor,
) *App {
	return &App{
		apiKey:    apiKey,
		pkgURL:    pkgURL,
		dryRun:    dryRun,
		verbose:   verbose,
		logger:    logger,
		scanner:   scanner,
		apiClient: apiClient,
		executor:  executor,
	}
}

// Run executes the apt remediation workflow
func (a *App) Run(ctx context.Context) error {
	a.logger.DebugContext(ctx, "Starting apt remediation", slog.Bool("dry_run", a.dryRun))

	// 1. Detect OS
	osInfo, err := a.scanner.DetectOS(ctx)
	if err != nil {
		return fmt.Errorf("OS detection failed: %w", err)
	}
	a.logger.DebugContext(ctx, "Detected OS",
		slog.String("ecosystem", osInfo.Ecosystem),
		slog.String("distro_version", osInfo.DistroVersion),
		slog.String("codename", osInfo.Codename),
	)

	// 2. Scan installed packages
	installed, err := a.scanner.ListPackages(ctx)
	if err != nil {
		return fmt.Errorf("package scan failed: %w", err)
	}
	a.logger.DebugContext(ctx, "Scanned installed packages", slog.Int("count", len(installed)))

	if len(installed) == 0 {
		fmt.Println("No packages found via dpkg-query")
		return nil
	}

	// 3. Call /v3/analyze/apt
	pkgs := make([]rootio.Package, len(installed))
	for i, p := range installed {
		pkgs[i] = rootio.Package{Name: p.Name, Version: p.Version}
	}

	a.logger.DebugContext(ctx, "Calling analyze API")
	response, err := a.apiClient.AnalyzeOsPackages(ctx, "apt", osInfo.Ecosystem, osInfo.DistroVersion, pkgs)
	if err != nil {
		return fmt.Errorf("API call failed: %w", err)
	}

	a.logger.DebugContext(ctx, "Analysis complete",
		slog.Int("patches", len(response.Patches)),
		slog.Int("upgradeable", len(response.Upgradeable)),
		slog.Int("skipped", len(response.Skipped)),
	)

	// 4. Nothing to do?
	if len(response.Patches) == 0 && len(response.Upgradeable) == 0 {
		fmt.Println("\nNo patches needed — all packages are up to date!")
		return nil
	}

	// 5. Dry-run: print what would be done
	if a.dryRun {
		common.ReportOsDryRun(response, "rootio_patcher apt remediate --dry-run=false")
		return nil
	}

	// 6. Execute remediation
	registryURL := fmt.Sprintf("%s/%s/%s", a.pkgURL, osInfo.Ecosystem, osInfo.Codename)
	hasPatches := len(response.Patches) > 0

	if hasPatches {
		fmt.Println("\nSetting up Root.io APT repository...")
		if err := a.executor.Setup(ctx, osInfo); err != nil {
			return fmt.Errorf("repository setup failed: %w", err)
		}
	} else {
		// Only upgrades — still need apt-get update
		fmt.Println("\nUpdating package lists...")
		if err := a.executor.AptUpdate(ctx); err != nil {
			return fmt.Errorf("apt-get update failed: %w", err)
		}
	}

	if len(response.Upgradeable) > 0 {
		fmt.Printf("\nInstalling %d upgrade(s)...\n", len(response.Upgradeable))
		if err := a.executor.InstallUpgrades(ctx, response.Upgradeable); err != nil {
			return fmt.Errorf("upgrades failed: %w", err)
		}
	}

	if len(response.Patches) > 0 {
		fmt.Printf("\nInstalling %d Root.io patch(es)...\n", len(response.Patches))
		if err := a.executor.InstallPatches(ctx, registryURL, response.Patches); err != nil {
			return fmt.Errorf("patches failed: %w", err)
		}
	}

	if hasPatches {
		fmt.Println("\nCleaning up Root.io APT repository...")
		if err := a.executor.Cleanup(ctx); err != nil {
			return fmt.Errorf("cleanup failed: %w", err)
		}
	} else {
		// No Root.io repo was added, just clear apt caches
		_ = a.executor.ClearAptCaches(ctx)
	}

	fmt.Println("\n✓ apt remediation complete")
	return nil
}
