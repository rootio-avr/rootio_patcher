package golang

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
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

// App handles Go module remediation.
type App struct {
	apiKey     string
	apiURL     string
	pkgURL     string
	goModPath  string
	reportPath string // set by WithReport; empty disables the report
	dryRun     bool
	ignoreSet  map[string]struct{}
	logger     *slog.Logger
	parser     GoModParser
	apiClient  common.APIClient
	cmdRunner  CommandRunner
}

// NewApp creates a new App with injected services.
func NewApp(
	apiKey, apiURL, pkgURL, goModPath string,
	dryRun bool,
	ignoreEntries []string,
	logger *slog.Logger,
	parser GoModParser,
	apiClient common.APIClient,
	cmdRunner CommandRunner,
) *App {
	ignoreFilePath := filepath.Join(filepath.Dir(goModPath), ".rootioignore")
	return &App{
		apiKey:    apiKey,
		apiURL:    apiURL,
		pkgURL:    pkgURL,
		goModPath: goModPath,
		dryRun:    dryRun,
		ignoreSet: common.LoadIgnoreList(ignoreFilePath, ignoreEntries),
		logger:    logger,
		parser:    parser,
		apiClient: apiClient,
		cmdRunner: cmdRunner,
	}
}

// WithReport makes Run write a JSON report of the remediated modules to path. An empty path
// disables it.
func (a *App) WithReport(path string) *App {
	a.reportPath = path
	return a
}

