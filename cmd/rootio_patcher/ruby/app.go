package ruby

import (
	"bytes"
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

// CommandRunner runs an external command and returns its combined output.
type CommandRunner interface {
	Run(ctx context.Context, dir string, env []string, name string, args ...string) (string, error)
}

// RealCommandRunner runs commands via os/exec.
type RealCommandRunner struct{}

// NewRealCommandRunner returns a CommandRunner backed by os/exec.
func NewRealCommandRunner() *RealCommandRunner { return &RealCommandRunner{} }

func (r *RealCommandRunner) Run(ctx context.Context, dir string, env []string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

// App handles RubyGems (Bundler) remediation.
type App struct {
	apiKey    string
	pkgURL    string
	dir       string
	dryRun    bool
	logger    *slog.Logger
	apiClient common.APIClient
	runner    CommandRunner
	ignoreSet map[string]struct{}
}

// NewApp creates a new App with injected services.
func NewApp(
	apiKey, pkgURL, dir string,
	dryRun bool,
	ignoreEntries []string,
	logger *slog.Logger,
	apiClient common.APIClient,
	runner CommandRunner,
) *App {
	return &App{
		apiKey: apiKey, pkgURL: pkgURL, dir: dir, dryRun: dryRun,
		logger: logger, apiClient: apiClient, runner: runner,
		ignoreSet: common.LoadIgnoreList(filepath.Join(dir, ".rootioignore"), ignoreEntries),
	}
}

// sourceURL is the gem index written into the Gemfile. It carries no credentials: the secret
// is passed to Bundler through BUNDLE_PKG__ROOT__IO instead of being committed.
func (a *App) sourceURL() (string, error) {
	u, err := url.Parse(a.pkgURL)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("invalid ROOTIO_PKG_URL %q", a.pkgURL)
	}
	return fmt.Sprintf("%s://%s/ruby", u.Scheme, u.Host), nil
}

// bundlerCredentialEnv returns the Bundler env var holding credentials for the Root.io host.
// Bundler maps host "pkg.root.io" to BUNDLE_PKG__ROOT__IO (dots -> "__", dashes -> "___").
func (a *App) bundlerCredentialEnv() (string, error) {
	u, err := url.Parse(a.pkgURL)
	if err != nil || u.Hostname() == "" {
		return "", fmt.Errorf("invalid ROOTIO_PKG_URL %q", a.pkgURL)
	}
	host := strings.ToUpper(u.Hostname())
	host = strings.ReplaceAll(strings.ReplaceAll(host, "-", "___"), ".", "__")
	return fmt.Sprintf("BUNDLE_%s=root:%s", host, a.apiKey), nil
}

// Run executes the RubyGems remediation workflow.
func (a *App) Run(ctx context.Context) error {
	gemfilePath := filepath.Join(a.dir, "Gemfile")
	lockPath := filepath.Join(a.dir, "Gemfile.lock")

	originalGemfile, err := os.ReadFile(gemfilePath)
	if err != nil {
		return fmt.Errorf("failed to read Gemfile: %w", err)
	}
	originalLock, err := os.ReadFile(lockPath)
	if err != nil {
		return fmt.Errorf("failed to read Gemfile.lock (run `bundle install` first): %w", err)
	}

	gems := ParseLockfile(string(originalLock))
	a.logger.DebugContext(ctx, "Parsed Gemfile.lock", slog.Int("gems", len(gems)))
	if len(gems) == 0 {
		fmt.Println("\nNo gems found in Gemfile.lock")
		return nil
	}

	pkgs := make([]rootio.Package, len(gems))
	for i, g := range gems {
		pkgs[i] = rootio.Package{Name: g.Name, Version: g.Version}
	}
	resp, err := a.apiClient.AnalyzePackages(ctx, pkgs, common.IgnoreListToPackages(a.ignoreSet), "ruby")
	if err != nil {
		return fmt.Errorf("failed to analyze packages: %w", err)
	}
	a.logger.DebugContext(ctx, "Vulnerability analysis complete",
		slog.Int("patches_available", len(resp.Patches)),
		slog.Int("packages_skipped", len(resp.Skipped)))

	if len(resp.Patches) == 0 {
		fmt.Println("\nNo patches available - all gems are up to date!")
		return nil
	}

	if a.dryRun {
		a.reportDryRun(resp.Patches)
		return common.ErrPatchesAvailable
	}

	fmt.Printf("\nApplying %d patch(es)...\n\n", len(resp.Patches))
	if err := a.applyPatches(ctx, resp.Patches, string(originalGemfile), string(originalLock)); err != nil {
		// Never leave the project half-edited.
		_ = os.WriteFile(gemfilePath, originalGemfile, 0o644)
		_ = os.WriteFile(lockPath, originalLock, 0o644)
		return err
	}

	fmt.Printf("\n✓ Successfully patched %d gem(s)!\n", len(resp.Patches))
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Review the changes in Gemfile and Gemfile.lock")
	fmt.Println("  2. Provide credentials for future installs (not stored in the Gemfile):")
	fmt.Println("       export BUNDLE_PKG__ROOT__IO=root:<your_api_key>")
	fmt.Println("  3. Run: bundle install, then test your application")
	return nil
}

func patchTarget(p rootio.PackagePatch) (name, version string) {
	name, version = p.PatchAlias.Name, p.PatchAlias.Version
	if version == "" {
		version = p.Patch.Version
	}
	// Gem names are fixed by the gemspec: always the original name.
	return p.PackageName, version
}

func (a *App) applyPatches(ctx context.Context, patches []rootio.PackagePatch, gemfile, lock string) error {
	source, err := a.sourceURL()
	if err != nil {
		return err
	}
	credEnv, err := a.bundlerCredentialEnv()
	if err != nil {
		return err
	}

	editor := NewGemfileEditor(gemfile)
	names := make([]string, 0, len(patches))
	for _, p := range patches {
		name, version := patchTarget(p)
		fmt.Printf("  - %s: %s → %s\n", name, p.Version, version)
		if err := editor.PatchDependency(name, version); err != nil {
			return fmt.Errorf("cannot edit Gemfile for %s (declaration too dynamic to rewrite safely): %w", name, err)
		}
		if err := editor.SetGemSource(name, source); err != nil {
			return fmt.Errorf("cannot edit Gemfile for %s (declaration too dynamic to rewrite safely): %w", name, err)
		}
		names = append(names, name)
	}

	gemfilePath := filepath.Join(a.dir, "Gemfile")
	lockPath := filepath.Join(a.dir, "Gemfile.lock")
	if err := os.WriteFile(gemfilePath, []byte(editor.String()), 0o644); err != nil {
		return fmt.Errorf("failed to write Gemfile: %w", err)
	}

	env := []string{credEnv}
	if !strings.Contains(lock, "libv8-node") {
		env = append(env, "BUNDLE_FORCE_RUBY_PLATFORM=true") // don't add x86_64-linux to the lockfile
	}
	args := append([]string{"update"}, names...)
	args = append(args, "--conservative")
	a.logger.DebugContext(ctx, "Running bundle", slog.Any("args", args))
	out, err := a.runner.Run(ctx, a.dir, env, "bundle", args...)
	if err != nil {
		return fmt.Errorf("bundle update failed: %w\n%s", err, redact(out, a.apiKey))
	}

	newLockBytes, err := os.ReadFile(lockPath)
	if err != nil {
		return fmt.Errorf("failed to read Gemfile.lock after update: %w", err)
	}
	newLock := string(newLockBytes)
	for _, p := range patches {
		name, version := patchTarget(p)
		// Exit 0 only means some consistent set resolved; a gem behind a dead conditional
		// branch would resolve without ever touching our edit.
		if got := ResolvedVersion(newLock, name); got != "" && compareVersions(got, version) < 0 {
			return fmt.Errorf("bundle update succeeded but %s resolved to %s, not %s; "+
				"its declaration in the Gemfile could not be rewritten automatically", name, got, version)
		}
	}
	return os.WriteFile(lockPath, []byte(RestoreRubyAndBundlerInfo(lock, newLock)), 0o644)
}

func redact(s, secret string) string {
	if secret == "" {
		return s
	}
	return strings.ReplaceAll(s, secret, "***")
}

func (a *App) reportDryRun(patches []rootio.PackagePatch) {
	fmt.Println("\n=== DRY-RUN MODE ===")
	fmt.Printf("The following gems would be updated:\n\n")
	for i, p := range patches {
		name, version := patchTarget(p)
		fmt.Printf("%d. Gem: %s\n", i+1, name)
		fmt.Printf("   Current version: %s\n", p.Version)
		fmt.Printf("   Patched version: %s\n", version)
		if len(p.CVEIDs) > 0 {
			fmt.Printf("   CVEs Fixed: %v\n", p.CVEIDs)
		}
		fmt.Println()
	}
	fmt.Println("To apply these patches:")
	fmt.Println("  Run: rootio_patcher ruby remediate --dry-run=false")
}
