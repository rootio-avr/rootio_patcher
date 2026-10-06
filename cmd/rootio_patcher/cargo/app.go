package cargo

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/pkg/rootio"
)

const patchHeader = "[patch.crates-io]"

// CommandRunner runs external commands in a given directory with optional extra environment variables.
type CommandRunner interface {
	Run(ctx context.Context, dir string, env []string, name string, args ...string) error
}

// RealCommandRunner runs commands via os/exec.
type RealCommandRunner struct{}

func (RealCommandRunner) Run(ctx context.Context, dir string, env []string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// App handles Cargo remediation. Cargo ignores "+aikido.N" when resolving, so each patched
// revision is its own sparse registry, wired in with [patch.crates-io] in the workspace-root
// Cargo.toml and [registries.<name>] in .cargo/config.toml.
type App struct {
	apiKey      string
	registryURL string // e.g. https://pkg.root.io/cargo
	dir         string // workspace root (holds Cargo.toml and Cargo.lock)
	dryRun      bool
	reportPath  string
	ignoreSet   map[string]struct{}
	logger      *slog.Logger
	apiClient   common.APIClient
	cmdRunner   CommandRunner
}

// NewApp creates a Cargo remediation App.
func NewApp(
	apiKey, registryURL, dir string,
	dryRun bool,
	reportPath string,
	ignoreEntries []string,
	logger *slog.Logger,
	apiClient common.APIClient,
	cmdRunner CommandRunner,
) *App {
	return &App{
		apiKey:      apiKey,
		registryURL: strings.TrimRight(registryURL, "/"),
		dir:         dir,
		dryRun:      dryRun,
		reportPath:  reportPath,
		ignoreSet:   common.LoadIgnoreList(filepath.Join(dir, ".rootioignore"), ignoreEntries),
		logger:      logger,
		apiClient:   apiClient,
		cmdRunner:   cmdRunner,
	}
}

// cratePatch is one patched revision, e.g. itoa 1.0.15 -> 1.0.15+aikido.3.
type cratePatch struct {
	rootio.PackagePatch
	base     string // "1.0.15"
	revision string // "3"
}

// registry is the per-version registry name, e.g. "aikido-itoa-1-0-15". A newer revision reuses
// it with a new index URL, so the entry is replaced and the CI token env var stays the same.
func (p cratePatch) registry() string {
	return "aikido-" + p.PackageName + "-" + strings.ReplaceAll(p.base, ".", "-")
}

// tokenEnv is the env var cargo reads a registry's token from.
func tokenEnv(registry string) string {
	return "CARGO_REGISTRIES_" + strings.ToUpper(strings.ReplaceAll(registry, "-", "_")) + "_TOKEN"
}

// Run executes the Cargo remediation workflow.
func (a *App) Run(ctx context.Context) error {
	lockPath := filepath.Join(a.dir, "Cargo.lock")
	var lock struct {
		Package []struct{ Name, Version, Source string } `toml:"package"`
	}
	if _, err := toml.DecodeFile(lockPath, &lock); err != nil {
		return fmt.Errorf("failed to read %s (run `cargo generate-lockfile` first): %w", lockPath, err)
	}

	var packages []rootio.Package
	for _, p := range lock.Package {
		// Registry crates only; path and git deps have no source or a git+ source.
		if strings.HasPrefix(p.Source, "registry+") || strings.HasPrefix(p.Source, "sparse+") {
			packages = append(packages, rootio.Package{Name: p.Name, Version: p.Version})
		}
	}
	if len(packages) == 0 {
		fmt.Println("\nNo registry crates found in Cargo.lock")
		return a.writeReport(nil)
	}

	response, err := a.apiClient.AnalyzePackages(ctx, packages, common.IgnoreListToPackages(a.ignoreSet), "cargo")
	if err != nil {
		return fmt.Errorf("failed to analyze packages: %w", err)
	}

	patches := make([]cratePatch, 0, len(response.Patches))
	for _, p := range response.Patches {
		base, revision, ok := strings.Cut(p.Patch.Version, "+aikido.")
		if !ok {
			return fmt.Errorf("unexpected patched version %q for %s", p.Patch.Version, p.PackageName)
		}
		patches = append(patches, cratePatch{PackagePatch: p, base: base, revision: revision})
	}
	if err := a.writeReport(patches); err != nil {
		return err
	}
	if len(patches) == 0 {
		fmt.Println("\nNo patches needed - all crates are up to date!")
		return nil
	}

	if a.dryRun {
		a.reportDryRun(patches)
		return common.ErrPatchesAvailable
	}

	fmt.Printf("\nApplying %d patch(es) in %s...\n\n", len(patches), a.dir)
	registries, err := a.applyPatches(ctx, patches)
	if err != nil {
		return err
	}

	fmt.Printf("\n✓ Successfully patched %d crate(s)!\n", len(patches))
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Review the changes in Cargo.toml, .cargo/config.toml and Cargo.lock")
	fmt.Println("  2. Set these env vars wherever cargo builds this project (CI/CD):")
	for _, name := range registries {
		fmt.Printf("       %s=\"Basic $(printf 'root:%%s' \"$ROOTIO_API_KEY\" | base64)\"\n", tokenEnv(name))
	}
	fmt.Println("  3. Build and test: cargo build")
	return nil
}

// applyPatches edits the Cargo files, runs `cargo update` (restoring the files if it fails) and
// returns every aikido registry in .cargo/config.toml, since cargo needs a token for each of them.
func (a *App) applyPatches(ctx context.Context, patches []cratePatch) ([]string, error) {
	manifestPath := filepath.Join(a.dir, "Cargo.toml")
	configPath := filepath.Join(a.dir, ".cargo", "config.toml")

	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", manifestPath, err)
	}
	config, err := os.ReadFile(configPath)
	configExisted := err == nil
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to read %s: %w", configPath, err)
	}

	names := make(map[string]bool, len(patches))
	entries := make([]string, 0, len(patches))
	tables := make([]string, 0, len(patches))
	for _, p := range patches {
		fmt.Printf("  - %s: %s → %s\n", p.PackageName, p.Version, p.Patch.Version)
		names[p.registry()] = true
		entries = append(entries, fmt.Sprintf("%s = { package = %q, version = %q, registry = %q }",
			p.registry(), p.PackageName, "="+p.base, p.registry()))
		tables = append(tables, fmt.Sprintf("[registries.%s]\nindex = %q\ncredential-provider = \"cargo:token\"\n",
			p.registry(), fmt.Sprintf("sparse+%s/%s/%s/aikido.%s/", a.registryURL, p.PackageName, p.base, p.revision)))
	}

	files := map[string]string{
		manifestPath: addPatchEntries(string(manifest), names, entries),
		configPath:   addRegistryTables(string(config), names, tables),
	}
	if _, err := toml.Decode(files[manifestPath], &map[string]any{}); err != nil {
		return nil, fmt.Errorf("refusing to write invalid TOML to %s: %w", manifestPath, err)
	}
	var parsedConfig struct {
		Registries map[string]any `toml:"registries"`
	}
	if _, err := toml.Decode(files[configPath], &parsedConfig); err != nil {
		return nil, fmt.Errorf("refusing to write invalid TOML to %s: %w", configPath, err)
	}
	var registries []string
	for name := range parsedConfig.Registries {
		if strings.HasPrefix(name, "aikido-") {
			registries = append(registries, name)
		}
	}
	slices.Sort(registries)

	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return nil, err
	}
	if err := writeFiles(files); err != nil {
		return nil, err
	}

	// Without `cargo update` Cargo keeps the locked crates.io copy. The token goes in env only:
	// the registry URL ends up in Cargo.lock.
	args := []string{"update"}
	for _, p := range patches {
		args = append(args, "-p", p.PackageName+"@"+p.Version)
	}
	env := make([]string, 0, len(registries))
	token := "Basic " + base64.StdEncoding.EncodeToString([]byte("root:"+a.apiKey))
	for _, name := range registries {
		env = append(env, tokenEnv(name)+"="+token)
	}
	if err := a.cmdRunner.Run(ctx, a.dir, env, "cargo", args...); err != nil {
		restoreErr := writeFiles(map[string]string{manifestPath: string(manifest)})
		if configExisted {
			restoreErr = errors.Join(restoreErr, writeFiles(map[string]string{configPath: string(config)}))
		} else {
			restoreErr = errors.Join(restoreErr, os.Remove(configPath))
		}
		if restoreErr != nil {
			return nil, fmt.Errorf("cargo update failed: %w (restoring Cargo files also failed: %w)", err, restoreErr)
		}
		return nil, fmt.Errorf("cargo update failed, Cargo files restored: %w", err)
	}
	return registries, nil
}

