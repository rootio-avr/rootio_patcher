package apt

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rootio_patcher/pkg/rootio"
)

// errRunner always returns the configured error from Run.
type errRunner struct{ err error }

func (r *errRunner) Run(_ context.Context, _ string, _ ...string) error { return r.err }

// recordingRunner captures every command passed to Run for later assertion.
// RunFunc, if set, overrides the default no-op-success behavior (used to stub
// out real binaries like dpkg that aren't available on the test host).
type recordingRunner struct {
	cmds    []recordedCmd
	RunFunc func(ctx context.Context, name string, args ...string) error
}

type recordedCmd struct {
	name string
	args []string
}

func (r *recordingRunner) Run(ctx context.Context, name string, args ...string) error {
	r.cmds = append(r.cmds, recordedCmd{name: name, args: args})
	if r.RunFunc != nil {
		return r.RunFunc(ctx, name, args...)
	}
	return nil
}

// fullCmd returns the full command string "name arg1 arg2 ..." for easy matching.
func (rc recordedCmd) fullCmd() string {
	parts := append([]string{rc.name}, rc.args...)
	return strings.Join(parts, " ")
}

// shScripts returns the sh -c script strings from all recorded "sh -c <script>" calls.
func shScripts(cmds []recordedCmd) []string {
	var out []string
	for _, c := range cmds {
		if c.name == "sh" && len(c.args) == 2 && c.args[0] == "-c" {
			out = append(out, c.args[1])
		}
	}
	return out
}

// ---- rewriteSourcesForEOL tests ----

func TestRewriteSourcesForEOL(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		codename      string
		wantScripts   []string // substrings that must appear in sh -c scripts
		wantCmdCount  int      // exact number of sh -c calls expected (0 = no-op)
	}{
		{
			codename: "buster",
			wantScripts: []string{
				// URL rewrites
				`test -f /etc/apt/sources.list && sed -i 's|http://deb.debian.org/debian|http://archive.debian.org/debian|g' /etc/apt/sources.list || true`,
				`test -f /etc/apt/sources.list && sed -i 's|http://deb.debian.org/debian-security|http://archive.debian.org/debian-security|g' /etc/apt/sources.list || true`,
				// drop -updates
				`test -f /etc/apt/sources.list && sed -i '/buster-updates/d' /etc/apt/sources.list || true`,
			},
			wantCmdCount: 3,
		},
		{
			codename: "stretch",
			wantScripts: []string{
				`test -f /etc/apt/sources.list && sed -i 's|http://deb.debian.org/debian|http://archive.debian.org/debian|g' /etc/apt/sources.list || true`,
				`test -f /etc/apt/sources.list && sed -i 's|http://deb.debian.org/debian-security|http://archive.debian.org/debian-security|g' /etc/apt/sources.list || true`,
				`test -f /etc/apt/sources.list && sed -i '/stretch-updates/d' /etc/apt/sources.list || true`,
			},
			wantCmdCount: 3,
		},
		{
			codename: "jessie",
			wantScripts: []string{
				`test -f /etc/apt/sources.list && sed -i 's|http://deb.debian.org/debian|http://archive.debian.org/debian|g' /etc/apt/sources.list || true`,
				`test -f /etc/apt/sources.list && sed -i 's|http://deb.debian.org/debian-security|http://archive.debian.org/debian-security|g' /etc/apt/sources.list || true`,
				`test -f /etc/apt/sources.list && sed -i '/jessie-updates/d' /etc/apt/sources.list || true`,
			},
			wantCmdCount: 3,
		},
		{
			codename: "oracular",
			wantScripts: []string{
				// old-format sources.list
				`test -f /etc/apt/sources.list && sed -i 's|http://archive.ubuntu.com/ubuntu|http://old-releases.ubuntu.com/ubuntu|g' /etc/apt/sources.list || true`,
				`test -f /etc/apt/sources.list && sed -i 's|http://security.ubuntu.com/ubuntu|http://old-releases.ubuntu.com/ubuntu|g' /etc/apt/sources.list || true`,
				// deb822 ubuntu.sources
				`test -f /etc/apt/sources.list.d/ubuntu.sources && sed -i 's|http://archive.ubuntu.com/ubuntu|http://old-releases.ubuntu.com/ubuntu|g' /etc/apt/sources.list.d/ubuntu.sources || true`,
				`test -f /etc/apt/sources.list.d/ubuntu.sources && sed -i 's|http://security.ubuntu.com/ubuntu|http://old-releases.ubuntu.com/ubuntu|g' /etc/apt/sources.list.d/ubuntu.sources || true`,
				// drop -updates
				`test -f /etc/apt/sources.list && sed -i '/oracular-updates/d' /etc/apt/sources.list || true`,
			},
			wantCmdCount: 5,
		},
		{
			codename: "mantic",
			wantScripts: []string{
				`test -f /etc/apt/sources.list && sed -i 's|http://archive.ubuntu.com/ubuntu|http://old-releases.ubuntu.com/ubuntu|g' /etc/apt/sources.list || true`,
				`test -f /etc/apt/sources.list && sed -i 's|http://security.ubuntu.com/ubuntu|http://old-releases.ubuntu.com/ubuntu|g' /etc/apt/sources.list || true`,
				`test -f /etc/apt/sources.list.d/ubuntu.sources && sed -i 's|http://archive.ubuntu.com/ubuntu|http://old-releases.ubuntu.com/ubuntu|g' /etc/apt/sources.list.d/ubuntu.sources || true`,
				`test -f /etc/apt/sources.list.d/ubuntu.sources && sed -i 's|http://security.ubuntu.com/ubuntu|http://old-releases.ubuntu.com/ubuntu|g' /etc/apt/sources.list.d/ubuntu.sources || true`,
				`test -f /etc/apt/sources.list && sed -i '/mantic-updates/d' /etc/apt/sources.list || true`,
			},
			wantCmdCount: 5,
		},
		{
			// Non-EOL codename — must be a pure no-op
			codename:     "bookworm",
			wantScripts:  nil,
			wantCmdCount: 0,
		},
		{
			// Another non-EOL codename
			codename:     "noble",
			wantScripts:  nil,
			wantCmdCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.codename, func(t *testing.T) {
			rec := &recordingRunner{}
			exec := NewExecutor("", "", false, rec)

			err := exec.rewriteSourcesForEOL(ctx, tc.codename)
			require.NoError(t, err)

			scripts := shScripts(rec.cmds)
			assert.Len(t, scripts, tc.wantCmdCount,
				"unexpected number of sh -c commands for codename %q", tc.codename)

			for _, want := range tc.wantScripts {
				assert.Contains(t, scripts, want,
					"expected script not found for codename %q", tc.codename)
			}
		})
	}
}

