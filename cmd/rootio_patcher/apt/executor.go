package apt

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"rootio_patcher/pkg/rootio"
)

const (
	gpgKeyPath     = "/etc/apt/keyrings/rootio.gpg"
	sourcesListDir = "/etc/apt/sources.list.d"
	prefsDir       = "/etc/apt/preferences.d"
	authConfDir    = "/etc/apt/auth.conf.d"

	// Base64-encoded dearmored GPG public key for the Root.io APT repository.
	// Keep in sync with backend/internal/constants/debian.go DebianGPGPublicKeyBase64.
	gpgPublicKeyBase64 = "mDMEackb4RYJKwYBBAHaRw8BAQdAXYXywiPx87Yh2coySuuX4GCwwW3HlQWh3+Tsp2uGp/e0JFJvb3QuaW8gQVBUIFJlcG9zaXRvcnkgPGFwdEByb290LmlvPoiZBBMWCgBBFiEEWXGkR0MAoq2axw+BRVs5xkCLj9MFAmnJG+ECGwMFCQPCZwAFCwkIBwICIgIGFQoJCAsCBBYCAwECHgcCF4AACgkQRVs5xkCLj9OkHQEArSfC4aoDw1WW6Xh6ygjDXeEaBU9fGsvwAPgyk6iRFeYBAIQAUOaC+RIQZah1237u7jHJlXpT1RMki/8qQk+SpKwCuDgEackb4RIKKwYBBAGXVQEFAQEHQOufOPOX57QLvxVMsvdFfz/4qYNxLV2tqM3EgRkkyatbAwEIB4h+BBgWCgAmFiEEWXGkR0MAoq2axw+BRVs5xkCLj9MFAmnJG+ECGwwFCQPCZwAACgkQRVs5xkCLj9N4zAEAt2tcEZclAYipGnc24DlcfY5ii7TCrwgy5S54nlCBrSEBAOGjU009/BJPXlu50wb2zRTuzKQ1vk+O+SVt0C3hcmMH"

	pkgRegistryBase = "https://pkg.root.io"
)

// lowLevelPackages require dpkg install to bypass apt's conflict resolver.
// Keep in sync with backend/.../os/debian.go lowLevelPackages.
var lowLevelPackages = map[string]bool{
	"rootio-util-linux": true,
	"rootio-libc6":      true,
}

// CommandRunner executes shell commands, streaming stdout/stderr to the terminal
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) error
}

type realRunner struct{}

func NewRealRunner() CommandRunner { return &realRunner{} }

func (r *realRunner) Run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = nil // inherit
	cmd.Stderr = nil
	return cmd.Run()
}

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
func (e *Executor) Setup(ctx context.Context, osInfo *OSInfo) error {
	registryURL := fmt.Sprintf("%s/%s/%s", pkgRegistryBase, osInfo.Ecosystem, osInfo.Codename)

	steps := []struct {
		desc string
		fn   func() error
	}{
		{"install GPG key", func() error { return e.installGPGKey(ctx) }},
		{"write auth config", func() error { return e.writeAuthConf(ctx) }},
		{"add APT source", func() error { return e.addSource(ctx, registryURL, osInfo.Codename) }},
		{"set pin priorities", func() error { return e.setPinPriority(ctx, registryURL) }},
		{"apt-get update", func() error { return e.AptUpdate(ctx) }},
	}

	for _, s := range steps {
		e.logf("→ %s", s.desc)
		if err := s.fn(); err != nil {
			return fmt.Errorf("%s: %w", s.desc, err)
		}
	}
	return nil
}

// InstallUpgrades installs packages available via official repo upgrades
func (e *Executor) InstallUpgrades(ctx context.Context, upgradeable []rootio.UpgradeableOsPackage) error {
	if len(upgradeable) == 0 {
		return nil
	}
	names := make([]string, len(upgradeable))
	for i, u := range upgradeable {
		names[i] = u.PackageName
	}
	e.logf("→ installing %d upgrade(s): %s", len(names), strings.Join(names, " "))
	args := append([]string{"install", "-y"}, names...)
	return e.runner.Run(ctx, "apt-get", args...)
}

