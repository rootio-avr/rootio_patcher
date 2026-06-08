package apk

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"rootio_patcher/pkg/rootio"
)

const (
	apkKeyPath  = "/etc/apk/keys/root@alpinelinux.org.rsa.pub"
	apkRepoFile = "/etc/apk/repositories"
	apkRepoMark = "pkg.root.io"
)

// packageBlacklist contains packages that must never be upgraded via apk.
// Keep in sync with backend/.../os/alpine.go.
var packageBlacklist = map[string]bool{
	"alpine-baselayout": true,
}

// CommandRunner executes shell commands, streaming stdout/stderr to the terminal
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) error
}

type realRunner struct{}

func NewRealRunner() CommandRunner { return &realRunner{} }

func (r *realRunner) Run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Executor performs the actual apk remediation steps on the running system
type Executor struct {
	apiKey  string
	pkgURL  string
	runner  CommandRunner
	fs      fileSystem
	verbose bool
}

func NewExecutor(apiKey, pkgURL string, verbose bool, runner CommandRunner) *Executor {
	return &Executor{apiKey: apiKey, pkgURL: pkgURL, verbose: verbose, runner: runner, fs: realFS{}}
}

// Setup installs the Root.io APK repository and public key
func (e *Executor) Setup(ctx context.Context, osInfo *OSInfo) error {
	registryURL := fmt.Sprintf("%s/alpine/%s", e.pkgURL, osInfo.DistroVersion)

	steps := []struct {
		desc string
		fn   func() error
	}{
		{"install public key", func() error { return e.installPublicKey() }},
		{"add APK repository", func() error { return e.addRepo(registryURL) }},
		{"apk update", func() error { return e.ApkUpdate(ctx) }},
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
	var names []string
	for _, u := range upgradeable {
		if packageBlacklist[u.PackageName] {
			e.logf("→ skipping blacklisted package %s", u.PackageName)
			continue
		}
		names = append(names, u.PackageName)
	}
	if len(names) == 0 {
		return nil
	}
	e.logf("→ installing %d upgrade(s): %s", len(names), strings.Join(names, " "))
	args := append([]string{"add", "--upgrade"}, names...)
	return e.runner.Run(ctx, "apk", args...)
}

// InstallPatches installs Root.io aliased packages via `apk add --upgrade`.
// APK handles replacement automatically via the `provides` mechanism.
func (e *Executor) InstallPatches(ctx context.Context, patches []rootio.PackagePatch) error {
	if len(patches) == 0 {
		return nil
	}
	var aliases []string
	for _, p := range patches {
		aliases = append(aliases, p.PatchAlias.Name)
	}
	e.logf("→ installing %d patch alias(es): %s", len(aliases), strings.Join(aliases, " "))
	args := append([]string{"add", "--upgrade"}, aliases...)
	return e.runner.Run(ctx, "apk", args...)
}

// Cleanup removes the Root.io APK repo entry and public key
func (e *Executor) Cleanup(_ context.Context) error {
	e.logf("→ cleanup")

	if err := e.removeRepoLine(apkRepoFile, apkRepoMark); err != nil {
		return fmt.Errorf("remove repo entry: %w", err)
	}

	if err := e.fs.Remove(apkKeyPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove public key: %w", err)
	}

	return nil
}

// removeRepoLine rewrites path omitting any line containing marker.
func (e *Executor) removeRepoLine(path, marker string) error {
	data, err := e.fs.ReadFile(path)
	if err != nil {
		return err
	}
	var kept []string
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		if !strings.Contains(sc.Text(), marker) {
			kept = append(kept, sc.Text())
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	content := strings.Join(kept, "\n")
	if len(kept) > 0 {
		content += "\n"
	}
	return e.fs.WriteFile(path, []byte(content), 0o644)
}

// ApkUpdate runs `apk update`
func (e *Executor) ApkUpdate(ctx context.Context) error {
	return e.runner.Run(ctx, "apk", "update")
}

// installPublicKey writes the Root.io RSA public key to /etc/apk/keys/
func (e *Executor) installPublicKey() error {
	if err := e.fs.MkdirAll("/etc/apk/keys", 0o755); err != nil {
		return err
	}
	keyBytes, err := base64.StdEncoding.DecodeString(rootio.APKPublicKeyBase64)
	if err != nil {
		return fmt.Errorf("decode public key: %w", err)
	}
	return e.fs.WriteFile(apkKeyPath, keyBytes, 0o644)
}

// addRepo appends the Root.io APK repository to /etc/apk/repositories
func (e *Executor) addRepo(registryURL string) error {
	authedURL := registryURL
	if e.apiKey != "" {
		host := strings.TrimPrefix(registryURL, "https://")
		authedURL = fmt.Sprintf("https://root:%s@%s", e.apiKey, host)
	}
	f, err := e.fs.OpenFile(apkRepoFile, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintln(f, authedURL)
	return err
}

func (e *Executor) logf(format string, args ...any) {
	if e.verbose {
		fmt.Printf("[apk] "+format+"\n", args...)
	}
}