// buildGoEnv constructs the go command environment with the Root.io GOPROXY and the given GONOSUMDB value.
func (a *App) buildGoEnv(noSumDB string) []string {
	u, err := url.Parse(a.pkgURL)
	if err != nil {
		return nil
	}
	proxyURL := &url.URL{
		Scheme: u.Scheme,
		User:   url.UserPassword("", a.apiKey),
		Host:   u.Host,
		Path:   "/gobinary",
	}
	return []string{
		"GOPROXY=" + proxyURL.String() + ",https://proxy.golang.org,direct",
		"GONOSUMDB=" + noSumDB,
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
	response, err := a.apiClient.AnalyzePackages(ctx, sdkPackages, common.IgnoreListToPackages(a.ignoreSet), "golang")
	if err != nil {
		return fmt.Errorf("failed to analyze packages: %w", err)
	}

	a.logger.DebugContext(ctx, "Vulnerability analysis complete",
		slog.Int("patches_available", len(response.Patches)),
		slog.Int("packages_skipped", len(response.Skipped)))

	// 5. Record what was found, in both dry-run and apply mode, so an empty report means
	// "nothing to fix" rather than "remediation never ran"
	if err := a.writeReport(response.Patches); err != nil {
		return err
	}

	// 6. Handle no patches
	if len(response.Patches) == 0 {
		fmt.Println("\nNo patches available - all packages are up to date!")
		return nil
	}

	// 7. Dry-run: print what would be done
	if a.dryRun {
		a.logger.DebugContext(ctx, "DRY-RUN MODE: No changes will be made")
		a.reportDryRun(response.Patches)
		return nil
	}

	goModDir := filepath.Dir(a.goModPath)
	goEnv := a.goEnv(response.Patches)

	// 7. Write replace directives, pointing each module at the patched version.
	updates := make([]GoModUpdate, len(response.Patches))
	for i, patch := range response.Patches {
		name, version := a.replaceTarget(patch)
		updates[i] = GoModUpdate{
			Module:         patch.PackageName,
			CurrentVersion: patch.Version,
			AliasName:      name,
			AliasVersion:   version,
		}
	}

	if err := a.applyGoModUpdates(ctx, updates); err != nil {
		return err
	}

	// 8. Run go mod tidy (with Root.io GOPROXY so the proxy supplies patched modules)
	a.logger.DebugContext(ctx, "Running go mod tidy", slog.String("dir", goModDir))
	if err := a.cmdRunner.Run(ctx, goModDir, goEnv, "go", "mod", "tidy"); err != nil {
		return fmt.Errorf("go mod tidy failed: %w", err)
	}

	// 9. If a vendor directory exists, run go mod vendor
	vendorFile := filepath.Join(goModDir, "vendor", "modules.txt")
	if _, err := os.Stat(vendorFile); err == nil {
		a.logger.DebugContext(ctx, "Vendor directory detected, running go mod vendor", slog.String("dir", goModDir))
		if err := a.cmdRunner.Run(ctx, goModDir, goEnv, "go", "mod", "vendor"); err != nil {
			return fmt.Errorf("go mod vendor failed: %w", err)
		}
	}

	fmt.Println("\nNext steps:")
	fmt.Println("  1. Review the changes in your go.mod")
	fmt.Println("  2. Run: go build ./...")
	fmt.Println("  3. Test your application")

	return nil
}

// ReportEntry is one remediated module in the --report file. Callers such as the binary build
// pipeline read this to learn which CVEs a dependency bump closed, since that is invisible in the
// built artifact.
type ReportEntry struct {
	Name       string   `json:"name"`
	OldVersion string   `json:"old_version"`
	NewVersion string   `json:"new_version"`
	CVEIDs     []string `json:"cve_ids"`
}

// writeReport records the remediation as JSON when --report is set, and is a no-op otherwise.
func (a *App) writeReport(patches []rootio.PackagePatch) error {
	if a.reportPath == "" {
		return nil
	}

	entries := make([]ReportEntry, 0, len(patches))
	for _, patch := range patches {
		_, newVersion := a.replaceTarget(patch)
		cves := patch.CVEIDs
		if cves == nil {
			cves = []string{}
		}
		entries = append(entries, ReportEntry{
			Name:       patch.PackageName,
			OldVersion: patch.Version,
			NewVersion: newVersion,
			CVEIDs:     cves,
		})
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}
	if err := os.WriteFile(a.reportPath, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("failed to write report %s: %w", a.reportPath, err)
	}

	a.logger.Debug("wrote remediation report",
		slog.String("path", a.reportPath), slog.Int("modules", len(entries)))
	return nil
}

// reportDryRun prints what would happen without modifying files.
func (a *App) reportDryRun(patches []rootio.PackagePatch) {
	fmt.Println("\n=== DRY-RUN MODE ===")

	fmt.Printf("The following replace directives would be added to %s:\n\n", a.goModPath)
	for i, patch := range patches {
		name, version := a.replaceTarget(patch)
		fmt.Printf("%d. replace %s %s => %s %s\n", i+1, patch.PackageName, patch.Version, name, version)
		if len(patch.CVEIDs) > 0 {
			fmt.Printf("   CVEs Fixed: %v\n", patch.CVEIDs)
		}
		fmt.Println()
	}

	fmt.Println("To apply these patches:")
	fmt.Println("  1. Run: rootio_patcher go remediate --dry-run=false")
	fmt.Println("  2. Then run: go build ./...")
}

// applyGoModUpdates patches go.mod with the given replace directives and writes the result
// to disk, printing progress messages. Shared by both the aliased and non-aliased flows.
func (a *App) applyGoModUpdates(ctx context.Context, updates []GoModUpdate) error {
	fmt.Printf("\nAdding %d replace directive(s) to %s...\n\n", len(updates), a.goModPath)

	updatedContent, err := a.parser.Patch(ctx, a.goModPath, updates)
	if err != nil {
		return fmt.Errorf("failed to patch %s: %w", a.goModPath, err)
	}
	if err := os.WriteFile(a.goModPath, []byte(updatedContent), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", a.goModPath, err)
	}

	fmt.Printf("\n✓ Successfully patched %s with %d replace directive(s)!\n", a.goModPath, len(updates))
	return nil
}

// goEnv returns env vars for the `go mod tidy`/`go mod vendor` step. GONOSUMDB is scoped to
// just the patched module paths so all other modules still get checksum-verified.
func (a *App) goEnv(patches []rootio.PackagePatch) []string {
	moduleNames := make([]string, len(patches))
	for i, patch := range patches {
		moduleNames[i] = patch.PackageName
	}
	return a.buildGoEnv(strings.Join(moduleNames, ","))
}

// replaceTarget returns the module path and version a require should be redirected to for a
// given patch: the same module path at the patched version.
func (a *App) replaceTarget(patch rootio.PackagePatch) (name, version string) {
	return patch.PackageName, patch.Patch.Version
}
