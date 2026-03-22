package npm

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/pkg/rootio"
)

// App handles npm package remediation (pre-install file patching)
type App struct {
	apiKey          string
	apiURL          string
	packageManager  string
	lockFilePath    string
	packageJSONPath string
	dryRun          bool
	logger          *slog.Logger
	parser          npmParser
	apiClient       common.APIClient
}

// NewApp creates a new npm application instance.
// directory is the project root where the lock file and package.json live (use "." for CWD).
func NewApp(apiKey, apiURL, packageManager, directory string, dryRun bool, logger *slog.Logger) *App {
	lockFileName := lockFileNameForPackageManager(packageManager)
	lockFilePath := filepath.Join(directory, lockFileName)

	parser, err := GetParserForPackageManagerInDir(packageManager, directory)
	if err != nil {
		parser = NewNpmParser()
	}

	return NewAppWithServices(
		apiKey,
		apiURL,
		lockFilePath,
		dryRun,
		logger,
		parser,
		rootio.NewClient(apiURL, apiKey),
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
	dryRun bool,
	logger *slog.Logger,
	parser npmParser,
	apiClient common.APIClient,
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
		logger:          logger,
		parser:          parser,
		apiClient:       apiClient,
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
	response, err := a.apiClient.AnalyzePackages(ctx, sdkPackages, "npm")
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

	// 6. Execute or dry-run patches
	if a.dryRun {
		a.logger.DebugContext(ctx, "DRY-RUN MODE: No changes will be made")
		a.reportDryRun(response.Patches)
		return nil
	}

	// 7. Apply patches by updating package.json
	fmt.Printf("\nApplying %d patches to package.json...\n\n", len(response.Patches))
	if err := a.applyPatches(ctx, response.Patches); err != nil {
		return err
	}

	fmt.Printf("\n✓ Successfully updated package.json with %d overrides!\n", len(response.Patches))
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Review the changes in package.json")
	fmt.Printf("  2. Run: %s install\n", a.packageManager)
	fmt.Println("  3. Test your application")

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
}

// applyPatches updates package.json with overrides
func (a *App) applyPatches(ctx context.Context, patches []rootio.PackagePatch) error {
	// Build overrides map: package name -> aliased package version
	// Always use aliased packages (e.g., express -> @rootio/express)
	overrides := make(map[string]string)
	for _, patch := range patches {
		// Use original package name as key, but aliased package name@version as value
		overrideValue := fmt.Sprintf("npm:%s@%s", patch.PatchAlias.Name, patch.PatchAlias.Version)
		overrides[patch.PackageName] = overrideValue
		fmt.Printf("  - %s: %s → %s@%s\n", patch.PackageName, patch.Version, patch.PatchAlias.Name, patch.PatchAlias.Version)
	}

	// Update package.json with overrides using the parser
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
