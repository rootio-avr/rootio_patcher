package nuget

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/pkg/rootio"
)

// App handles NuGet package remediation.
type App struct {
	apiKey    string
	apiURL    string
	path      string // file or directory path
	dryRun    bool
	useAlias  bool // true=rewrite to Root.io aliased package, false=keep original package name, patched version
	ignoreSet map[string]struct{}
	logger    *slog.Logger
	parser    common.Parser
	apiClient common.APIClient
}

// NewApp creates a new NuGet application instance.
func NewApp(apiKey, apiURL, path string, dryRun, useAlias bool, ignoreEntries []string, logger *slog.Logger) *App {
	ignoreDir := path
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		ignoreDir = filepath.Dir(path)
	}
	ignoreFilePath := filepath.Join(ignoreDir, ".rootioignore")
	return NewAppWithServices(apiKey, apiURL, path, dryRun, useAlias, common.LoadIgnoreList(ignoreFilePath, ignoreEntries), logger, NewParser(logger), rootio.NewClient(apiURL, apiKey))
}

// NewAppWithServices creates a new NuGet app with injected services (for testing).
func NewAppWithServices(
	apiKey, apiURL, path string,
	dryRun, useAlias bool,
	ignoreSet map[string]struct{},
	logger *slog.Logger,
	parser common.Parser,
	apiClient common.APIClient,
) *App {
	return &App{
		apiKey:    apiKey,
		apiURL:    apiURL,
		path:      path,
		dryRun:    dryRun,
		useAlias:  useAlias,
		ignoreSet: ignoreSet,
		logger:    logger,
		parser:    parser,
		apiClient: apiClient,
	}
}

// Run executes the NuGet remediation workflow.
func (a *App) Run(ctx context.Context) error {
	a.logger.DebugContext(ctx, "Starting NuGet remediation",
		slog.String("path", a.path),
		slog.Bool("dry_run", a.dryRun))

	// 1. Check path exists
	if _, err := os.Stat(a.path); err != nil {
		return fmt.Errorf("path not found: %s", a.path)
	}

	// 2. Parse packages
	a.logger.DebugContext(ctx, "Parsing NuGet manifests")
	packages, err := a.parser.Parse(ctx, a.path)
	if err != nil {
		return fmt.Errorf("failed to parse %s: %w", a.path, err)
	}
	a.logger.DebugContext(ctx, "Parsed packages", slog.Int("count", len(packages)))

	if len(packages) == 0 {
		fmt.Println("\nNo packages found in NuGet manifest(s)")
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

	// 4. Analyze for vulnerabilities
	a.logger.DebugContext(ctx, "Analyzing packages for vulnerabilities")
	response, err := a.apiClient.AnalyzePackages(ctx, sdkPackages, common.IgnoreListToPackages(a.ignoreSet), "nuget")
	if err != nil {
		return fmt.Errorf("failed to analyze packages: %w", err)
	}

	a.logger.DebugContext(ctx, "Vulnerability analysis complete",
		slog.Int("patches_available", len(response.Patches)),
		slog.Int("packages_skipped", len(response.Skipped)))

	if len(response.Patches) == 0 {
		fmt.Println("\nNo patches needed - all packages are up to date!")
		return nil
	}

	// 5. Dry-run or apply
	if a.dryRun {
		a.logger.DebugContext(ctx, "DRY-RUN MODE: No changes will be made")
		a.reportDryRun(response.Patches)
		return common.ErrPatchesAvailable
	}

	fmt.Printf("\nApplying %d patches...\n\n", len(response.Patches))
	if err := a.applyPatches(ctx, response.Patches); err != nil {
		return err
	}
	fmt.Printf("\n✓ Successfully patched %d packages!\n", len(response.Patches))
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Review the changes in your manifest file(s)")
	fmt.Println("  2. Run: dotnet restore")
	fmt.Println("  3. Test your application")
	return nil
}

// patchNameVersion returns the name/version to apply for a patch, based on
// useAlias: true rewrites to Root.io's aliased package, false keeps the
// original package name at the patched version. Falls back to Patch fields
// if PatchAlias is unset.
func (a *App) patchNameVersion(patch rootio.PackagePatch) (name, version string) {
	if a.useAlias {
		name, version = patch.PatchAlias.Name, patch.PatchAlias.Version
	} else {
		name, version = patch.Patch.Name, patch.Patch.Version
	}
	if name == "" {
		name = patch.Patch.Name
	}
	if version == "" {
		version = patch.Patch.Version
	}
	return name, version
}

// reportDryRun prints what would be changed without modifying files.
func (a *App) reportDryRun(patches []rootio.PackagePatch) {
	fmt.Println("\n=== DRY-RUN MODE ===")
	fmt.Printf("The following packages would be updated:\n\n")

	for i, patch := range patches {
		name, version := a.patchNameVersion(patch)
		fmt.Printf("%d. Package: %s\n", i+1, patch.PackageName)
		fmt.Printf("   Current version: %s\n", patch.Version)
		if a.useAlias {
			fmt.Printf("   Aliased package: %s @ %s\n", name, version)
		} else {
			fmt.Printf("   Patched version: %s\n", version)
		}
		if len(patch.CVEIDs) > 0 {
			fmt.Printf("   CVEs Fixed: %v\n", patch.CVEIDs)
		}
		fmt.Println()
	}

	fmt.Println("To apply these patches:")
	fmt.Println("  Run: rootio_patcher nuget remediate --dry-run=false")
	fmt.Println("  Then run: dotnet restore")
}


// applyPatches updates the manifest file(s) with patched versions.
// Packages are grouped by their source file (Location) so each file is updated independently.
func (a *App) applyPatches(ctx context.Context, patches []rootio.PackagePatch) error {
	// Re-parse to get Location info for each package so we know which file to update.
	packages, err := a.parser.Parse(ctx, a.path)
	if err != nil {
		return fmt.Errorf("failed to re-parse for location info: %w", err)
	}

	// Build a map from package name → source file path.
	pkgLocation := make(map[string]string, len(packages))
	for _, pkg := range packages {
		pkgLocation[pkg.Name] = pkg.Location
	}

	// Build a per-file updates map: file path → (original package name → "aliasName:aliasVersion").
	fileUpdates := make(map[string]map[string]string)
	for _, patch := range patches {
		name, version := a.patchNameVersion(patch)

		loc := pkgLocation[patch.PackageName]
		if loc == "" {
			loc = a.path
		}

		if fileUpdates[loc] == nil {
			fileUpdates[loc] = make(map[string]string)
		}
		fileUpdates[loc][patch.PackageName] = name + ":" + version
		fmt.Printf("  - %s: %s → %s@%s\n", patch.PackageName, patch.Version, name, version)
	}

	// Apply updates file by file.
	for filePath, updates := range fileUpdates {
		updatedContent, err := a.parser.Update(ctx, filePath, updates)
		if err != nil {
			return fmt.Errorf("failed to update %s: %w", filePath, err)
		}

		if !a.parser.Validate(updatedContent) {
			return fmt.Errorf("updated content of %s is invalid XML", filePath)
		}

		if err := os.WriteFile(filePath, []byte(updatedContent), 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", filePath, err)
		}
	}

	return nil
}
