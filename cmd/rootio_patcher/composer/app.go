package composer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
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

// App handles Composer package remediation.
type App struct {
	apiKey    string
	apiURL    string
	pkgURL    string
	filePath  string
	dryRun    bool
	useAlias  bool
	logger    *slog.Logger
	parser    common.Parser
	apiClient common.APIClient
	cmdRunner CommandRunner
}

// NewApp creates a new App with default injected services.
func NewApp(apiKey, apiURL, pkgURL, filePath string, dryRun, useAlias bool, logger *slog.Logger) *App {
	return NewAppWithServices(
		apiKey, apiURL, pkgURL, filePath, dryRun, useAlias, logger,
		NewParser(logger, pkgURL),
		rootio.NewClient(apiURL, apiKey),
		NewRealCommandRunner(),
	)
}

// NewAppWithServices creates a new App with injected services (for testing).
func NewAppWithServices(
	apiKey, apiURL, pkgURL, filePath string,
	dryRun, useAlias bool,
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
		logger:    logger,
		parser:    parser,
		apiClient: apiClient,
		cmdRunner: cmdRunner,
	}
}

// Run executes the Composer remediation workflow.
func (a *App) Run(ctx context.Context) error {
	a.logger.DebugContext(ctx, "Starting Composer remediation",
		slog.String("file", a.filePath),
		slog.Bool("dry_run", a.dryRun),
		slog.Bool("use_alias", a.useAlias))

	if _, err := os.Stat(a.filePath); err != nil {
		return fmt.Errorf("file not found: %s", a.filePath)
	}

	a.logger.DebugContext(ctx, "Parsing composer.lock")
	packages, err := a.parser.Parse(ctx, a.filePath)
	if err != nil {
		return fmt.Errorf("failed to parse %s: %w", a.filePath, err)
	}
	a.logger.DebugContext(ctx, "Parsed packages", slog.Int("count", len(packages)))

	if len(packages) == 0 {
		fmt.Println("\nNo packages found in composer.lock")
		return nil
	}

	sdkPackages := make([]rootio.Package, len(packages))
	for i, pkg := range packages {
		sdkPackages[i] = rootio.Package{Name: pkg.Name, Version: pkg.Version}
	}

	a.logger.DebugContext(ctx, "Analyzing packages for vulnerabilities")
	response, err := a.apiClient.AnalyzePackages(ctx, sdkPackages, "composer")
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

	if a.dryRun {
		a.logger.DebugContext(ctx, "DRY-RUN MODE: No changes will be made")
		a.reportDryRun(response.Patches)
		return common.ErrPatchesAvailable
	}

	fmt.Printf("\nApplying %d patch(es) to %s...\n\n", len(response.Patches), a.filePath)
	if err := a.applyPatches(ctx, response.Patches); err != nil {
		return err
	}

	fmt.Printf("\n✓ Successfully patched %s with %d update(s)!\n", a.filePath, len(response.Patches))
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Review the changes in composer.json and composer.lock")
	fmt.Println("  2. Ensure COMPOSER_AUTH is configured in your CI/CD environment for future installs")
	fmt.Println("  3. Test and deploy your application")

	return nil
}

// applyPatches updates composer.json and runs composer update.
func (a *App) applyPatches(ctx context.Context, patches []rootio.PackagePatch) error {
	updates := make(map[string]string, len(patches))
	affectedPackages := make([]string, 0, len(patches))

	for _, patch := range patches {
		var patchInfo rootio.PatchInfo
		if a.useAlias {
			patchInfo = patch.PatchAlias
		} else {
			patchInfo = patch.Patch
		}

		updates[patch.PackageName] = patchInfo.Name + ":" + patchInfo.Version
		affectedPackages = append(affectedPackages, patchInfo.Name)
		fmt.Printf("  - %s: %s → %s@%s\n", patch.PackageName, patch.Version, patchInfo.Name, patchInfo.Version)
	}

	updatedContent, err := a.parser.Update(ctx, a.filePath, updates)
	if err != nil {
		return fmt.Errorf("failed to update %s: %w", a.filePath, err)
	}

	if !a.parser.Validate(updatedContent) {
		return fmt.Errorf("updated composer.json content is invalid JSON")
	}

	if err := os.WriteFile(a.filePath, []byte(updatedContent), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", a.filePath, err)
	}

	dir := filepath.Dir(a.filePath)
	args := []string{"update", "--with-dependencies"}
	if a.logger.Enabled(context.Background(), slog.LevelDebug) {
		args = append(args, "-vvv")
	}
	args = append(args, affectedPackages...)
	a.logger.DebugContext(ctx, "Running composer update", slog.String("dir", dir))
	if err := a.cmdRunner.Run(ctx, dir, a.composerEnv(), "composer", args...); err != nil {
		return fmt.Errorf("composer update failed: %w", err)
	}

	return nil
}

// composerEnv returns the environment variables needed to authenticate with pkg.root.io.
// Credentials are passed in-process via COMPOSER_AUTH and never written to disk.
func (a *App) composerEnv() []string {
	u, err := url.Parse(a.pkgURL)
	if err != nil {
		return nil
	}

	auth := map[string]interface{}{
		"http-basic": map[string]interface{}{
			u.Hostname(): map[string]string{
				"username": "",
				"password": a.apiKey,
			},
		},
	}
	authJSON, err := json.Marshal(auth)
	if err != nil {
		return nil
	}

	return []string{"COMPOSER_AUTH=" + string(authJSON)}
}

// reportDryRun prints what would be changed without modifying files.
func (a *App) reportDryRun(patches []rootio.PackagePatch) {
	fmt.Println("\n=== DRY-RUN MODE ===")
	fmt.Printf("The following packages in %s would be updated:\n\n", a.filePath)

	for i, patch := range patches {
		var patchInfo rootio.PatchInfo
		if a.useAlias {
			patchInfo = patch.PatchAlias
		} else {
			patchInfo = patch.Patch
		}

		fmt.Printf("%d. Package: %s\n", i+1, patch.PackageName)
		fmt.Printf("   Current version: %s\n", patch.Version)
		if a.useAlias && patchInfo.Name != patch.PackageName {
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
	fmt.Println("  Run: rootio_patcher composer remediate --dry-run=false")
	fmt.Println("  Then ensure COMPOSER_AUTH is configured in your CI/CD environment")
}
