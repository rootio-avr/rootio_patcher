package nuget

import (
	"context"
	"encoding/xml"
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

// App handles NuGet package remediation.
type App struct {
	apiKey    string
	apiURL    string
	pkgURL    string
	path      string // file or directory path
	dryRun    bool
	useAlias  bool // true=rewrite to Root.io aliased package, false=keep original package name, patched version
	ignoreSet map[string]struct{}
	logger    *slog.Logger
	parser    common.Parser
	apiClient common.APIClient
	cmdRunner CommandRunner
	lookPath  func(string) (string, error) // Injectable PATH lookup for testing
}

// NewApp creates a new NuGet application instance.
func NewApp(apiKey, apiURL, pkgURL, path string, dryRun, useAlias bool, ignoreEntries []string, logger *slog.Logger) *App {
	ignoreDir := path
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		ignoreDir = filepath.Dir(path)
	}
	ignoreFilePath := filepath.Join(ignoreDir, ".rootioignore")
	return NewAppWithServices(apiKey, apiURL, pkgURL, path, dryRun, useAlias, common.LoadIgnoreList(ignoreFilePath, ignoreEntries), logger, NewParser(logger), rootio.NewClient(apiURL, apiKey), NewRealCommandRunner())
}

// NewAppWithServices creates a new NuGet app with injected services (for testing).
func NewAppWithServices(
	apiKey, apiURL, pkgURL, path string,
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
		path:      path,
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

	// 8. Resolve the patched manifest so dependencies are downloaded and the manifest is validated (self-contained inline).
	// Degrade gracefully: if dotnet isn't on PATH (manifest patched in a stage that
	// resolves later), warn and skip rather than fail the build.
	if _, lookErr := a.lookPath("dotnet"); lookErr != nil {
		a.logger.WarnContext(ctx, "dotnet not found on PATH, skipping dependency resolution; patched manifest left unresolved (downstream build may fail to fetch the patched dependencies)", slog.String("resolver", "dotnet"))
	} else {
		dir := a.path
		if fi, statErr := os.Stat(a.path); statErr == nil && !fi.IsDir() {
			dir = filepath.Dir(a.path)
		}
		// The patched manifest references `-root.io.N` package versions that live only in the
		// Root.io NuGet feed — nuget.org has no such versions. dotnet has no env-var form for
		// adding an authenticated source, so we write a temporary NuGet.Config (feed at
		// <pkgURL>/nuget/v3/index.json + basic auth root:<apiKey>) and pass it via --configfile.
		restoreArgs := []string{"restore"}
		configPath, cleanup, cerr := a.writeNuGetConfig(dir)
		if cerr != nil {
			a.logger.WarnContext(ctx, "could not write NuGet.Config for Root.io feed auth; restoring without it (patched versions may be unresolvable)", slog.String("error", cerr.Error()))
		} else if configPath != "" {
			defer cleanup()
			restoreArgs = append(restoreArgs, "--configfile", configPath)
		}
		a.logger.DebugContext(ctx, "Running dotnet restore", slog.String("dir", dir))
		if err := a.cmdRunner.Run(ctx, dir, a.nugetEnv(), "dotnet", restoreArgs...); err != nil {
			return fmt.Errorf("dotnet restore failed: %w", err)
		}
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
// original package name at the patched version.
func (a *App) patchNameVersion(patch rootio.PackagePatch) (name, version string) {
	if a.useAlias {
		name, version = patch.PatchAlias.Name, patch.PatchAlias.Version
	} else {
		name, version = patch.Patch.Name, patch.Patch.Version
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

		if name == "" || version == "" {
			continue
		}

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

// nugetEnv returns extra environment for the resolver. NuGet source auth cannot be
// expressed via environment variables (it requires a NuGet.Config), so this stays nil —
// see writeNuGetConfig, which supplies the Root.io feed + credentials via --configfile.
func (a *App) nugetEnv() []string {
	return nil
}

// writeNuGetConfig writes a temporary NuGet.Config that points `dotnet restore` at the
// Root.io NuGet feed (<pkgURL>/nuget/v3/index.json) with Root.io basic auth (user "root",
// password = apiKey), so it can fetch the `-root.io.N` patched packages that exist only
// there (never on nuget.org). Credentials use ClearTextPassword (dotnet expects an
// encrypted value otherwise, which isn't portable across machines/CI).
//
// Returns the config path and a cleanup func. If pkgURL or apiKey is unset it returns
// ("", noop, nil) so the caller restores without it (using whatever sources already apply).
func (a *App) writeNuGetConfig(dir string) (string, func(), error) {
	noop := func() {}
	if a.pkgURL == "" || a.apiKey == "" {
		return "", noop, nil
	}
	u, err := url.Parse(a.pkgURL)
	if err != nil || u.Host == "" {
		return "", noop, fmt.Errorf("invalid pkgURL %q: %w", a.pkgURL, err)
	}
	feedURL := fmt.Sprintf("%s://%s/nuget/v3/index.json", u.Scheme, u.Host)

	config := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<configuration>
  <packageSources>
    <add key="rootio" value="%s" />
  </packageSources>
  <packageSourceCredentials>
    <rootio>
      <add key="Username" value="root" />
      <add key="ClearTextPassword" value="%s" />
    </rootio>
  </packageSourceCredentials>
</configuration>
`, xmlEscape(feedURL), xmlEscape(a.apiKey))

	f, err := os.CreateTemp(dir, "rootio-nuget-*.config")
	if err != nil {
		f, err = os.CreateTemp("", "rootio-nuget-*.config")
		if err != nil {
			return "", noop, fmt.Errorf("create NuGet.Config: %w", err)
		}
	}
	path := f.Name()
	if _, err := f.WriteString(config); err != nil {
		f.Close()
		os.Remove(path)
		return "", noop, fmt.Errorf("write NuGet.Config: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return "", noop, fmt.Errorf("close NuGet.Config: %w", err)
	}
	return path, func() { os.Remove(path) }, nil
}

// xmlEscape escapes a string for safe inclusion in XML text/attribute content.
func xmlEscape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
