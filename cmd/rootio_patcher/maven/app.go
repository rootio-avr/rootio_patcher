package maven

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/pkg/rootio"
)

// App handles Maven package remediation (pre-install file patching)
type App struct {
	apiKey    string
	apiURL    string
	filePath  string
	dryRun    bool
	logger    *slog.Logger
	parser    common.Parser
	apiClient common.APIClient
}

// NewApp creates a new Maven application instance
func NewApp(apiKey, apiURL, filePath string, dryRun bool, logger *slog.Logger) *App {
	return NewAppWithServices(
		apiKey,
		apiURL,
		filePath,
		dryRun,
		logger,
		NewParser(),
		rootio.NewClient(apiURL, apiKey),
	)
}

// NewAppWithServices creates a new Maven app with injected services (for testing)
func NewAppWithServices(
	apiKey, apiURL, filePath string,
	dryRun bool,
	logger *slog.Logger,
	parser common.Parser,
	apiClient common.APIClient,
) *App {
	return &App{
		apiKey:    apiKey,
		apiURL:    apiURL,
		filePath:  filePath,
		dryRun:    dryRun,
		logger:    logger,
		parser:    parser,
		apiClient: apiClient,
	}
}

// Run executes the Maven remediation workflow
func (a *App) Run(ctx context.Context) error {
	a.logger.DebugContext(ctx, "Starting Maven remediation",
		slog.String("file", a.filePath),
		slog.Bool("dry_run", a.dryRun))

	// 1. Check if file exists
	if _, err := os.Stat(a.filePath); err != nil {
		return fmt.Errorf("file not found: %s", a.filePath)
	}

	// 2. Parse pom.xml
	a.logger.DebugContext(ctx, "Parsing pom.xml")
	packages, err := a.parser.Parse(ctx, a.filePath)
	if err != nil {
		return fmt.Errorf("failed to parse %s: %w", a.filePath, err)
	}
	a.logger.DebugContext(ctx, "Parsed packages", slog.Int("count", len(packages)))

	if len(packages) == 0 {
		fmt.Println("\nNo packages found in pom.xml")
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
	response, err := a.apiClient.AnalyzePackages(ctx, sdkPackages)
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

	// 7. Apply patches by updating the file
	fmt.Printf("\nApplying %d patches to %s...\n\n", len(response.Patches), a.filePath)
	if err := a.applyPatches(ctx, response.Patches); err != nil {
		return err
	}

	fmt.Printf("\n✓ Successfully updated %s with %d patches!\n", a.filePath, len(response.Patches))
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Review the changes in your pom.xml")
	fmt.Println("  2. Run: mvn clean install")
	fmt.Println("  3. Test your application")

	return nil
}

// reportDryRun shows what would be changed without modifying files
func (a *App) reportDryRun(patches []rootio.PackagePatch) {
	fmt.Println("\n=== DRY-RUN MODE ===")
	fmt.Printf("The following packages in %s would be updated:\n\n", a.filePath)

	for i, patch := range patches {
		fmt.Printf("%d. Package: %s\n", i+1, patch.PackageName)
		fmt.Printf("   Current version: %s\n", patch.Version)
		fmt.Printf("   Patched version: %s\n", patch.Patch.Version)
		if len(patch.CVEIDs) > 0 {
			fmt.Printf("   CVEs Fixed: %v\n", patch.CVEIDs)
		}
		fmt.Println()
	}

	fmt.Println("To apply these patches:")
	fmt.Printf("  1. Run: DRY_RUN=false rootio_patcher maven remediate\n")
	fmt.Println("  2. Then run: mvn clean install")
}

// applyPatches updates the pom.xml file with patched versions
func (a *App) applyPatches(ctx context.Context, patches []rootio.PackagePatch) error {
	// Build updates map: package name -> new version
	updates := make(map[string]string)
	for _, patch := range patches {
		updates[patch.PackageName] = patch.Patch.Version
		fmt.Printf("  - %s: %s → %s\n", patch.PackageName, patch.Version, patch.Patch.Version)
	}

	// Update the file
	a.logger.DebugContext(ctx, "Updating pom.xml", slog.Int("updates", len(updates)))
	updatedContent, err := a.parser.Update(ctx, a.filePath, updates)
	if err != nil {
		return fmt.Errorf("failed to update file: %w", err)
	}

	// Validate the updated content
	if !a.parser.Validate(updatedContent) {
		return fmt.Errorf("updated file content is invalid")
	}

	// Write the updated content back to the file
	if err := os.WriteFile(a.filePath, []byte(updatedContent), 0644); err != nil {
		return fmt.Errorf("failed to write updated file: %w", err)
	}

	return nil
}
