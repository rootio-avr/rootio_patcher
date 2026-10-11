package ruby

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/pkg/rootio"
)

type fakeAPI struct {
	resp *rootio.AnalyzePackagesResponse
}

func (f *fakeAPI) AnalyzePackages(_ context.Context, _, _ []rootio.Package, eco string) (*rootio.AnalyzePackagesResponse, error) {
	if eco != "ruby" {
		return nil, errors.New("wrong ecosystem " + eco)
	}
	return f.resp, nil
}

type fakeRunner struct {
	env     []string
	args    []string
	lockOut string
	err     error
	dir     string
}

func (f *fakeRunner) Run(_ context.Context, dir string, env []string, name string, args ...string) (string, error) {
	f.env, f.args = env, append([]string{name}, args...)
	if f.lockOut != "" {
		_ = os.WriteFile(filepath.Join(dir, "Gemfile.lock"), []byte(f.lockOut), 0o644)
	}
	return "boom secret-key", f.err
}

func setup(t *testing.T) string {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Gemfile"), []byte("source 'https://rubygems.org'\ngem 'rack'\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Gemfile.lock"), []byte(lock), 0o644))
	return dir
}

func resp() *rootio.AnalyzePackagesResponse {
	return &rootio.AnalyzePackagesResponse{Patches: []rootio.PackagePatch{{
		PackageName: "rack", Version: "2.2.3",
		Patch:      rootio.PatchInfo{Name: "rack", Version: "2.2.3.41610.1"},
		PatchAlias: rootio.PatchInfo{Name: "rack", Version: "2.2.3.41610.1"},
	}}}
}

func app(dir string, dry bool, r CommandRunner) *App {
	return NewApp("secret-key", "https://pkg.root.io", dir, dry, nil, slog.Default(), &fakeAPI{resp()}, r)
}

func TestRun_DryRunSignalsPatches(t *testing.T) {
	dir := setup(t)
	err := app(dir, true, &fakeRunner{}).Run(context.Background())
	assert.ErrorIs(t, err, common.ErrPatchesAvailable)
	b, _ := os.ReadFile(filepath.Join(dir, "Gemfile"))
	assert.NotContains(t, string(b), "41610")
}

func TestRun_AppliesWithoutTokenInGemfile(t *testing.T) {
	dir := setup(t)
	newLock := strings.Replace(lock, "rack (2.2.3)", "rack (2.2.3.41610.1)", 1)
	newLock = strings.Replace(newLock, "2.4.10", "2.5.0", 1)
	r := &fakeRunner{lockOut: newLock}
	require.NoError(t, app(dir, false, r).Run(context.Background()))

	gemfile, _ := os.ReadFile(filepath.Join(dir, "Gemfile"))
	assert.Contains(t, string(gemfile), "gem 'rack', '>= 2.2.3.41610.1', source: 'https://pkg.root.io/ruby'")
	assert.NotContains(t, string(gemfile), "secret-key")
	assert.Equal(t, []string{"bundle", "update", "rack", "--conservative"}, r.args)
	assert.Contains(t, r.env, "BUNDLE_PKG__ROOT__IO=root:secret-key")

	l, _ := os.ReadFile(filepath.Join(dir, "Gemfile.lock"))
	assert.Contains(t, string(l), "2.4.10") // BUNDLED WITH restored
}

func TestRun_BundleFailureRestoresFilesAndRedacts(t *testing.T) {
	dir := setup(t)
	err := app(dir, false, &fakeRunner{err: errors.New("exit 1")}).Run(context.Background())
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "secret-key")
	gemfile, _ := os.ReadFile(filepath.Join(dir, "Gemfile"))
	assert.Equal(t, "source 'https://rubygems.org'\ngem 'rack'\n", string(gemfile))
}

func TestRun_UnresolvedPatchIsError(t *testing.T) {
	dir := setup(t)
	err := app(dir, false, &fakeRunner{lockOut: lock}).Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolved to 2.2.3")
	l, _ := os.ReadFile(filepath.Join(dir, "Gemfile.lock"))
	assert.Equal(t, lock, string(l))
}
