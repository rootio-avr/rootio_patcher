package apk

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"rootio_patcher/pkg/rootio"
)

// APIClient is the interface for calling the Root.io OS package analysis endpoint
type APIClient interface {
	AnalyzeOsPackages(ctx context.Context, endpoint, ecosystem, distroVersion string, packages []rootio.Package) (*rootio.OsAnalyzeResponse, error)
}

// App orchestrates the apk remediate workflow
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

// Run executes the apk remediation workflow
func (a *App) Run(ctx context.Context) error {
	a.logger.DebugContext(ctx, "Starting apk remediation", slog.Bool("dry_run", a.dryRun))

	// 1. Detect OS
	osInfo, err := a.scanner.DetectOS(ctx)
	if err != nil {
		return fmt.Errorf("OS detection failed: %w", err)
	}
	a.logger.DebugContext(ctx, "Detected OS", slog.String("distro_version", osInfo.DistroVersion))

	// 2. Scan installed packages
	installed, err := a.scanner.ListPackages(ctx)
	if err != nil {
		return fmt.Errorf("package scan failed: %w", err)
	}
	a.logger.DebugContext(ctx, "Scanned installed packages", slog.Int("count", len(installed)))

	if len(installed) == 0 {
		fmt.Println("No packages found via apk info -v")
		return nil
	}

	// 3. Call /v3/analyze/apk
	pkgs := make([]rootio.Package, len(installed))
	for i, p := range installed {
		pkgs[i] = rootio.Package{Name: p.Name, Version: p.Version}
	}

	a.logger.DebugContext(ctx, "Calling analyze API")
	response, err := a.apiClient.AnalyzeOsPackages(ctx, "apk", "alpine", osInfo.DistroVersion, pkgs)
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
		a.reportDryRun(response)
		return nil
	}

	// 6. Execute remediation
	hasPatches := len(response.Patches) > 0

	if hasPatches {
		fmt.Println("\nSetting up Root.io APK repository...")
		if err := a.executor.Setup(ctx, osInfo); err != nil {
			return fmt.Errorf("repository setup failed: %w", err)
		}
	} else {
		// Only upgrades — still need apk update
		fmt.Println("\nUpdating package index...")
		if err := a.executor.ApkUpdate(ctx); err != nil {
			return fmt.Errorf("apk update failed: %w", err)
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
		if err := a.executor.InstallPatches(ctx, response.Patches); err != nil {
			return fmt.Errorf("patches failed: %w", err)
		}
	}

	if hasPatches {
		fmt.Println("\nCleaning up Root.io APK repository...")
		if err := a.executor.Cleanup(ctx); err != nil {
			return fmt.Errorf("cleanup failed: %w", err)
		}
	}

	fmt.Println("\n✓ apk remediation complete")
	return nil
}

func (a *App) reportDryRun(response *rootio.OsAnalyzeResponse) {
	fmt.Println("\n=== DRY-RUN MODE ===")

	if len(response.Upgradeable) > 0 {
		fmt.Printf("\n%d package(s) upgradeable via official repo:\n", len(response.Upgradeable))
		for _, u := range response.Upgradeable {
			cves := ""
			if len(u.CVEIDs) > 0 {
				cves = " (fixes: " + strings.Join(u.CVEIDs, ", ") + ")"
			}
			fmt.Printf("  • %s %s → %s%s\n", u.PackageName, u.CurrentVersion, u.UpgradeVersion, cves)
		}
	}

	if len(response.Patches) > 0 {
		fmt.Printf("\n%d package(s) patchable via Root.io alias:\n", len(response.Patches))
		for _, p := range response.Patches {
			cves := ""
			if len(p.CVEIDs) > 0 {
				cves = " (fixes: " + strings.Join(p.CVEIDs, ", ") + ")"
			}
			fmt.Printf("  • %s %s → %s %s%s\n",
				p.PackageName, p.Version,
				p.PatchAlias.Name, p.PatchAlias.Version,
				cves)
		}
	}

	fmt.Println("\nTo apply: rootio_patcher apk remediate --dry-run=false")
}