// InstallPatches installs Root.io aliased packages and removes the originals they replace
func (e *Executor) InstallPatches(ctx context.Context, registryURL string, patches []rootio.PackagePatch) error {
	if len(patches) == 0 {
		return nil
	}

	// Block original packages from being pulled in from the Root.io registry
	for _, p := range patches {
		if err := e.blockOriginalFromRegistry(ctx, p.PackageName, registryURL); err != nil {
			return err
		}
	}

	var originals []string
	for _, p := range patches {
		alias := p.PatchAlias.Name
		e.logf("→ installing alias %s", alias)

		if lowLevelPackages[alias] {
			if err := e.installLowLevel(ctx, alias, p.PackageName); err != nil {
				return err
			}
		} else {
			if err := e.runner.Run(ctx, "apt-get",
				"-o", "Dpkg::Options::=--force-overwrite",
				"install", "--allow-remove-essential", "--no-install-recommends", "-y",
				alias,
			); err != nil {
				return fmt.Errorf("install %s: %w", alias, err)
			}
			originals = append(originals, p.PackageName)
		}
	}

	if len(originals) > 0 {
		e.logf("→ removing replaced originals: %s", strings.Join(originals, " "))
		args := append([]string{"remove", "-y", "--allow-remove-essential"}, originals...)
		if err := e.runner.Run(ctx, "env", append([]string{"SUDO_FORCE_REMOVE=yes", "apt-get"}, args...)...); err != nil {
			return fmt.Errorf("remove originals: %w", err)
		}
	}

	return nil
}

// Cleanup removes the Root.io APT repo files and clears apt caches
func (e *Executor) Cleanup(ctx context.Context) error {
	e.logf("→ cleanup")
	for _, cmd := range [][]string{
		{"rm", "-f", sourcesListDir + "/rootio.list"},
		{"rm", "-f", prefsDir + "/rootio"},
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
	script := fmt.Sprintf(`echo '%s' | base64 -d > %s`, gpgPublicKeyBase64, gpgKeyPath)
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
	script := fmt.Sprintf(
		`printf 'machine pkg.root.io\nlogin root\npassword %s\n' > %s/rootio.conf && chmod 600 %s/rootio.conf`,
		e.apiKey, authConfDir, authConfDir,
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
	host := strings.TrimPrefix(registryURL, "https://")
	host = strings.SplitN(host, "/", 2)[0]
	script := fmt.Sprintf(
		`echo 'Package: *\nPin: origin %s\nPin-Priority: 1001' > %s/rootio`,
		host, prefsDir,
	)
	return e.runner.Run(ctx, "sh", "-c", script)
}

// blockOriginalFromRegistry adds a -1 pin so the original package is never pulled from the Root.io registry
func (e *Executor) blockOriginalFromRegistry(ctx context.Context, pkgName, registryURL string) error {
	host := strings.TrimPrefix(registryURL, "https://")
	host = strings.SplitN(host, "/", 2)[0]
	script := fmt.Sprintf(
		`echo 'Package: %s\nPin: origin %s\nPin-Priority: -1' >> %s/rootio`,
		pkgName, host, prefsDir,
	)
	return e.runner.Run(ctx, "sh", "-c", script)
}

func (e *Executor) AptUpdate(ctx context.Context) error {
	return e.runner.Run(ctx, "apt-get", "update")
}

// installLowLevel uses dpkg to install a package that conflicts with apt's resolver
func (e *Executor) installLowLevel(ctx context.Context, alias, original string) error {
	steps := [][]string{
		{"apt-get", "download", alias},
		{"sh", "-c", fmt.Sprintf("dpkg -i --force-conflicts --force-overwrite %s_*.deb", alias)},
		{"dpkg", "--force-remove-essential", "--purge", original},
		{"apt-get", "install", "-f", "-y"},
		{"sh", "-c", fmt.Sprintf("rm %s_*.deb", alias)},
	}
	for _, s := range steps {
		if err := e.runner.Run(ctx, s[0], s[1:]...); err != nil {
			return fmt.Errorf("low-level install %s step %v: %w", alias, s, err)
		}
	}
	return nil
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
