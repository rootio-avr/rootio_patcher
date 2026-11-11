package apt

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rootio_patcher/pkg/rootio"
)

// holdRunner is a MockRunner that also answers read-only queries
// (apt-mark showhold, apt-get -s) like a Root FIPS bookworm base that holds
// libgnutls30 and libk5crypto3. Simulating libkrb5-3 reports a held change,
// as apt does when a patch needs a newer held dependency.
type holdRunner struct {
	MockRunner
	held    string
	queries []CommandCall
}

func (h *holdRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	h.queries = append(h.queries, CommandCall{Name: name, Args: args})
	if name == "apt-mark" {
		return []byte(h.held), nil
	}
	for _, a := range args {
		if a == "libkrb5-3" {
			return []byte("Reading package lists...\n" +
				"The following held packages will be changed:\n  libk5crypto3\n" +
				"The following packages will be upgraded:\n  libk5crypto3 libkrb5-3\n" +
				"2 upgraded, 0 newly installed, 0 to remove and 0 not upgraded.\n" +
				"Inst libk5crypto3 [1.20.1-2+deb12u5] (1.20.1-2+deb12u5.aikido.12 Root.io:bookworm [amd64])\n" +
				"Inst libkrb5-3 [1.20.1-2+deb12u5] (1.20.1-2+deb12u5.aikido.12 Root.io:bookworm [amd64])\n"), nil
		}
	}
	return []byte("Reading package lists...\n0 upgraded, 0 newly installed, 0 to remove and 0 not upgraded.\n"), nil
}

func patch(name string) rootio.PackagePatch {
	return rootio.PackagePatch{
		PackageName: name,
		Version:     "1",
		Patch:       rootio.PatchInfo{Name: name, Version: "1.aikido.1"},
		PatchAlias:  rootio.PatchInfo{Name: "rootio-" + name, Version: "1.root.io.1"},
	}
}

func heldScanner() *MockScanner {
	return &MockScanner{ListPackagesFunc: func(_ context.Context) ([]InstalledPackage, error) {
		var out []InstalledPackage
		for _, n := range []string{"curl", "libcurl4", "libgnutls30", "libk5crypto3", "libkrb5-3", "zlib1g"} {
			out = append(out, InstalledPackage{Name: n, Version: "1"})
		}
		return out, nil
	}}
}

func heldAPI() *MockAPIClient {
	return &MockAPIClient{AnalyzeOsPackagesFunc: func(_ context.Context, _, _, _ string, _ []rootio.Package) (*rootio.OsAnalyzeResponse, error) {
		return &rootio.OsAnalyzeResponse{Patches: []rootio.PackagePatch{
			patch("curl"), patch("libcurl4"), patch("libgnutls30"), patch("libk5crypto3"), patch("libkrb5-3"),
		}}, nil
	}}
}

func runHeld(t *testing.T, runner CommandRunner, useAlias, skipUpgrades bool) {
	t.Helper()
	e := NewExecutor("k", "https://pkg.root.io", false, runner)
	app := NewAppWithServices("k", "https://pkg.root.io", false, useAlias, false, skipUpgrades, nil, logger(), heldScanner(), heldAPI(), e)
	require.NoError(t, app.Run(context.Background()), "skipping held-package patches must not fail the run")
}

func installCalls(calls []CommandCall) [][]string {
	var out [][]string
	for _, c := range calls {
		if c.Name == "apt-get" && len(c.Args) > 0 && (c.Args[0] == "install" || (len(c.Args) > 2 && c.Args[2] == "install")) {
			out = append(out, c.Args)
		}
	}
	return out
}

// The cp-kafka FIPS failure: held packages and a patch that changes a held
// package are skipped; every other patch installs in one transaction.
func TestHeld_NonAliased_SkipsHeldAndHeldChangingPatches(t *testing.T) {
	r := &holdRunner{held: "libgnutls30\nlibk5crypto3\n"}
	runHeld(t, r, false, true)

	assert.True(t, r.calledWith("apt-get", "apt-get", "-o", "Dpkg::Options::=--force-overwrite", "install", "-y", "--allow-downgrades", "curl", "libcurl4"),
		"non-held patches must install exactly as today, minus the held ones; got %v", installCalls(r.Calls))
	for _, c := range r.Calls {
		for _, a := range c.Args {
			assert.NotEqual(t, "--allow-change-held-packages", a)
		}
		if c.Name == "apt-get" {
			assert.NotContains(t, c.Args, "libgnutls30")
			assert.NotContains(t, c.Args, "libk5crypto3")
			assert.NotContains(t, c.Args, "libkrb5-3")
		}
	}
}