func writeFiles(files map[string]string) error {
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return fmt.Errorf("failed to write %s: %w", path, err)
		}
	}
	return nil
}

// addPatchEntries drops our existing entries from [patch.crates-io] and inserts entries under its
// header, appending the table if missing. Text edits keep the user's comments and formatting.
// ponytail: only matches single-line `key = {...}` entries, which is what we write.
func addPatchEntries(manifest string, names map[string]bool, entries []string) string {
	lines := strings.Split(manifest, "\n")
	out := make([]string, 0, len(lines)+len(entries)+2)
	inPatch, inserted := false, false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			inPatch = trimmed == patchHeader
		}
		if inPatch && names[strings.TrimSpace(strings.SplitN(trimmed, "=", 2)[0])] {
			continue
		}
		out = append(out, line)
		if trimmed == patchHeader && !inserted {
			out = append(out, entries...)
			inserted = true
		}
	}
	result := strings.Join(out, "\n")
	if !inserted {
		result = strings.TrimRight(result, "\n") + "\n\n" + patchHeader + "\n" + strings.Join(entries, "\n") + "\n"
	}
	return result
}

// addRegistryTables replaces our [registries.<name>] tables with the new ones.
func addRegistryTables(config string, names map[string]bool, tables []string) string {
	var out []string
	skipping := false
	for _, line := range strings.Split(config, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			skipping = names[strings.TrimSuffix(strings.TrimPrefix(trimmed, "[registries."), "]")]
		}
		if !skipping {
			out = append(out, line)
		}
	}
	result := strings.TrimRight(strings.Join(out, "\n"), "\n")
	if result != "" {
		result += "\n\n"
	}
	return result + strings.Join(tables, "\n")
}

