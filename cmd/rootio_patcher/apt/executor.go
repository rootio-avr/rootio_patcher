package apt

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/pkg/rootio"
)

// hostFromURL extracts the bare host[:port] from a package/registry URL,
// independent of scheme. Matches the url.Parse convention used by the golang/
// composer/pip remediators; a plain TrimPrefix("https://") mis-parses an
// http:// URL (leaving "http://host" as the host), which breaks the apt
// auth.conf machine match and pin origin. Falls back to the raw string on
// parse failure so behavior degrades to the old path rather than emptying.
func hostFromURL(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Host
	}
	h := strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "http://")
	return strings.SplitN(h, "/", 2)[0]
}

// authMachine returns the value for the apt auth.conf `machine` field.
// For https, apt matches on the bare host (machine pkg.root.io). For an
// unencrypted http repo, apt REFUSES to send credentials unless the machine
// line is annotated with the scheme (machine http://host) — otherwise it warns
// "Credentials ... match, but the protocol is not encrypted" and sends none,
// yielding a 401. So keep the scheme for http and strip it for https.
func authMachine(rawURL string) string {
	host := hostFromURL(rawURL)
	if strings.HasPrefix(rawURL, "http://") {
		return "http://" + host
	}
	return host
}

const (
	gpgKeyPath     = "/etc/apt/keyrings/rootio.gpg"
	sourcesListDir = "/etc/apt/sources.list.d"
	prefsDir       = "/etc/apt/preferences.d"
	authConfDir    = "/etc/apt/auth.conf.d"
)

// CommandRunner is an alias for common.CommandRunner
type CommandRunner = common.CommandRunner

func NewRealRunner() CommandRunner { return common.NewRealRunner() }

// Executor performs the actual apt remediation steps on the running system
type Executor struct {
	apiKey  string
	pkgURL  string
	runner  CommandRunner
	verbose bool
}

func NewExecutor(apiKey, pkgURL string, verbose bool, runner CommandRunner) *Executor {
	return &Executor{apiKey: apiKey, pkgURL: pkgURL, verbose: verbose, runner: runner}
}

// Setup installs the Root.io APT repository, GPG key, auth config, and pin preferences
func (e *Executor) Setup(ctx context.Context, registryURL string) error {
	codename := path.Base(registryURL)

	steps := []struct {
		desc string
		fn   func() error
	}{
		{"rewrite EOL sources", func() error { return e.rewriteSourcesForEOL(ctx, codename) }},
		{"install GPG key", func() error { return e.installGPGKey(ctx) }},
		{"write auth config", func() error { return e.writeAuthConf(ctx) }},
		{"add APT source", func() error { return e.addSource(ctx, registryURL, codename) }},
		{"set pin priorities", func() error { return e.setPinPriority(ctx, registryURL) }},
		{"apt-get update", func() error { return e.IndexUpdate(ctx) }},
	}

	for _, s := range steps {
		e.logf("→ %s", s.desc)
		if err := s.fn(); err != nil {
			return fmt.Errorf("%s: %w", s.desc, err)
		}
	}
	return nil
}

// InstallUpgrades installs the named packages from the official distro repository.
func (e *Executor) InstallUpgrades(ctx context.Context, names []string) error {
	if len(names) == 0 {
		return nil
	}
	e.logf("→ installing %d upgrade(s): %s", len(names), strings.Join(names, " "))
	args := append([]string{"install", "-y"}, names...)
	return e.runner.Run(ctx, "apt-get", args...)
}

// InstallPatches installs Root.io packages under their original names. The
// Root.io registry is pinned at priority 1001, so APT prefers it over upstream.
func (e *Executor) InstallPatches(ctx context.Context, _ string, patches []rootio.PackagePatch) error {
	if len(patches) == 0 {
		return nil
	}
	var names []string
	for _, p := range patches {
		names = append(names, p.Patch.Name)
		e.logf("→ installing %s", p.Patch.Name)
	}
	args := append([]string{"-o", "Dpkg::Options::=--force-overwrite", "install", "-y", "--allow-downgrades"}, names...)
	return e.runner.Run(ctx, "apt-get", args...)
}