// ---- Setup integration: EOL rewrite runs first ----

func TestSetup_EOLRewriteRunsFirst(t *testing.T) {
	ctx := context.Background()
	rec := &recordingRunner{}
	exec := NewExecutor("apikey", "https://pkg.root.io", false, rec)

	// Use a buster registry URL so codename == "buster"
	_ = exec.Setup(ctx, "https://pkg.root.io/buster")

	// Find the first sh -c script issued
	var firstScript string
	for _, c := range rec.cmds {
		if c.name == "sh" && len(c.args) == 2 && c.args[0] == "-c" {
			firstScript = c.args[1]
			break
		}
	}

	require.NotEmpty(t, firstScript, "expected at least one sh -c call")
	assert.True(t,
		strings.Contains(firstScript, "archive.debian.org"),
		fmt.Sprintf("first sh -c script should be an EOL rewrite, got: %q", firstScript),
	)
}

// TestRewriteSourcesForEOL_PropagatesRunnerError verifies that a runner failure
// during an EOL rewrite is propagated rather than swallowed.
func TestRewriteSourcesForEOL_PropagatesRunnerError(t *testing.T) {
	sentinel := errors.New("boom")
	exec := NewExecutor("", "", false, &errRunner{err: sentinel})

	err := exec.rewriteSourcesForEOL(context.Background(), "buster")

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

func TestHostFromURL(t *testing.T) {
	cases := map[string]string{
		"https://pkg.root.io":                                  "pkg.root.io",
		"https://pkg.root.local/debian":                        "pkg.root.local",
		"http://artrepo-service.artrepo.svc.cluster.local":     "artrepo-service.artrepo.svc.cluster.local",
		"http://artrepo-service.artrepo.svc.cluster.local/deb": "artrepo-service.artrepo.svc.cluster.local",
		"http://localhost:8080/debian":                         "localhost:8080",
	}
	for in, want := range cases {
		if got := hostFromURL(in); got != want {
			t.Errorf("hostFromURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// ---- InstallPatches (non-aliased) version-downgrade guard ----

// stubDpkgCompare fakes `dpkg --compare-versions <a> lt <b>` using Go's own
// string comparison on the fake test versions, so tests don't depend on dpkg
// being installed on the test host.
func stubDpkgCompare(olderVersions map[string]bool) func(ctx context.Context, name string, args ...string) error {
	return func(_ context.Context, name string, args ...string) error {
		if name == "dpkg" && len(args) == 4 && args[0] == "--compare-versions" && args[2] == "lt" {
			if olderVersions[args[1]] {
				return nil // exit 0: true, candidate is older
			}
			return errors.New("not older") // exit non-zero: false
		}
		return nil
	}
}

// aptCmds filters out the dpkg --compare-versions probe calls, leaving only
// the actual apt-get invocations for assertion.
func aptCmds(cmds []recordedCmd) []recordedCmd {
	var out []recordedCmd
	for _, c := range cmds {
		if c.name != "dpkg" {
			out = append(out, c)
		}
	}
	return out
}

func TestInstallPatches_NonAliased_SkipsDowngrade(t *testing.T) {
	rec := &recordingRunner{RunFunc: stubDpkgCompare(map[string]bool{
		"3.5.6-1~deb13u2.aikido.2": true, // older than installed 3.5.7-1~deb13u2
	})}
	exec := NewExecutor("", "", false, rec)

	patches := []rootio.PackagePatch{
		// Installed is newer than the Root.io candidate: must be skipped.
		{PackageName: "openssl", Version: "3.5.7-1~deb13u2", Patch: rootio.PatchInfo{Name: "openssl", Version: "3.5.6-1~deb13u2.aikido.2"}},
		// Installed is older than the Root.io candidate: must be installed.
		{PackageName: "gzip", Version: "1.13-1+deb13u1", Patch: rootio.PatchInfo{Name: "gzip", Version: "1.13-1.aikido.3"}},
	}

	err := exec.InstallPatches(context.Background(), "https://pkg.root.io/debian/trixie", patches, false)
	require.NoError(t, err)

	cmds := aptCmds(rec.cmds)
	require.Len(t, cmds, 1, "expected exactly one apt-get invocation")
	cmd := cmds[0].fullCmd()
	assert.NotContains(t, cmd, "openssl", "downgrade candidate must not be installed")
	assert.Contains(t, cmd, "gzip", "upgrade candidate must still be installed")
	assert.NotContains(t, cmd, "--allow-downgrades", "guard makes --allow-downgrades unnecessary")
}

func TestInstallPatches_NonAliased_AllDowngrades_NoOp(t *testing.T) {
	rec := &recordingRunner{RunFunc: stubDpkgCompare(map[string]bool{
		"3.5.6-1~deb13u2.aikido.2": true,
	})}
	exec := NewExecutor("", "", false, rec)

	patches := []rootio.PackagePatch{
		{PackageName: "openssl", Version: "3.5.7-1~deb13u2", Patch: rootio.PatchInfo{Name: "openssl", Version: "3.5.6-1~deb13u2.aikido.2"}},
	}

	err := exec.InstallPatches(context.Background(), "https://pkg.root.io/debian/trixie", patches, false)
	require.NoError(t, err)
	assert.Empty(t, aptCmds(rec.cmds), "no apt-get call expected when every candidate is a downgrade")
}

func TestAuthMachine(t *testing.T) {
	cases := map[string]string{
		"https://pkg.root.io":                              "pkg.root.io",
		"https://pkg.root.local/debian":                    "pkg.root.local",
		"http://artrepo-service.artrepo.svc.cluster.local": "http://artrepo-service.artrepo.svc.cluster.local",
		"http://localhost:8080/debian":                     "http://localhost:8080",
	}
	for in, want := range cases {
		if got := authMachine(in); got != want {
			t.Errorf("authMachine(%q) = %q, want %q", in, got, want)
		}
	}
}
