package common

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"rootio_patcher/pkg/rootio"
)

// OsExecutor performs the OS-level remediation steps
type OsExecutor interface {
	// Setup installs the Root.io repository and public key, then runs an index update.
	// registryURL is the base repo URL without credentials.
	Setup(ctx context.Context, registryURL string) error
	// IndexUpdate refreshes the package index (apt-get update / apk update)
	IndexUpdate(ctx context.Context) error
	// InstallUpgrades installs the named packages from the official distro repository.
	InstallUpgrades(ctx context.Context, names []string) error
	// InstallPatches installs Root.io packages.
	// registryURL is provided for executors that need it for pinning (e.g. apt); others ignore it.
	// useAlias controls whether the aliased (rootio-*) or original package name is installed.
	InstallPatches(ctx context.Context, registryURL string, patches []rootio.PackagePatch, useAlias bool) error
	// RemoveRootioRepo removes the Root.io repository (and apt pin) and refreshes the index,
	// so subsequent upgrades resolve only against the official distro repo. Called exactly
	// once, after patches are installed.
	RemoveRootioRepo(ctx context.Context) error
	// Cleanup removes the remaining Root.io files (keys, auth, caches)
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
	// PackageBlacklist is a per-ecosystem set of package names never offered for upgrade
	PackageBlacklist map[string]bool
}

// OsApp orchestrates the OS-level remediation workflow
type OsApp[T any] struct {
	pkgURL       string
	dryRun       bool
	useAlias     bool
	skipUpgrades bool
	ignoreSet    map[string]struct{}
	logger       *slog.Logger
	scanner      Scanner[T]
	apiClient    OsAPIClient
	executor     OsExecutor
	config       OsAppConfig[T]
}

func NewOsApp[T any](
	pkgURL string,
	dryRun bool,
	useAlias bool,
	skipUpgrades bool,
	ignoreSet map[string]struct{},
	logger *slog.Logger,
	scanner Scanner[T],
	apiClient OsAPIClient,
	executor OsExecutor,
	config OsAppConfig[T],
) *OsApp[T] {
	return &OsApp[T]{
		pkgURL:       pkgURL,
		dryRun:       dryRun,
		useAlias:     useAlias,
		skipUpgrades: skipUpgrades,
		ignoreSet:    ignoreSet,
		logger:       logger,
		scanner:      scanner,
		apiClient:    apiClient,
		executor:     executor,
		config:       config,
	}
}

// Run executes the OS remediation workflow
func (a *OsApp[T]) Run(ctx context.Context) error {
	cfg := a.config
	a.logger.DebugContext(ctx, "Starting "+cfg.Name+" remediation", slog.Bool("dry_run", a.dryRun), slog.Bool("use_alias", a.useAlias))

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

	// 4. Compute the work set.
	//    Patches are filtered by the ignore list; the broad upgrade set is
	//    computed client-side as installed − patched − blacklist − ignored.
	patches := filterPatches(response.Patches, a.ignoreSet)
	var upgradeNames []string
	if !a.skipUpgrades {
		upgradeNames = computeUpgradeSet(installed, patches, a.config.PackageBlacklist, a.ignoreSet)
	}
	hasPatches := len(patches) > 0

	// 5. Nothing to do?
	if !hasPatches && len(upgradeNames) == 0 {
		fmt.Println("\nNo patches needed — all packages are up to date!")
		return nil
	}

	// 6. Dry-run: print what would be done
	if a.dryRun {
		ReportOsDryRun(response, patches, upgradeNames, "rootio_patcher "+cfg.Name+" remediate --dry-run=false", a.useAlias)
		return nil
	}

	// 7. Execute remediation
	if hasPatches {
		registryURL := cfg.GetRegistryURL(a.pkgURL, osInfo)

		fmt.Println(cfg.SetupMsg)
		if err := a.executor.Setup(ctx, registryURL); err != nil {
			return fmt.Errorf("repository setup failed: %w", err)
		}

		fmt.Printf("\nInstalling %d Root.io patch(es)...\n", len(patches))
		if err := a.executor.InstallPatches(ctx, registryURL, patches, a.useAlias); err != nil {
			return fmt.Errorf("patches failed: %w", err)
		}

		// Remove the Root.io repo+pin BEFORE upgrades so the broad upgrade
		// resolves only against the official distro repo.
		if err := a.executor.RemoveRootioRepo(ctx); err != nil {
			return fmt.Errorf("remove Root.io repository failed: %w", err)
		}

		if len(upgradeNames) > 0 {
			fmt.Printf("\nInstalling %d upgrade(s)...\n", len(upgradeNames))
			if err := a.executor.InstallUpgrades(ctx, upgradeNames); err != nil {
				return fmt.Errorf("upgrades failed: %w", err)
			}
		}

		fmt.Println(cfg.CleanupMsg)
		if err := a.executor.Cleanup(ctx); err != nil {
			return fmt.Errorf("cleanup failed: %w", err)
		}
	} else {
		fmt.Println(cfg.UpdateMsg)
		if err := a.executor.IndexUpdate(ctx); err != nil {
			return fmt.Errorf(cfg.UpdateErrMsg+": %w", err)
		}

		if len(upgradeNames) > 0 {
			fmt.Printf("\nInstalling %d upgrade(s)...\n", len(upgradeNames))
			if err := a.executor.InstallUpgrades(ctx, upgradeNames); err != nil {
				return fmt.Errorf("upgrades failed: %w", err)
			}
		}

		if err := a.executor.PostUpgradesOnly(ctx); err != nil {
			return fmt.Errorf("post-upgrade cleanup failed: %w", err)
		}
	}

	fmt.Println("\n✓ " + cfg.Name + " remediation complete")
	return nil
}

