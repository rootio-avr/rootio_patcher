package npm

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/pkg/rootio"
)

// CommandRunner runs external commands in a given directory with optional extra environment variables.
type CommandRunner interface {
	Run(ctx context.Context, dir string, env []string, name string, args ...string) error
}

// RealCommandRunner runs commands via os/exec.
type RealCommandRunner struct{}

// NewRealCommandRunner returns a CommandRunner backed by os/exec.
func NewRealCommandRunner() *RealCommandRunner { return &RealCommandRunner{} }

func (r *RealCommandRunner) Run(ctx context.Context, dir string, env []string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// App handles npm package remediation (pre-install file patching)
type App struct {
	apiKey          string
	apiURL          string
	packageManager  string
	lockFilePath    string
	packageJSONPath string
	dryRun          bool
	ignoreSet       map[string]struct{}
	useAlias        bool // Controls direct dep rewrite format: true=keep npm: prefix, false=replace package name
	logger          *slog.Logger
	parser          npmParser
	apiClient       common.APIClient
	cmdRunner       CommandRunner
	lookPath        func(string) (string, error) // Injectable PATH lookup for testing
}

// NewApp creates a new npm application instance.
// directory is the project root where the lock file and package.json live (use "." for CWD).
func NewApp(apiKey, apiURL, packageManager, directory string, dryRun, useAlias bool, ignoreEntries []string, logger *slog.Logger) *App {
	lockFileName := lockFileNameForPackageManager(packageManager)
	lockFilePath := filepath.Join(directory, lockFileName)

	parser, err := GetParserForPackageManagerInDir(packageManager, directory)
	if err != nil {
		parser = NewNpmParser()
	}

	ignoreFilePath := filepath.Join(directory, ".rootioignore")
	ignoreSet := common.LoadIgnoreList(ignoreFilePath, ignoreEntries)

	return NewAppWithServices(
		apiKey,
		apiURL,
		lockFilePath,
		dryRun,
		useAlias,
		ignoreSet,
		logger,
		parser,
		rootio.NewClient(apiURL, apiKey),
		NewRealCommandRunner(),
	)
}

// lockFileNameForPackageManager returns the default lock file name for a package manager.
func lockFileNameForPackageManager(packageManager string) string {
	switch packageManager {
	case "yarn":
		return "yarn.lock"
	case "pnpm":
		return "pnpm-lock.yaml"
	default:
		return "package-lock.json"
	}
}

// NewAppWithServices creates a new npm app with injected services (for testing).
// packageManagerOrPath accepts either a package manager name ("npm", "yarn", "pnpm")
// or an absolute/relative lock file path (used in tests).
func NewAppWithServices(
	apiKey, apiURL, packageManagerOrPath string,
	dryRun, useAlias bool,
	ignoreSet map[string]struct{},
	logger *slog.Logger,
	parser npmParser,
	apiClient common.APIClient,
	cmdRunner CommandRunner,
) *App {
	var packageManager string
	var lockFilePath string

	// Check if it's an absolute path (for testing) or a package manager name
	if filepath.IsAbs(packageManagerOrPath) || strings.Contains(packageManagerOrPath, string(filepath.Separator)) {
		// It's a file path (for testing)
		lockFilePath = packageManagerOrPath
		// Infer package manager from file extension
		switch {
		case strings.HasSuffix(lockFilePath, "yarn.lock"):
			packageManager = "yarn"
		case strings.HasSuffix(lockFilePath, "pnpm-lock.yaml"):
			packageManager = "pnpm"
		default:
			packageManager = "npm"
		}
	} else {
		// It's a package manager name
		packageManager = packageManagerOrPath
		lockFilePath = lockFileNameForPackageManager(packageManager)
	}

	// Derive package.json path from the directory containing the lock file
	packageJSONPath := filepath.Join(filepath.Dir(lockFilePath), "package.json")

	return &App{
		apiKey:          apiKey,
		apiURL:          apiURL,
		packageManager:  packageManager,
		lockFilePath:    lockFilePath,
		packageJSONPath: packageJSONPath,
		dryRun:          dryRun,
		useAlias:        useAlias,
		ignoreSet:       ignoreSet,
		logger:          logger,
		parser:          parser,
		apiClient:       apiClient,
		cmdRunner:       cmdRunner,
		lookPath:        exec.LookPath, // Default to real exec.LookPath
	}
}

// Run executes the npm remediation workflow
func (a *App) Run(ctx context.Context) error {
	a.logger.DebugContext(ctx, "Starting npm remediation",
		slog.String("package_manager", a.packageManager),
		slog.String("lock_file", a.lockFilePath),
		slog.Bool("dry_run", a.dryRun))

	// 1. Check if lock file exists - crash if not found
	if _, err := os.Stat(a.lockFilePath); err != nil {
		return fmt.Errorf("lock file not found: %s (package manager: %s)", a.lockFilePath, a.packageManager)
	}

	// 2. Parse lock file
	a.logger.DebugContext(ctx, "Parsing lock file", slog.String("file", a.lockFilePath))
	packages, err := a.parser.Parse(ctx, a.lockFilePath)
	if err != nil {
		return fmt.Errorf("failed to parse %s: %w", a.lockFilePath, err)
	}
	a.logger.DebugContext(ctx, "Parsed packages", slog.Int("count", len(packages)))

	if len(packages) == 0 {
		fmt.Printf("\nNo packages found in %s\n", a.lockFilePath)
		return nil
	}

	// 3. Convert to SDK format
	sdkPackages := make([]rootio.Package, len(packages))
	for i, pkg := range packages {
		sdkPackages[i] = rootio.Package{
			Name:    pkg.Name,
			Version: pkg.Version,
		}
	}

	// 4. Call backend API to analyze vulnerabilities
	a.logger.DebugContext(ctx, "Analyzing packages for vulnerabilities")
	response, err := a.apiClient.AnalyzePackages(ctx, sdkPackages, common.IgnoreListToPackages(a.ignoreSet), "npm")
	if err != nil {
		return fmt.Errorf("failed to analyze packages: %w", err)
	}

	// 5. Log analysis results
	a.logger.DebugContext(ctx, "Vulnerability analysis complete",
		slog.Int("patches_available", len(response.Patches)),
		slog.Int("packages_skipped", len(response.Skipped)))

	if len(response.Patches) == 0 {
		fmt.Println("\nNo patches needed - all packages are up to date!")
		return nil
	}

	if len(response.Patches) == 0 {
		fmt.Println("\nNo patches needed - all packages are up to date!")
		return nil
	}

	// 6. Execute or dry-run patches
	if a.dryRun {
		a.logger.DebugContext(ctx, "DRY-RUN MODE: No changes will be made")
		a.reportDryRun(response.Patches)
		return common.ErrPatchesAvailable
	}

	// 7. Apply patches by updating package.json
	fmt.Printf("\nApplying %d patches to package.json...\n\n", len(response.Patches))
	if err := a.applyPatches(ctx, response.Patches); err != nil {
		return err
	}

	// 8. Resolve the patched manifest so the lockfile reflects the overrides (self-contained inline).
	// Degrade gracefully: if the package manager isn't on PATH (manifest patched in a stage that
	// resolves later), warn and skip rather than fail the build.
	pm := a.packageManager
	if pm == "" {
		pm = "npm"
	}
	if _, lookErr := a.lookPath(pm); lookErr != nil {
		a.logger.WarnContext(ctx, "package manager not found on PATH, skipping dependency resolution; lockfile left unresolved (downstream install/ci may reject the out-of-sync lock)", slog.String("resolver", pm))
	} else {
		dir := filepath.Dir(a.lockFilePath)
		a.logger.DebugContext(ctx, "Running install to resolve patched manifest", slog.String("dir", dir), slog.String("pm", pm))
		if err := a.cmdRunner.Run(ctx, dir, a.npmEnv(), pm, "install"); err != nil {
			return fmt.Errorf("%s install failed: %w", pm, err)
		}
	}

	fmt.Printf("\n✓ Successfully updated package.json with %d overrides!\n", len(response.Patches))
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Review the changes in package.json")
	fmt.Printf("  2. Run: %s install\n", a.packageManager)
	fmt.Println("  3. Test your application")
	if a.isYarnClassic() {
		printYarnClassicCaveat()
	}

	return nil
}

// reportDryRun shows what would be changed without modifying files
func (a *App) reportDryRun(patches []rootio.PackagePatch) {
	fmt.Println("\n=== DRY-RUN MODE ===")
	fmt.Printf("The following overrides would be added to package.json:\n\n")

	// Determine override field name based on package manager
	overrideField := a.getOverrideField()

	for i, patch := range patches {
		fmt.Printf("%d. Package: %s\n", i+1, patch.PackageName)
		fmt.Printf("   Current version: %s\n", patch.Version)
		fmt.Printf("   Aliased package: npm:%s@%s\n", patch.PatchAlias.Name, patch.PatchAlias.Version)
		if len(patch.CVEIDs) > 0 {
			fmt.Printf("   CVEs Fixed: %v\n", patch.CVEIDs)
		}
		fmt.Println()
	}

	// Show where overrides will be placed
	if a.packageManager == "pnpm" {
		fmt.Printf("These will be added to package.json under \"pnpm.overrides\" field\n\n")
	} else {
		fmt.Printf("These will be added to package.json under \"%s\" field\n\n", overrideField)
	}

	fmt.Println("To apply these patches, run with --dry-run=false")
	fmt.Printf("Then run: %s install\n", a.packageManager)
	if a.isYarnClassic() {
		printYarnClassicCaveat()
	}
}

// isYarnClassic returns true when the configured lockfile is a Yarn 1 yarn.lock.
// Yarn 1 marks v1 lockfiles with a "# yarn lockfile v1" header; Yarn 2+ uses YAML
// with an __metadata block instead. We only care about this for --package-manager=yarn.
func (a *App) isYarnClassic() bool {
	if a.packageManager != "yarn" {
		return false
	}
	content, err := os.ReadFile(a.lockFilePath)
	if err != nil {
		return false
	}
	return strings.Contains(string(content), "# yarn lockfile v1")
}

// printYarnClassicCaveat warns Yarn 1 users that a plain `yarn install` won't
// re-resolve from newly-added resolutions — the existing lock entries still
// satisfy the original ranges, so yarn reports "Already up-to-date".
func printYarnClassicCaveat() {
	fmt.Println()
	fmt.Println("Note: Yarn 1 (classic) won't re-resolve from \"resolutions\" alone.")
	fmt.Println("      If yarn reports \"Already up-to-date\", do one of:")
	fmt.Println("        - rm yarn.lock && yarn install   (clean re-resolve)")
	fmt.Println("        - yarn install --force            (force re-fetch)")
}

// applyPatches updates package.json with overrides scoped to the vulnerable
// package version. It never touches direct dependencies — the user's chosen
// versions remain authoritative.
func (a *App) applyPatches(ctx context.Context, patches []rootio.PackagePatch) error {
	overrides := make([]ScopedOverride, 0, len(patches))
	for _, patch := range patches {
		parents, err := a.parser.FindParents(ctx, a.lockFilePath, patch.PackageName, patch.Version)
		if err != nil {
			return fmt.Errorf("failed to find parents for %s@%s: %w", patch.PackageName, patch.Version, err)
		}
		direct, err := a.parser.IsDirectVulnerable(ctx, a.lockFilePath, a.packageJSONPath, patch.PackageName, patch.Version)
		if err != nil {
			return fmt.Errorf("failed to check direct dep for %s@%s: %w", patch.PackageName, patch.Version, err)
		}

		// Choose patch info based on useAlias flag
		patchInfo := patch.Patch
		if a.useAlias {
			patchInfo = patch.PatchAlias
		}

		// For overrides (transitive deps), always use aliased package with npm: prefix
		overrideValue := fmt.Sprintf("npm:%s@%s", patch.PatchAlias.Name, patch.PatchAlias.Version)

		overrides = append(overrides, ScopedOverride{
			PackageName:   patch.PackageName,
			Version:       patch.Version,
			Value:         overrideValue,
			PatchInfo:     patchInfo, // Used for direct dependency rewrite
			Parents:       parents,
			RewriteDirect: direct,
			UseAlias:      a.useAlias,
		})
		var scopes []string
		if direct {
			scopes = append(scopes, "direct")
		}
		if len(parents) > 0 {
			scopes = append(scopes, "under "+strings.Join(parents, ", "))
		}
		if len(scopes) == 0 {
			fmt.Printf("  - %s@%s → %s@%s\n", patch.PackageName, patch.Version, patch.PatchAlias.Name, patch.PatchAlias.Version)
		} else {
			fmt.Printf("  - %s@%s (%s) → %s@%s\n", patch.PackageName, patch.Version, strings.Join(scopes, ", "), patch.PatchAlias.Name, patch.PatchAlias.Version)
		}
	}

	a.logger.DebugContext(ctx, "Updating package.json with overrides", slog.Int("count", len(overrides)))
	if err := a.parser.UpdatePackageJSON(ctx, overrides, a.packageJSONPath); err != nil {
		return fmt.Errorf("failed to update package.json: %w", err)
	}

	return nil
}

// getOverrideField returns the override field name based on package manager
// Used only for display purposes in dry-run mode
func (a *App) getOverrideField() string {
	switch a.packageManager {
	case "yarn":
		return "resolutions"
	case "npm", "pnpm":
		return "overrides"
	default:
		return "overrides"
	}
}

// npmEnv returns extra environment for the resolver.
// npm overrides pin @rootio-scoped packages published to the public npm registry
// (registry.npmjs.org); the resolver needs no pkg.root.io authentication.
// Returns nil intentionally.
func (a *App) npmEnv() []string {
	return nil
}