// RemoveRootioRepo removes the Root.io APT source list and pin preferences, then
// refreshes the package index so subsequent upgrades resolve only against the
// official distro repo. `rm -f` is idempotent, but this is called exactly once.
func (e *Executor) RemoveRootioRepo(ctx context.Context) error {
	e.logf("→ removing Root.io repository")
	for _, cmd := range [][]string{
		{"rm", "-f", sourcesListDir + "/rootio.list"},
		{"rm", "-f", prefsDir + "/rootio"},
	} {
		if err := e.runner.Run(ctx, cmd[0], cmd[1:]...); err != nil {
			return fmt.Errorf("remove repo %v: %w", cmd, err)
		}
	}
	return e.IndexUpdate(ctx)
}

// Cleanup removes the remaining Root.io files (GPG key, auth config) and clears
// apt caches. The repo source list and pin are removed by RemoveRootioRepo.
func (e *Executor) Cleanup(ctx context.Context) error {
	e.logf("→ cleanup")
	for _, cmd := range [][]string{
		{"rm", "-f", gpgKeyPath},
		{"rm", "-f", authConfDir + "/rootio.conf"},
		{"rm", "-rf", "/var/lib/apt/lists/*"},
	} {
		if err := e.runner.Run(ctx, cmd[0], cmd[1:]...); err != nil {
			return fmt.Errorf("cleanup %v: %w", cmd, err)
		}
	}
	return nil
}

// installGPGKey decodes the base64 GPG key and writes it to gpgKeyPath
func (e *Executor) installGPGKey(ctx context.Context) error {
	if err := e.runner.Run(ctx, "mkdir", "-p", "/etc/apt/keyrings"); err != nil {
		return err
	}
	script := fmt.Sprintf(`echo '%s' | base64 -d > %s`, rootio.GPGPublicKeyBase64, gpgKeyPath)
	return e.runner.Run(ctx, "sh", "-c", script)
}

// writeAuthConf creates an APT auth config so pkg.root.io can authenticate
func (e *Executor) writeAuthConf(ctx context.Context) error {
	if e.apiKey == "" {
		return nil
	}
	if err := e.runner.Run(ctx, "mkdir", "-p", authConfDir); err != nil {
		return err
	}
	machine := authMachine(e.pkgURL)
	script := fmt.Sprintf(
		`printf 'machine %s\nlogin root\npassword %s\n' > %s/rootio.conf && chmod 600 %s/rootio.conf`,
		machine, e.apiKey, authConfDir, authConfDir,
	)
	return e.runner.Run(ctx, "sh", "-c", script)
}

// addSource writes the Root.io APT source list entry
func (e *Executor) addSource(ctx context.Context, registryURL, codename string) error {
	if err := e.runner.Run(ctx, "mkdir", "-p", sourcesListDir); err != nil {
		return err
	}
	line := fmt.Sprintf("deb [signed-by=%s] %s %s main", gpgKeyPath, registryURL, codename)
	return e.runner.Run(ctx, "sh", "-c", fmt.Sprintf("echo '%s' > %s/rootio.list", line, sourcesListDir))
}

// setPinPriority sets global 1001 pin-priority for all Root.io packages
func (e *Executor) setPinPriority(ctx context.Context, registryURL string) error {
	if err := e.runner.Run(ctx, "mkdir", "-p", prefsDir); err != nil {
		return err
	}
	host := hostFromURL(registryURL)
	script := fmt.Sprintf(
		`echo 'Package: *\nPin: origin %s\nPin-Priority: 1001' > %s/rootio`,
		host, prefsDir,
	)
	return e.runner.Run(ctx, "sh", "-c", script)
}

func (e *Executor) IndexUpdate(ctx context.Context) error {
	return e.runner.Run(ctx, "apt-get", "update")
}

// PostUpgradesOnly clears apt caches when only official upgrades were applied (no Root.io repo)
func (e *Executor) PostUpgradesOnly(ctx context.Context) error {
	return e.ClearAptCaches(ctx)
}

// ClearAptCaches removes only the apt lists cache (no repo files)
func (e *Executor) ClearAptCaches(ctx context.Context) error {
	return e.runner.Run(ctx, "rm", "-rf", "/var/lib/apt/lists/*")
}

func (e *Executor) logf(format string, args ...any) {
	if e.verbose {
		fmt.Printf("[apt] "+format+"\n", args...)
	}
}
