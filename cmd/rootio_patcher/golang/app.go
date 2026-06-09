package golang

import (
	"context"
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
	apiKey    string
	apiURL    string
	pkgURL    string
	goModPath string
	dryRun    bool
	useAlias  bool
	ignoreSet map[string]struct{}
	logger    *slog.Logger
	parser    GoModParser
	apiClient common.APIClient
	cmdRunner CommandRunner
}

// NewApp creates a new App with injected services.
func NewApp(
	apiKey, apiURL, pkgURL, goModPath string,
	dryRun, useAlias bool,
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
		useAlias:  useAlias,
		ignoreSet: common.LoadIgnoreList(ignoreFilePath, ignoreEntries),
		logger:    logger,
		parser:    parser,
		apiClient: apiClient,
		cmdRunner: cmdRunner,
	}
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

// goEnv returns env vars for aliased patching: GONOSUMDB covers the Root.io proxy host.
func (a *App) goEnv() []string {
	u, err := url.Parse(a.pkgURL)
	if err != nil {
		return nil
	}
	return a.buildGoEnv(u.Hostname())
}

// goEnvNonAliased returns env vars for non-aliased patching: GONOSUMDB is scoped to only
// the patched module paths so all other modules still get checksum-verified.
func (a *App) goEnvNonAliased(patches []rootio.PackagePatch) []string {
	moduleNames := make([]string, len(patches))
	for i, patch := range patches {
		moduleNames[i] = patch.PackageName
	}
	return a.buildGoEnv(strings.Join(moduleNames, ","))
}

// removeGoSumEntries removes all go.sum lines for the given patched modules so Go
// will re-fetch and re-hash them from the Root.io proxy.
func removeGoSumEntries(goSumPath string, patches []rootio.PackagePatch) error {
	data, err := os.ReadFile(goSumPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	toRemove := make(map[string]struct{}, len(patches))
	for _, patch := range patches {
		toRemove[patch.PackageName] = struct{}{}
	}

	var kept []string
	for line := range strings.SplitSeq(string(data), "\n") {
		if line == "" {
			continue
		}
		// go.sum line: "<module> <version>[/go.mod] h1:<hash>="
		parts := strings.SplitN(line, " ", 2)
		if _, skip := toRemove[parts[0]]; !skip {
			kept = append(kept, line)
		}
	}

	content := strings.Join(kept, "\n")
	if len(kept) > 0 {
		content += "\n"
	}
	return os.WriteFile(goSumPath, []byte(content), 0644)
}

// Run executes the Go module remediation workflow.
func (a *App) Run(ctx context.Context) error {
	a.logger.DebugContext(ctx, "Starting golang remediation",
		slog.String("go_mod", a.goModPath),
		slog.Bool("dry_run", a.dryRun),
		slog.Bool("use_alias", a.useAlias))

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

	// 5. Handle no patches
	if len(response.Patches) == 0 {
		fmt.Println("\nNo patches available - all packages are up to date!")
		return nil
	}

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

	goModDir := filepath.Dir(a.goModPath)
	goEnv := a.goEnv()

	if a.useAlias {
		// 7a. Aliased: write replace directives pointing to pkg.root.io/...
		updates := make([]GoModUpdate, len(response.Patches))
		for i, patch := range response.Patches {
			updates[i] = GoModUpdate{
				Module:         patch.PackageName,
				CurrentVersion: patch.Version,
				AliasName:      patch.PatchAlias.Name,
				AliasVersion:   patch.PatchAlias.Version,
			}
		}

		fmt.Printf("\nApplying %d patch(es) to %s...\n\n", len(updates), a.goModPath)

		updatedContent, err := a.parser.Patch(ctx, a.goModPath, updates)
		if err != nil {
			return fmt.Errorf("failed to patch %s: %w", a.goModPath, err)
		}
		if err := os.WriteFile(a.goModPath, []byte(updatedContent), 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", a.goModPath, err)
		}

		fmt.Printf("\n✓ Successfully patched %s with %d replace directive(s)!\n", a.goModPath, len(updates))
	} else {
		// 7b. Non-aliased: remove go.sum entries for patched modules so Go re-fetches
		// them from the Root.io proxy (which serves patched bytes under the original paths).
		goSumPath := filepath.Join(goModDir, "go.sum")
		if err := removeGoSumEntries(goSumPath, response.Patches); err != nil {
			return fmt.Errorf("failed to update go.sum: %w", err)
		}

		// Use GONOSUMDB scoped to patched modules only so others still get verified.
		goEnv = a.goEnvNonAliased(response.Patches)

		fmt.Printf("\nFetching %d patch(es) via Root.io proxy (non-aliased)...\n\n", len(response.Patches))
		for _, patch := range response.Patches {
			fmt.Printf("  - %s %s\n", patch.PackageName, patch.Version)
			arg := patch.PackageName + "@" + patch.Version
			if err := a.cmdRunner.Run(ctx, goModDir, goEnv, "go", "get", arg); err != nil {
				return fmt.Errorf("go get %s failed: %w", arg, err)
			}
		}
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

// reportDryRun prints what would happen without modifying files.
func (a *App) reportDryRun(patches []rootio.PackagePatch) {
	fmt.Println("\n=== DRY-RUN MODE ===")

	if a.useAlias {
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
	} else {
		fmt.Printf("The following packages would be fetched via the Root.io proxy (no replace directives added to %s):\n\n", a.goModPath)
		for i, patch := range patches {
			fmt.Printf("%d. %s %s (patched via proxy)\n", i+1, patch.PackageName, patch.Version)
			if len(patch.CVEIDs) > 0 {
				fmt.Printf("   CVEs Fixed: %v\n", patch.CVEIDs)
			}
			fmt.Println()
		}
	}

	fmt.Println("To apply these patches:")
	fmt.Println("  1. Run: rootio_patcher go remediate --dry-run=false")
	fmt.Println("  2. Then run: go build ./...")
}
