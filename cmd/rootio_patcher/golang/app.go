package golang

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

// goModParser is the interface for parsing and patching go.mod files.
type goModParser interface {
	Parse(ctx context.Context, filePath string) ([]common.PackageInfo, error)
	Patch(ctx context.Context, filePath string, updates []GoModUpdate) (string, error)
}

// CommandRunner runs external commands in a given directory.
type CommandRunner interface {
	Run(ctx context.Context, dir string, name string, args ...string) error
}

// realCommandRunner runs commands via os/exec.
type realCommandRunner struct{}

func (r *realCommandRunner) Run(ctx context.Context, dir string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// App handles Go module remediation.
type App struct {
	apiKey    string
	apiURL    string
	goModPath string
	dryRun    bool
	logger    *slog.Logger
	parser    goModParser
	apiClient common.APIClient
	cmdRunner CommandRunner
}

// NewApp creates a new App with production services.
func NewApp(apiKey, apiURL, goModPath string, dryRun bool, logger *slog.Logger) *App {
	return NewAppWithServices(
		apiKey, apiURL, goModPath, dryRun, logger,
		NewGoModParser(logger),
		rootio.NewClient(apiURL, apiKey),
		&realCommandRunner{},
	)
}

// NewAppWithServices creates a new App with injected services (for testing).
func NewAppWithServices(
	apiKey, apiURL, goModPath string,
	dryRun bool,
	logger *slog.Logger,
	parser goModParser,
	apiClient common.APIClient,
	cmdRunner CommandRunner,
) *App {
	return &App{
		apiKey:    apiKey,
		apiURL:    apiURL,
		goModPath: goModPath,
		dryRun:    dryRun,
		logger:    logger,
		parser:    parser,
		apiClient: apiClient,
		cmdRunner: cmdRunner,
	}
}

// Run executes the Go module remediation workflow.
func (a *App) Run(ctx context.Context) error {
	a.logger.DebugContext(ctx, "Starting golang remediation",
		slog.String("go_mod", a.goModPath),
		slog.Bool("dry_run", a.dryRun))

	// 1. Check go.mod exists
	if _, err := os.Stat(a.goModPath); err != nil {
		return fmt.Errorf("file not found: %s", a.goModPath)
	}

	// 2. Parse go.mod
	a.logger.DebugContext(ctx, "Parsing go.mod")
	packages, err := a.parser.Parse(ctx, a.goModPath)
	if err != nil {
		return fmt.Errorf("failed to parse %s: %w", a.goModPath, err)
	}
	a.logger.DebugContext(ctx, "Parsed packages", slog.Int("count", len(packages)))

	if len(packages) == 0 {
		fmt.Println("\nNo packages found in go.mod")
		return nil
	}

	// 3. Convert to API format
	sdkPackages := make([]rootio.Package, len(packages))
	for i, pkg := range packages {
		sdkPackages[i] = rootio.Package{
			Name:    pkg.Name,
			Version: pkg.Version,
		}
	}

	// 4. Call API
	a.logger.DebugContext(ctx, "Analyzing packages for vulnerabilities")
	response, err := a.apiClient.AnalyzePackages(ctx, sdkPackages, "golang")
	if err != nil {
		return fmt.Errorf("failed to analyze packages: %w", err)
	}

	a.logger.DebugContext(ctx, "Vulnerability analysis complete",
		slog.Int("patches_available", len(response.Patches)),
		slog.Int("packages_skipped", len(response.Skipped)))

	// 5. Handle no patches
	if len(response.Patches) == 0 {
		fmt.Println("\nNo patches available - all packages are up to date!")
		return nil
	}

	// 6. Dry-run: print what would be done
	if a.dryRun {
		a.logger.DebugContext(ctx, "DRY-RUN MODE: No changes will be made")
		a.reportDryRun(response.Patches)
		return nil
	}

	// 7. Build updates
	updates := make([]GoModUpdate, len(response.Patches))
	for i, patch := range response.Patches {
		updates[i] = GoModUpdate{
			Module:         patch.PackageName,
			CurrentVersion: patch.Version,
			AliasName:      patch.PatchAlias.Name,
			AliasVersion:   patch.PatchAlias.Version,
		}
	}

	// 8. Apply patches to go.mod
	fmt.Printf("\nApplying %d patch(es) to %s...\n\n", len(updates), a.goModPath)
	for _, u := range updates {
		fmt.Printf("  - replace %s %s => %s %s\n", u.Module, u.CurrentVersion, u.AliasName, u.AliasVersion)
	}

	updatedContent, err := a.parser.Patch(ctx, a.goModPath, updates)
	if err != nil {
		return fmt.Errorf("failed to patch %s: %w", a.goModPath, err)
	}
	if err := os.WriteFile(a.goModPath, []byte(updatedContent), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", a.goModPath, err)
	}

	// 9. Warn about checksum verification
	fmt.Println()
	fmt.Println("WARNING: pkg.root.io modules may not be in the Go checksum database.")
	fmt.Println("  If you encounter checksum errors, set one of:")
	fmt.Println("    GONOSUMCHECK=pkg.root.io/*")
	fmt.Println("    GONOSUMDB=pkg.root.io")

	// 10. Run go mod tidy
	goModDir := filepath.Dir(a.goModPath)
	a.logger.DebugContext(ctx, "Running go mod tidy", slog.String("dir", goModDir))
	if err := a.cmdRunner.Run(ctx, goModDir, "go", "mod", "tidy"); err != nil {
		return fmt.Errorf("go mod tidy failed: %w", err)
	}

	// 11. If vendor directory exists, run go mod vendor
	vendorFile := filepath.Join(goModDir, "vendor", "modules.txt")
	if _, err := os.Stat(vendorFile); err == nil {
		a.logger.DebugContext(ctx, "Vendor directory detected, running go mod vendor", slog.String("dir", goModDir))
		if err := a.cmdRunner.Run(ctx, goModDir, "go", "mod", "vendor"); err != nil {
			return fmt.Errorf("go mod vendor failed: %w", err)
		}
	}

	fmt.Printf("\n✓ Successfully patched %s with %d replace directive(s)!\n", a.goModPath, len(updates))
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Review the changes in your go.mod")
	fmt.Println("  2. Run: go build ./...")
	fmt.Println("  3. Test your application")

	return nil
}

// reportDryRun prints the replace directives that would be added without modifying files.
func (a *App) reportDryRun(patches []rootio.PackagePatch) {
	fmt.Println("\n=== DRY-RUN MODE ===")
	fmt.Printf("The following replace directives would be added to %s:\n\n", a.goModPath)

	for i, patch := range patches {
		fmt.Printf("%d. replace %s %s => %s %s\n",
			i+1,
			patch.PackageName, patch.Version,
			patch.PatchAlias.Name, patch.PatchAlias.Version)
		if len(patch.CVEIDs) > 0 {
			fmt.Printf("   CVEs Fixed: %v\n", patch.CVEIDs)
		}
		fmt.Println()
	}

	fmt.Println("To apply these patches:")
	fmt.Println("  1. Run: rootio_patcher golang remediate --dry-run=false")
	fmt.Println("  2. Then run: go build ./...")
}
