package apk

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rootio_patcher/pkg/rootio"
)

// fipsRunner is a MockRunner that answers `apk info -v` with the next of infoOut (the last repeats).
type fipsRunner struct {
	MockRunner
	infoOut []string
	queries int
}

func (f *fipsRunner) Output(_ context.Context, _ string, _ ...string) ([]byte, error) {
	i := min(f.queries, len(f.infoOut)-1)
	f.queries++
	return []byte(f.infoOut[i]), nil
}

func fipsPatch(name, version string) rootio.PackagePatch {
	return rootio.PackagePatch{PackageName: name, Patch: rootio.PatchInfo{Name: name, Version: version}}
}

func runFips(t *testing.T, runner *fipsRunner, patches []rootio.PackagePatch, dryRun bool) *App {
	t.Helper()
	return runFipsAlias(t, runner, patches, dryRun, false)
}

func runFipsAlias(t *testing.T, runner *fipsRunner, patches []rootio.PackagePatch, dryRun, useAlias bool) *App {
	t.Helper()
	scanner := &MockScanner{ListPackagesFunc: func(_ context.Context) ([]InstalledPackage, error) {
		return []InstalledPackage{{Name: "curl", Version: "8.5.0"}, {Name: "libcrypto3", Version: "3.5.0"}, {Name: "libssl3", Version: "3.5.0"}, {Name: "zlib", Version: "1.3"}, {Name: "musl", Version: "1.2"}}, nil
	}}
	api := &MockAPIClient{AnalyzeOsPackagesFunc: func(_ context.Context, _, _, _ string, _ []rootio.Package) (*rootio.OsAnalyzeResponse, error) {
		return &rootio.OsAnalyzeResponse{Patches: patches}, nil
	}}
	executor := NewExecutor("k", "https://pkg.root.io", logger(), runner)
	executor.fs = mockFS{}
	app := NewAppWithServices("k", "https://pkg.root.io", dryRun, useAlias, false, false, nil, logger(), scanner, api, executor)
	require.NoError(t, app.Run(context.Background()))
	return app
}

const staleFips = "curl-8.5.0-r00083\nlibcrypto3-3.5.0-r00083\nlibssl3-3.5.0-r00083\nzlib-1.3-r0\nmusl-1.2-r1\n"

func pinned(r *fipsRunner) bool {
	for _, c := range r.Calls {
		if c.Name == "apk" && strings.Contains(strings.Join(c.Args, " "), "=") && c.Args[0] == "add" && c.Args[1] != "--upgrade" {
			return true
		}
	}
	return false
}

// Stale FIPS curl only has a plain Root patch (kept), libcrypto3 has a newer FIPS patch (taken), neither
// is in the distro upgrade, and both are pinned in the world file.
func TestFips_KeepsInstalledFipsPackages(t *testing.T) {
	runner := &fipsRunner{infoOut: []string{staleFips}}
	var app *App
	out := captureStdout(t, func() {
		app = runFips(t, runner, []rootio.PackagePatch{
			fipsPatch("curl", "8.5.0-r00073"),
			fipsPatch("libcrypto3", "3.5.1-r00083"),
		}, false)
	})
	assert.Contains(t, out, "FIPS_KEPT 1: curl=8.5.0-r00073")

	assert.True(t, runner.calledWith("apk", "apk", "add", "--upgrade", "libcrypto3"))
	for _, c := range runner.Calls { // calledWith is a prefix match, so check every call
		if c.Name == "apk" && len(c.Args) > 1 && c.Args[1] == "--upgrade" {
			assert.NotContains(t, c.Args, "curl", "a plain Root patch must not replace the FIPS build")
			assert.NotContains(t, c.Args, "libssl3", "an installed FIPS package stays out of the distro upgrade")
		}
	}
	assert.True(t, runner.calledWith("apk", "apk", "add", "--upgrade", "musl", "zlib"), "only non-FIPS packages are upgraded")
	assert.True(t, runner.calledWith("apk", "apk", "add", "curl=8.5.0-r00083", "libcrypto3=3.5.0-r00083", "libssl3=3.5.0-r00083"))
	assert.Equal(t, "FIPS_KEPT 1: curl=8.5.0-r00073", app.executor.fipsSummary())
}

func TestFips_NoFipsPackagesWritesNothing(t *testing.T) {
	runner := &fipsRunner{infoOut: []string{"curl-8.5.0-r00073\nzlib-1.3-r0\n"}}
	app := runFips(t, runner, []rootio.PackagePatch{fipsPatch("curl", "8.5.0-r00083")}, false)
	assert.False(t, pinned(runner))
	assert.Empty(t, app.executor.fipsSummary())
}

func TestFips_DryRunWritesNothing(t *testing.T) {
	runner := &fipsRunner{infoOut: []string{staleFips}}
	runFips(t, runner, []rootio.PackagePatch{fipsPatch("libcrypto3", "3.5.1-r00083")}, true)
	assert.Empty(t, runner.Calls)
}

// With --use-alias the install path uses the alias name and version; a FIPS package must be kept there too.
func TestFips_AliasPatchDoesNotReplaceFipsBuild(t *testing.T) {
	runner := &fipsRunner{infoOut: []string{"rootio-curl-8.5.0-r00083\nzlib-1.3-r0\n"}} // FIPS build installed under the alias name
	aliased := fipsPatch("curl", "8.5.0-r00073")
	aliased.PatchAlias = rootio.PatchInfo{Name: "rootio-curl", Version: "8.5.0-r00073"}
	app := runFipsAlias(t, runner, []rootio.PackagePatch{aliased}, false, true)
	for _, c := range runner.Calls {
		assert.NotContains(t, c.Args, "rootio-curl")
	}
	assert.Equal(t, "FIPS_KEPT 1: curl=8.5.0-r00073", app.executor.fipsSummary())
}

func TestIsFips(t *testing.T) {
	for v, want := range map[string]bool{
		"8.5.0-r00083": true, "8.5.0-r10084": true,
		"8.5.0-r8": false, "8.5.0-r10": false, "8.5.0-r00073": false, "8.5.0-r0": false, "8.5.0-r0083": false, "8.5.0-r00083x": false,
	} {
		assert.Equal(t, want, isFips(v), v)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	fn()
	require.NoError(t, w.Close())
	os.Stdout = old
	b, _ := io.ReadAll(r)
	return string(b)
}