// ignoreNamesFromSet extracts the package-name portion from each ignoreSet key.
// Keys are "name@version"; the name is the substring before the LAST "@".
// If there is no "@", the whole key is treated as the package name.
//
// This DIVERGES from IgnoreListToPackages (ignore.go) intentionally: the OS
// upgrade path ignores strictly by name, so a bare key with no version
// (e.g. "nginx") means "never touch nginx" and is honored here.
// IgnoreListToPackages, by contrast, skips bare keys because the API only
// accepts name@version pairs.
func ignoreNamesFromSet(ignoreSet map[string]struct{}) map[string]struct{} {
	names := make(map[string]struct{}, len(ignoreSet))
	for key := range ignoreSet {
		at := strings.LastIndex(key, "@")
		if at >= 0 {
			names[key[:at]] = struct{}{}
		} else {
			names[key] = struct{}{}
		}
	}
	return names
}

// computeUpgradeSet returns the names of installed packages that should be
// offered for a broad OS upgrade: those NOT already covered by a Root.io patch,
// NOT in the blacklist, and NOT ignored.
// The result is de-duplicated and sorted ascending for deterministic output.
func computeUpgradeSet(installed []InstalledPackage, patches []rootio.PackagePatch, blacklist map[string]bool, ignoreSet map[string]struct{}) []string {
	patched := make(map[string]struct{}, len(patches))
	for _, p := range patches {
		patched[p.PackageName] = struct{}{}
	}

	ignored := ignoreNamesFromSet(ignoreSet)

	seen := make(map[string]struct{})
	for _, pkg := range installed {
		name := pkg.Name
		if _, isPatch := patched[name]; isPatch {
			continue
		}
		if blacklist[name] {
			continue
		}
		if _, isIgnored := ignored[name]; isIgnored {
			continue
		}
		seen[name] = struct{}{}
	}

	result := make([]string, 0, len(seen))
	for name := range seen {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

// filterPatches drops any patch whose PackageName is in the ignored-names set
// (derived from ignoreSet using the same "name@version" / bare-name rules).
// The remaining patches are returned in input order.
//
// When nothing is filtered (empty patches or empty ignoreSet), the caller's
// slice is returned unchanged rather than copied; callers must not mutate the
// returned slice's elements expecting an independent copy.
func filterPatches(patches []rootio.PackagePatch, ignoreSet map[string]struct{}) []rootio.PackagePatch {
	if len(patches) == 0 || len(ignoreSet) == 0 {
		return patches
	}

	ignored := ignoreNamesFromSet(ignoreSet)

	result := make([]rootio.PackagePatch, 0, len(patches))
	for _, p := range patches {
		if _, isIgnored := ignored[p.PackageName]; isIgnored {
			continue
		}
		result = append(result, p)
	}
	return result
}