func TestHeld_Aliased_SkipsHeldOriginal(t *testing.T) {
	r := &holdRunner{held: "libgnutls30\n"}
	runHeld(t, r, true, true)

	for _, c := range r.Calls {
		assert.NotContains(t, c.Args, "rootio-libgnutls30", "held package must not be replaced by its alias")
		if c.Name == "env" {
			assert.NotContains(t, c.Args, "libgnutls30", "held original must not be removed")
		}
	}
	assert.True(t, r.calledWith("apt-get", "apt-get", "-o", "Dpkg::Options::=--force-overwrite", "install", "--allow-remove-essential", "--no-install-recommends", "-y", "rootio-curl"))
}

func TestHeld_Upgrades_SkipHeldNames(t *testing.T) {
	r := &holdRunner{held: "zlib1g\n"}
	e := NewExecutor("k", "https://pkg.root.io", false, r)
	require.NoError(t, e.InstallUpgrades(context.Background(), []string{"bash", "zlib1g", "libkrb5-3", "tar"}))

	got := installCalls(r.Calls)
	require.Len(t, got, 1)
	assert.Equal(t, []string{"install", "-y", "bash", "tar"}, got[0])
}

// Must-not-change: with no held packages the commands run are identical to a
// runner that cannot query holds at all (today's path), and no simulation runs.
func TestHeld_NoHeldPackages_IdenticalCommands(t *testing.T) {
	for _, useAlias := range []bool{false, true} {
		plain := &MockRunner{}
		runHeld(t, plain, useAlias, false)

		withQuery := &holdRunner{held: "\n"}
		runHeld(t, withQuery, useAlias, false)

		assert.Equal(t, plain.Calls, withQuery.Calls, "useAlias=%v", useAlias)
		for _, q := range withQuery.queries {
			assert.Equal(t, "apt-mark", q.Name, "no apt-get -s may run without held packages: %v", q)
		}
	}
}

func TestHeldChanges_Parse(t *testing.T) {
	held := map[string]bool{"libgnutls30": true, "libk5crypto3": true, "libssl3": true}
	out := "The following held packages will be changed:\n  libgnutls30 libk5crypto3:amd64\n" +
		"The following packages will be upgraded:\n  curl libssl3\n" +
		"Inst curl [1] (2 x)\nRemv libssl3 [3]\n"
	assert.Equal(t, []string{"libgnutls30", "libk5crypto3", "libssl3"}, heldChanges([]byte(out), held))
	assert.Empty(t, heldChanges([]byte("Inst curl [1] (2 x)\n"), held))
	assert.Equal(t, "libc6", stripArch("libc6:amd64"))
}

// A held skip must be visible: a WARN record per skip with package, version
// and reason, and one HELD_SKIPPED summary line on stdout. Exit stays 0.
func TestHeld_SkipLogsWarnAndSummary(t *testing.T) {
	var logBuf bytes.Buffer
	lg := slog.New(slog.NewTextHandler(&logBuf, nil))
	r := &holdRunner{held: "libgnutls30\nlibk5crypto3\n"}
	e := NewExecutor("k", "https://pkg.root.io", false, r)
	app := NewAppWithServices("k", "https://pkg.root.io", false, false, false, true, nil, lg, heldScanner(), heldAPI(), e)

	stdout := os.Stdout
	rd, wr, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = wr
	runErr := app.Run(context.Background())
	os.Stdout = stdout
	require.NoError(t, wr.Close())
	out, err := io.ReadAll(rd)
	require.NoError(t, err)

	require.NoError(t, runErr, "held skips must not fail the run")
	logs := logBuf.String()
	assert.Contains(t, logs, `level=WARN msg="skipped patch: held package" ecosystem=apt package=libgnutls30 version=1.aikido.1 reason="held by the base image"`)
	assert.Contains(t, logs, `level=WARN msg="skipped patch: held package" ecosystem=apt package=libkrb5-3 version=1.aikido.1 reason="would change held package libk5crypto3"`)
	assert.Contains(t, string(out), "HELD_SKIPPED 3: libgnutls30=1.aikido.1,libk5crypto3=1.aikido.1,libkrb5-3=1.aikido.1\n")
}
