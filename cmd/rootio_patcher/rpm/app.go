package rpm

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
)

// App orchestrates the scan-and-upgrade-all workflow for RPM-based package
// managers (yum, dnf, microdnf). Root.io has no targeted patches for these
// ecosystems, so unlike apt/apk this never calls the analyze API or installs
// Root.io patches — it only scans installed packages and upgrades them to
// the latest version available from the configured repositories.
type App struct {
	manager   Manager
	dryRun    bool
	ignoreSet map[string]struct{}
	logger    *slog.Logger
	scanner   Scanner
	executor  *Executor
}

func NewApp(manager Manager, dryRun bool, ignoreSet map[string]struct{}, logger *slog.Logger) *App {
	runner := NewRealRunner()
	return NewAppWithServices(manager, dryRun, ignoreSet, logger, NewScanner(), NewExecutor(manager, logger, runner))
}

func NewAppWithServices(manager Manager, dryRun bool, ignoreSet map[string]struct{}, logger *slog.Logger, scanner Scanner, executor *Executor) *App {
	return &App{manager: manager, dryRun: dryRun, ignoreSet: ignoreSet, logger: logger, scanner: scanner, executor: executor}
}

func (a *App) Run(ctx context.Context) error {
	a.logger.DebugContext(ctx, "Starting "+a.manager.Name+" remediation", slog.Bool("dry_run", a.dryRun))

	installed, err := a.scanner.ListPackages(ctx)
	if err != nil {
		return fmt.Errorf("package scan failed: %w", err)
	}
	a.logger.DebugContext(ctx, "Scanned installed packages", slog.Int("count", len(installed)))

	if len(installed) == 0 {
		fmt.Printf("No packages found via %s\n", a.manager.Name)
		return nil
	}

	names := upgradeNames(installed, a.ignoreSet)
	if len(names) == 0 {
		fmt.Println("\nNo packages to upgrade — all installed packages are ignored!")
		return nil
	}

	if a.dryRun {
		fmt.Println("\n=== DRY-RUN MODE ===")
		fmt.Printf("\n%d package(s) will be upgraded to the latest version:\n", len(names))
		for _, name := range names {
			fmt.Printf("  • %s\n", name)
		}
		fmt.Printf("\nTo apply: rootio_patcher %s remediate --dry-run=false\n", a.manager.Name)
		return nil
	}

	if err := a.executor.Refresh(ctx); err != nil {
		return fmt.Errorf("%s refresh failed: %w", a.manager.Name, err)
	}

	fmt.Printf("\nUpgrading %d package(s)...\n", len(names))
	if err := a.executor.UpgradeAll(ctx, names); err != nil {
		return fmt.Errorf("upgrade failed: %w", err)
	}

	fmt.Println("\n✓ " + a.manager.Name + " remediation complete")
	return nil
}

// upgradeNames returns the sorted, de-duplicated, non-ignored installed
// package names to upgrade.
func upgradeNames(installed []InstalledPackage, ignoreSet map[string]struct{}) []string {
	ignored := ignoredNames(ignoreSet)

	seen := make(map[string]struct{}, len(installed))
	for _, pkg := range installed {
		if _, isIgnored := ignored[pkg.Name]; isIgnored {
			continue
		}
		seen[pkg.Name] = struct{}{}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ignoredNames extracts the package-name portion from each ignoreSet key.
// Keys are "name@version"; the name is the substring before the LAST "@".
// A bare key with no "@" (e.g. "nginx") means "never touch nginx", mirroring
// common.ignoreNamesFromSet used by the apt/apk OS-upgrade path.
func ignoredNames(ignoreSet map[string]struct{}) map[string]struct{} {
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
