package npm

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/pkg/rootio"

	"github.com/tidwall/gjson"
	"github.com/tidwall/pretty"
	"github.com/tidwall/sjson"
)

// App handles npm package remediation (pre-install file patching)
type App struct {
	apiKey         string
	apiURL         string
	packageManager string
	lockFilePath   string
	dryRun         bool
	logger         *slog.Logger
	parser         common.Parser
	apiClient      common.APIClient
}

// NewApp creates a new npm application instance
func NewApp(apiKey, apiURL, packageManager string, dryRun bool, logger *slog.Logger) *App {
	return NewAppWithServices(
		apiKey,
		apiURL,
		packageManager,
		dryRun,
		logger,
		NewParser(),
		rootio.NewClient(apiURL, apiKey),
	)
}

// NewAppWithServices creates a new npm app with injected services (for testing)
// For testing: if packageManager is an absolute path, it's treated as a lock file path
func NewAppWithServices(
	apiKey, apiURL, packageManagerOrPath string,
	dryRun bool,
	logger *slog.Logger,
	parser common.Parser,
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
		// Determine lock file path based on package manager
		switch packageManager {
		case "npm":
			lockFilePath = "package-lock.json"
		case "yarn":
			lockFilePath = "yarn.lock"
		case "pnpm":
			lockFilePath = "pnpm-lock.yaml"
		default:
			lockFilePath = "package-lock.json"
		}
	}

	return &App{
		apiKey:         apiKey,
		apiURL:         apiURL,
		packageManager: packageManager,
		lockFilePath:   lockFilePath,
		dryRun:         dryRun,
		logger:         logger,
		parser:         parser,
		apiClient:      apiClient,
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

	// Update package.json with overrides
	a.logger.DebugContext(ctx, "Updating package.json with overrides", slog.Int("count", len(overrides)))
	if err := a.updatePackageJSON(overrides); err != nil {
		return fmt.Errorf("failed to update package.json: %w", err)
	}

	return nil
}

// getOverrideField returns the override field name based on package manager
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

// escapeSjsonKey escapes special characters for sjson paths
// Only @ and . need escaping based on sjson behavior
func escapeSjsonKey(key string) string {
	key = strings.ReplaceAll(key, `\`, `\\`)
	key = strings.ReplaceAll(key, `@`, `\@`)
	key = strings.ReplaceAll(key, `.`, `\.`)
	return key
}

// detectIndentation detects the indentation used in JSON file
func detectIndentation(content []byte) string {
	// Look for first indented line after opening brace
	re := regexp.MustCompile(`\{\s*\n([ \t]+)`)
	matches := re.FindSubmatch(content)
	if len(matches) > 1 {
		return string(matches[1])
	}
	// Default to 2 spaces
	return "  "
}

// updatePackageJSON updates package.json with version overrides while preserving structure
func (a *App) updatePackageJSON(overrides map[string]string) error {
	packageJSONPath := "package.json"

	// Check if package.json exists
	if _, err := os.Stat(packageJSONPath); err != nil {
		return fmt.Errorf("package.json not found in current directory")
	}

	// Read package.json as bytes
	content, err := os.ReadFile(packageJSONPath)
	if err != nil {
		return fmt.Errorf("failed to read package.json: %w", err)
	}

	// Detect original indentation
	indent := detectIndentation(content)

	// Update direct dependencies if they exist
	for pkgName, overrideValue := range overrides {
		escapedPkg := escapeSjsonKey(pkgName)

		// Check if package exists in dependencies
		if gjson.GetBytes(content, "dependencies."+escapedPkg).Exists() {
			content, err = sjson.SetBytes(content, "dependencies."+escapedPkg, overrideValue)
			if err != nil {
				return fmt.Errorf("failed to update dependencies.%s: %w", pkgName, err)
			}
		}

		// Check if package exists in devDependencies
		if gjson.GetBytes(content, "devDependencies."+escapedPkg).Exists() {
			content, err = sjson.SetBytes(content, "devDependencies."+escapedPkg, overrideValue)
			if err != nil {
				return fmt.Errorf("failed to update devDependencies.%s: %w", pkgName, err)
			}
		}
	}

	// Add or update overrides based on package manager (for transitive dependencies)
	overrideField := a.getOverrideField()

	// pnpm requires nested structure: { "pnpm": { "overrides": { ... } } }
	if a.packageManager == "pnpm" {
		for pkgName, overrideValue := range overrides {
			escapedPkg := escapeSjsonKey(pkgName)
			content, err = sjson.SetBytes(content, "pnpm.overrides."+escapedPkg, overrideValue)
			if err != nil {
				return fmt.Errorf("failed to update pnpm.overrides.%s: %w", pkgName, err)
			}
		}
	} else {
		// npm and yarn use top-level field
		for pkgName, overrideValue := range overrides {
			escapedPkg := escapeSjsonKey(pkgName)
			content, err = sjson.SetBytes(content, overrideField+"."+escapedPkg, overrideValue)
			if err != nil {
				return fmt.Errorf("failed to update %s.%s: %w", overrideField, pkgName, err)
			}
		}
	}

	// Pretty-print with detected indentation to fix formatting
	content = pretty.PrettyOptions(content, &pretty.Options{
		Width:    80,
		Prefix:   "",
		Indent:   indent,
		SortKeys: false,
	})

	// Write to file
	if err := os.WriteFile(packageJSONPath, content, 0644); err != nil {
		return fmt.Errorf("failed to write package.json: %w", err)
	}

	return nil
}
