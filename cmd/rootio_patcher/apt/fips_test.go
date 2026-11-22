package apt

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rootio_patcher/pkg/rootio"
)

// fipsRunner is a MockRunner that answers dpkg-query with the next of dpkgOut (the last one repeats),
// like a Root FIPS base before and after the patches are installed.
type fipsRunner struct {
	MockRunner
	dpkgOut []string
	queries int
}

func (f *fipsRunner) Output(_ context.Context, name string, _ ...string) ([]byte, error) {
	if name != "dpkg-query" {
		return nil, nil
	}
	i := min(f.queries, len(f.dpkgOut)-1)
	f.queries++
	return []byte(f.dpkgOut[i]), nil
}

func (f *fipsRunner) pinWritten() bool {
	for _, c := range f.Calls {
		if c.Name == "sh" && strings.Contains(strings.Join(c.Args, " "), "rootio-fips-keep") {
			return true
		}
	}
	return false
}

func fipsPatch(name, version string) rootio.PackagePatch {
	return rootio.PackagePatch{PackageName: name, Patch: rootio.PatchInfo{Name: name, Version: version}}
}

func runFips(t *testing.T, runner *fipsRunner, patches []rootio.PackagePatch, dryRun bool) *App {
	t.Helper()
	scanner := &MockScanner{ListPackagesFunc: func(_ context.Context) ([]InstalledPackage, error) {
		return []InstalledPackage{{Name: "curl", Version: "1"}, {Name: "libsystemd0", Version: "1"}, {Name: "libudev1", Version: "1"}, {Name: "bash", Version: "1"}}, nil
	}}
	api := &MockAPIClient{AnalyzeOsPackagesFunc: func(_ context.Context, _, _, _ string, _ []rootio.Package) (*rootio.OsAnalyzeResponse, error) {
		return &rootio.OsAnalyzeResponse{Patches: patches}, nil
	}}
	app := NewAppWithServices("k", "https://pkg.root.io", dryRun, false, false, false, nil, logger(), scanner, api,
		NewExecutor("k", "https://pkg.root.io", false, runner))
	require.NoError(t, app.Run(context.Background()))
	return app
}

const staleFips = "curl 7.88.1-10+deb12u1.aikido.fips.9\nlibsystemd0 252.30-1~deb12u1.aikido.fips.3\nlibudev1 252.30-1~deb12u1.aikido.fips.3\nbash 5.2\n"

// Stale FIPS curl only has a non-FIPS patch (kept), libsystemd0 has a newer FIPS patch (taken), libudev1
// has no patch and must stay out of the distro upgrade. All three are pinned.
func TestFips_KeepsInstalledFipsPackages(t *testing.T) {
	runner := &fipsRunner{dpkgOut: []string{staleFips}}
	app := runFips(t, runner, []rootio.PackagePatch{
		fipsPatch("curl", "7.88.1-10+deb12u15.aikido.20"),
		fipsPatch("libsystemd0", "252.30-1~deb12u1.aikido.fips.5"),
	}, false)

	assert.True(t, runner.calledWith("apt-get", "apt-get", "-o", "Dpkg::Options::=--force-overwrite", "install", "-y", "--allow-downgrades", "--no-install-recommends", "libsystemd0"))
	assert.True(t, runner.calledWith("apt-get", "apt-get", "install", "-y", "--no-install-recommends", "bash"), "non-FIPS packages are upgraded")
	for _, c := range runner.Calls { // calledWith is a prefix match, so check the whole call list
		if c.Name == "apt-get" {
			assert.NotContains(t, c.Args, "libudev1", "installed FIPS package must stay out of the distro upgrade")
		}
	}
	assert.True(t, runner.calledWith("sh", "sh", "-c", `printf 'Package: curl libsystemd0 libudev1\nPin: release c=main\nPin-Priority: -1\n' > /etc/apt/preferences.d/rootio-fips-keep`))
	assert.Equal(t, "FIPS_KEPT 1: curl=7.88.1-10+deb12u15.aikido.20", app.executor.FipsSummary())
}

// curl is not FIPS before the patches and is after: the pin comes from the fresh list.
func TestFips_PinsPackageThatBecomesFips(t *testing.T) {
	runner := &fipsRunner{dpkgOut: []string{"curl 7.88.1-10+deb12u1\n", "curl 7.88.1-10+deb12u15.aikido.fips.22\n"}}
	runFips(t, runner, []rootio.PackagePatch{fipsPatch("curl", "7.88.1-10+deb12u15.aikido.fips.22")}, false)
	assert.True(t, runner.calledWith("sh", "sh", "-c", `printf 'Package: curl\nPin: release c=main\nPin-Priority: -1\n' > /etc/apt/preferences.d/rootio-fips-keep`))
}

func TestFips_NoFipsPackagesWritesNothing(t *testing.T) {
	runner := &fipsRunner{dpkgOut: []string{"curl 7.88.1-10+deb12u1\nbash 5.2\n"}}
	app := runFips(t, runner, []rootio.PackagePatch{fipsPatch("curl", "7.88.1-10+deb12u15.aikido.20")}, false)
	assert.False(t, runner.pinWritten())
	assert.Empty(t, app.executor.FipsSummary())
}

func TestFips_DryRunWritesNothing(t *testing.T) {
	runner := &fipsRunner{dpkgOut: []string{staleFips}}
	runFips(t, runner, []rootio.PackagePatch{fipsPatch("libsystemd0", "252.30-1~deb12u1.aikido.fips.5")}, true)
	assert.False(t, runner.pinWritten())
	assert.Empty(t, runner.Calls)
}
