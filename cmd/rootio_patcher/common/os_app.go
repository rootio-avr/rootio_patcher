package common

import (
	"context"
	"fmt"
	"log/slog"

	"rootio_patcher/pkg/rootio"
)

// OsExecutor performs the OS-level remediation steps
type OsExecutor interface {
	// Setup installs the Root.io repository and public key, then runs an index update.
	// registryURL is the base repo URL without credentials.
	Setup(ctx context.Context, registryURL string) error
	// IndexUpdate refreshes the package index (apt-get update / apk update)
	IndexUpdate(ctx context.Context) error
	// InstallUpgrades installs packages available from the official repository
	InstallUpgrades(ctx context.Context, upgradeable []rootio.UpgradeableOsPackage) error
	// InstallPatches installs Root.io alias packages.
	// registryURL is provided for executors that need it for pinning (e.g. apt); others ignore it.
	InstallPatches(ctx context.Context, registryURL string, patches []rootio.PackagePatch) error
	// Cleanup removes the Root.io repository and any related files
	Cleanup(ctx context.Context) error
	// PostUpgradesOnly is called after upgrades when no patches were applied.
	// Executors that need post-upgrade cleanup (e.g. clearing apt caches) implement it here.
	PostUpgradesOnly(ctx context.Context) error
}

// OsAppConfig holds the OS-specific parameters for OsApp
type OsAppConfig[T any] struct {
	// Name is the package manager name, used in log messages (e.g. "apt", "apk")
	Name string
	// NoPackagesMsg is printed when the installed package list is empty
	NoPackagesMsg string
	// UpdateMsg is printed before running an index update in the upgrades-only path
	UpdateMsg string
	// UpdateErrMsg is the error prefix if the index update fails
	UpdateErrMsg string
	// SetupMsg is printed before setting up the Root.io repository
	SetupMsg string
	// CleanupMsg is printed before cleanup
	CleanupMsg string
	// GetAPIParams extracts the endpoint, ecosystem, and distroVersion to send to the API
	GetAPIParams func(*T) (endpoint, ecosystem, distroVersion string)
	// GetRegistryURL builds the Root.io repository URL from the base pkgURL and OS info
	GetRegistryURL func(pkgURL string, osInfo *T) string
	// LogOsInfo emits OS-specific debug fields after OS detection
	LogOsInfo func(ctx context.Context, logger *slog.Logger, osInfo *T)
}

// OsApp orchestrates the OS-level remediation workflow
type OsApp[T any] struct {
	pkgURL    string
	dryRun    bool
	logger    *slog.Logger
	scanner   Scanner[T]
	apiClient OsAPIClient
	executor  OsExecutor
	config    OsAppConfig[T]
}

func NewOsApp[T any](
	pkgURL string,
	dryRun bool,
	logger *slog.Logger,
	scanner Scanner[T],
	apiClient OsAPIClient,
	executor OsExecutor,
	config OsAppConfig[T],
) *OsApp[T] {
	return &OsApp[T]{
		pkgURL:    pkgURL,
		dryRun:    dryRun,
		logger:    logger,
		scanner:   scanner,
		apiClient: apiClient,
		executor:  executor,
		config:    config,
	}
}

// Run executes the OS remediation workflow
func (a *OsApp[T]) Run(ctx context.Context) error {
	cfg := a.config
	a.logger.DebugContext(ctx, "Starting "+cfg.Name+" remediation", slog.Bool("dry_run", a.dryRun))

	// 1. Detect OS
	osInfo, err := a.scanner.DetectOS(ctx)
	if err != nil {
		return fmt.Errorf("OS detection failed: %w", err)
	}
	cfg.LogOsInfo(ctx, a.logger, osInfo)

	// 2. Scan installed packages
	installed, err := a.scanner.ListPackages(ctx)
	if err != nil {
		return fmt.Errorf("package scan failed: %w", err)
	}
	a.logger.DebugContext(ctx, "Scanned installed packages", slog.Int("count", len(installed)))

	if len(installed) == 0 {
		fmt.Println(cfg.NoPackagesMsg)
		return nil
	}

	// 3. Call analyze API
	pkgs := make([]rootio.Package, len(installed))
	for i, p := range installed {
		pkgs[i] = rootio.Package{Name: p.Name, Version: p.Version}
	}

	endpoint, ecosystem, distroVersion := cfg.GetAPIParams(osInfo)
	a.logger.DebugContext(ctx, "Calling analyze API")
	response, err := a.apiClient.AnalyzeOsPackages(ctx, endpoint, ecosystem, distroVersion, pkgs)
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
		ReportOsDryRun(response, "rootio_patcher "+cfg.Name+" remediate --dry-run=false")
		return nil
	}

	// 6. Execute remediation
	registryURL := cfg.GetRegistryURL(a.pkgURL, osInfo)
	hasPatches := len(response.Patches) > 0

	if hasPatches {
		fmt.Println(cfg.SetupMsg)
		if err := a.executor.Setup(ctx, registryURL); err != nil {
			return fmt.Errorf("repository setup failed: %w", err)
		}
	} else {
		fmt.Println(cfg.UpdateMsg)
		if err := a.executor.IndexUpdate(ctx); err != nil {
			return fmt.Errorf(cfg.UpdateErrMsg+": %w", err)
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
		fmt.Println(cfg.CleanupMsg)
		if err := a.executor.Cleanup(ctx); err != nil {
			return fmt.Errorf("cleanup failed: %w", err)
		}
	} else {
		if err := a.executor.PostUpgradesOnly(ctx); err != nil {
			return fmt.Errorf("post-upgrade cleanup failed: %w", err)
		}
	}

	fmt.Println("\n✓ " + cfg.Name + " remediation complete")
	return nil
}
