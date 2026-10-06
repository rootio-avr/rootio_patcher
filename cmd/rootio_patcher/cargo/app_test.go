package cargo

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rootio_patcher/pkg/rootio"
)

type fakeAPI struct{ patches []rootio.PackagePatch }

func (f fakeAPI) AnalyzePackages(context.Context, []rootio.Package, []rootio.Package, string) (*rootio.AnalyzePackagesResponse, error) {
	return &rootio.AnalyzePackagesResponse{Patches: f.patches}, nil
}

type call struct {
	env  []string
	args []string
}

type fakeRunner struct {
	calls []call
	err   error
}

func (f *fakeRunner) Run(_ context.Context, _ string, env []string, _ string, args ...string) error {
	f.calls = append(f.calls, call{env, args})
	return f.err
}

const (
	manifestBefore = `[package]
name = "consumer" # keep me

[dependencies]
itoa = "1"

[patch.crates-io]
aikido-itoa-1-0-15 = { package = "itoa", version = "=1.0.15", registry = "aikido-itoa-1-0-15" }
aikido-ryu-1-0-20 = { package = "ryu", version = "=1.0.20", registry = "aikido-ryu-1-0-20" }
`
	configBefore = `[net]
retry = 3

[registries.aikido-itoa-1-0-15]
index = "sparse+https://pkg.root.io/cargo/itoa/1.0.15/aikido.2/"
credential-provider = "cargo:token"

[registries.aikido-ryu-1-0-20]
index = "sparse+https://pkg.root.io/cargo/ryu/1.0.20/aikido.1/"
credential-provider = "cargo:token"
`
)

// A project patched before: itoa at aikido.2 and ryu (from an earlier run). The API now offers itoa aikido.3.
func runItoaBump(t *testing.T, runner *fakeRunner) (string, error) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"Cargo.toml":         manifestBefore,
		".cargo/config.toml": configBefore,
		"Cargo.lock": `version = 4

[[package]]
name = "itoa"
version = "1.0.15+aikido.2"
source = "sparse+https://pkg.root.io/cargo/itoa/1.0.15/aikido.2/"
`,
	} {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	}
	api := fakeAPI{patches: []rootio.PackagePatch{{
		PackageName: "itoa", Version: "1.0.15+aikido.2",
		Patch: rootio.PatchInfo{Name: "itoa", Version: "1.0.15+aikido.3"},
	}}}
	app := NewApp("sk_test", "https://pkg.root.io/cargo/", dir, false, "", nil, slog.Default(), api, runner)
	return dir, app.Run(context.Background())
}

func read(t *testing.T, path string) string {
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

// A newer revision repoints the existing entries, keeps the user's content, and passes a token for every
// aikido registry (cargo update loads all registries in the lockfile, not just the patched one).
func TestApp_ReplacesStaleRevision(t *testing.T) {
	runner := &fakeRunner{}
	dir, err := runItoaBump(t, runner)
	require.NoError(t, err)

	assert.Equal(t, manifestBefore, read(t, filepath.Join(dir, "Cargo.toml")))
	assert.Equal(t, `[net]
retry = 3

[registries.aikido-ryu-1-0-20]
index = "sparse+https://pkg.root.io/cargo/ryu/1.0.20/aikido.1/"
credential-provider = "cargo:token"

[registries.aikido-itoa-1-0-15]
index = "sparse+https://pkg.root.io/cargo/itoa/1.0.15/aikido.3/"
credential-provider = "cargo:token"
`, read(t, filepath.Join(dir, ".cargo", "config.toml")))

	require.Len(t, runner.calls, 1)
	assert.Equal(t, []string{"update", "-p", "itoa@1.0.15+aikido.2"}, runner.calls[0].args)
	assert.Equal(t, []string{
		"CARGO_REGISTRIES_AIKIDO_ITOA_1_0_15_TOKEN=Basic cm9vdDpza190ZXN0",
		"CARGO_REGISTRIES_AIKIDO_RYU_1_0_20_TOKEN=Basic cm9vdDpza190ZXN0",
	}, runner.calls[0].env)
}

func TestApp_RestoresFilesWhenCargoUpdateFails(t *testing.T) {
	dir, err := runItoaBump(t, &fakeRunner{err: errors.New("boom")})
	require.ErrorContains(t, err, "Cargo files restored")

	assert.Equal(t, manifestBefore, read(t, filepath.Join(dir, "Cargo.toml")))
	assert.Equal(t, configBefore, read(t, filepath.Join(dir, ".cargo", "config.toml")))
}