// ReportEntry is one remediated crate in the --report file.
type ReportEntry struct {
	Name       string   `json:"name"`
	OldVersion string   `json:"old_version"`
	NewVersion string   `json:"new_version"`
	CVEIDs     []string `json:"cve_ids"`
}

// writeReport records what was found (in dry-run too) when --report is set; no-op otherwise.
func (a *App) writeReport(patches []cratePatch) error {
	if a.reportPath == "" {
		return nil
	}
	entries := make([]ReportEntry, 0, len(patches))
	for _, p := range patches {
		cves := p.CVEIDs
		if cves == nil {
			cves = []string{}
		}
		entries = append(entries, ReportEntry{Name: p.PackageName, OldVersion: p.Version, NewVersion: p.Patch.Version, CVEIDs: cves})
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}
	if err := os.WriteFile(a.reportPath, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("failed to write report %s: %w", a.reportPath, err)
	}
	return nil
}

func (a *App) reportDryRun(patches []cratePatch) {
	fmt.Println("\n=== DRY-RUN MODE ===")
	fmt.Printf("The following crates in %s would be patched:\n\n", a.dir)
	for i, p := range patches {
		fmt.Printf("%d. %s: %s → %s\n", i+1, p.PackageName, p.Version, p.Patch.Version)
		if len(p.CVEIDs) > 0 {
			fmt.Printf("   CVEs Fixed: %v\n", p.CVEIDs)
		}
	}
	fmt.Println("\nTo apply these patches:")
	fmt.Println("  Run: rootio_patcher cargo remediate --dry-run=false")
}
