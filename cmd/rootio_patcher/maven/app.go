package maven

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

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

// App handles Maven package remediation (pre-install file patching)
type App struct {
	apiKey    string
	apiURL    string
	pkgURL    string
	filePath  string
	dryRun    bool
	useAlias  bool // true=rewrite to io.root.io.* group ID, false=keep original groupId, patched version
	ignoreSet map[string]struct{}
	logger    *slog.Logger
	parser    common.Parser
	apiClient common.APIClient
	cmdRunner CommandRunner
	lookPath  func(string) (string, error) // Injectable PATH lookup for testing
}

// NewApp creates a new Maven application instance
func NewApp(apiKey, apiURL, pkgURL, filePath string, dryRun, useAlias bool, ignoreEntries []string, logger *slog.Logger) *App {
	ignoreFilePath := filepath.Join(filepath.Dir(filePath), ".rootioignore")
	return NewAppWithServices(apiKey, apiURL, pkgURL, filePath, dryRun, useAlias, common.LoadIgnoreList(ignoreFilePath, ignoreEntries), logger, NewParser(logger), rootio.NewClient(apiURL, apiKey), NewRealCommandRunner())
}

// NewAppWithServices creates a new Maven app with injected services (for testing)
func NewAppWithServices(
	apiKey, apiURL, pkgURL, filePath string,
	dryRun, useAlias bool,
	ignoreSet map[string]struct{},
	logger *slog.Logger,
	parser common.Parser,
	apiClient common.APIClient,
	cmdRunner CommandRunner,
) *App {
	return &App{
		apiKey:    apiKey,
		apiURL:    apiURL,
		pkgURL:    pkgURL,
		filePath:  filePath,
		dryRun:    dryRun,
		useAlias:  useAlias,
		ignoreSet: ignoreSet,
		logger:    logger,
		parser:    parser,
		apiClient: apiClient,
		cmdRunner: cmdRunner,
		lookPath:  exec.LookPath, // Default to real exec.LookPath
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
	response, err := a.apiClient.AnalyzePackages(ctx, sdkPackages, common.IgnoreListToPackages(a.ignoreSet), "maven")
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
		return common.ErrPatchesAvailable
	}

	// 7. Apply patches by updating the file
	fmt.Printf("\nApplying %d patches to %s...\n\n", len(response.Patches), a.filePath)
	if err := a.applyPatches(ctx, response.Patches); err != nil {
		return err
	}

	// 8. Resolve the patched manifest so dependencies are downloaded and the pom is validated (self-contained inline).
	// Degrade gracefully: if mvn isn't on PATH (manifest patched in a stage that
	// resolves later), warn and skip rather than fail the build.
	if _, lookErr := a.lookPath("mvn"); lookErr != nil {
		a.logger.WarnContext(ctx, "mvn not found on PATH, skipping dependency resolution; patched pom left unresolved (downstream build may fail to fetch the patched dependencies)", slog.String("resolver", "mvn"))
	} else {
		dir := filepath.Dir(a.filePath)
		a.logger.DebugContext(ctx, "Running mvn dependency:resolve", slog.String("dir", dir))
		if err := a.cmdRunner.Run(ctx, dir, a.mavenEnv(), "mvn", "dependency:resolve"); err != nil {
			return fmt.Errorf("mvn dependency:resolve failed: %w", err)
		}
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
		patchInfo := patch.Patch
		if a.useAlias {
			patchInfo = patch.PatchAlias
		}
		fmt.Printf("%d. Package: %s\n", i+1, patch.PackageName)
		fmt.Printf("   Current version: %s\n", patch.Version)
		if a.useAlias {
			fmt.Printf("   Aliased package: %s @ %s\n", patchInfo.Name, patchInfo.Version)
		} else {
			fmt.Printf("   Patched version: %s\n", patchInfo.Version)
		}
		if len(patch.CVEIDs) > 0 {
			fmt.Printf("   CVEs Fixed: %v\n", patch.CVEIDs)
		}
		fmt.Println()
	}

	fmt.Println("To apply these patches:")
	fmt.Printf("  1. Run: rootio_patcher maven remediate --dry-run=false\n")
	fmt.Println("  2. Then run: mvn clean install")
}

// applyPatches updates the pom.xml file with patched versions
func (a *App) applyPatches(ctx context.Context, patches []rootio.PackagePatch) error {
	// Build updates map: package name -> new groupId:artifactId:version
	// This format supports changing the groupId for Root.io patched packages.
	// useAlias=true rewrites the groupId to Root.io's io.root.* namespace;
	// useAlias=false keeps the original groupId, only bumping the version.
	updates := make(map[string]string)
	for _, patch := range patches {
		patchInfo := patch.Patch
		if a.useAlias {
			patchInfo = patch.PatchAlias
		}
		updateValue := patchInfo.Name + ":" + patchInfo.Version
		updates[patch.PackageName] = updateValue
		fmt.Printf("  - %s: %s → %s:%s\n", patch.PackageName, patch.Version, patchInfo.Name, patchInfo.Version)
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

// mavenEnv returns extra environment for the resolver.
// Root.io repo auth (settings.xml) is a deferred follow-up. Returns nil for now.
func (a *App) mavenEnv() []string {
	return nil
}
