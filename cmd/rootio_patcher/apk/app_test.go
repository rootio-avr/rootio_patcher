package apk

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rootio_patcher/pkg/rootio"
)

func logger() *slog.Logger { return slog.New(slog.NewTextHandler(os.Stdout, nil)) }

func alpineScanner() *MockScanner {
	return &MockScanner{
		DetectOSFunc: func(_ context.Context) (*OSInfo, error) {
			return &OSInfo{DistroVersion: "3.18"}, nil
		},
		ListPackagesFunc: func(_ context.Context) ([]InstalledPackage, error) {
			return []InstalledPackage{
				{Name: "curl", Version: "8.5.0"},
				{Name: "openssl", Version: "3.1.4"},
			}, nil
		},
	}
}

func newTestApp(scanner *MockScanner, apiClient *MockAPIClient, runner *MockRunner, dryRun bool) *App {
	return newTestAppFull(scanner, apiClient, runner, dryRun, true, false, nil)
}

func newTestAppFull(scanner *MockScanner, apiClient *MockAPIClient, runner *MockRunner, dryRun, useAlias, skipUpgrades bool, ignoreSet map[string]struct{}) *App {
	executor := NewExecutor("test-api-key", "https://pkg.root.io", logger(), runner)
	executor.fs = mockFS{}
	return NewAppWithServices(
		"test-api-key", "https://pkg.root.io",
		dryRun, useAlias, false, skipUpgrades, ignoreSet,
		logger(),
		scanner, apiClient, executor,
	)
}

// --- OS detection ---

func TestApp_Run_OSDetectionFailure(t *testing.T) {
	scanner := &MockScanner{
		DetectOSFunc: func(_ context.Context) (*OSInfo, error) {
			return nil, errors.New("not an alpine system")
		},
	}
	app := newTestApp(scanner, &MockAPIClient{}, &MockRunner{}, true)
	err := app.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OS detection failed")
}

// --- Empty package list ---

func TestApp_Run_NoPackages(t *testing.T) {
	scanner := &MockScanner{
		DetectOSFunc: func(_ context.Context) (*OSInfo, error) {
			return &OSInfo{DistroVersion: "3.18"}, nil
		},
		ListPackagesFunc: func(_ context.Context) ([]InstalledPackage, error) {
			return []InstalledPackage{}, nil
		},
	}
	apiClient := &MockAPIClient{
		AnalyzeOsPackagesFunc: func(_ context.Context, _, _, _ string, _ []rootio.Package) (*rootio.OsAnalyzeResponse, error) {
			t.Fatal("API should not be called when there are no packages")
			return nil, nil
		},
	}
	app := newTestApp(scanner, apiClient, &MockRunner{}, true)
	require.NoError(t, app.Run(context.Background()))
}

// --- Package scan failure ---

func TestApp_Run_PackageScanFailure(t *testing.T) {
	scanner := &MockScanner{
		DetectOSFunc: func(_ context.Context) (*OSInfo, error) {
			return &OSInfo{DistroVersion: "3.19"}, nil
		},
		ListPackagesFunc: func(_ context.Context) ([]InstalledPackage, error) {
			return nil, errors.New("apk info -v failed")
		},
	}
	app := newTestApp(scanner, &MockAPIClient{}, &MockRunner{}, true)
	err := app.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "package scan failed")
}

// --- API failure ---

func TestApp_Run_APIError(t *testing.T) {
	apiClient := &MockAPIClient{
		AnalyzeOsPackagesFunc: func(_ context.Context, _, _, _ string, _ []rootio.Package) (*rootio.OsAnalyzeResponse, error) {
			return nil, errors.New("connection refused")
		},
	}
	app := newTestApp(alpineScanner(), apiClient, &MockRunner{}, true)
	err := app.Run(context.Background())
	require.Error(t, err)
	assert.ErrorContains(t, err, "API call failed")
}

// --- Nothing to do ---

func TestApp_Run_NoPatches(t *testing.T) {
	runner := &MockRunner{}
	apiClient := &MockAPIClient{
		AnalyzeOsPackagesFunc: func(_ context.Context, _, _, _ string, _ []rootio.Package) (*rootio.OsAnalyzeResponse, error) {
			return &rootio.OsAnalyzeResponse{}, nil
		},
	}
	// SkipUpgrades=true so that with no patches there is genuinely nothing to do.
	app := newTestAppFull(alpineScanner(), apiClient, runner, false, true, true, nil)
	require.NoError(t, app.Run(context.Background()))
	assert.Empty(t, runner.Calls, "no commands should run when there is nothing to patch")
}

// --- API receives correct ecosystem + distro ---

func TestApp_Run_APICalledWithCorrectParams(t *testing.T) {
	var gotEndpoint, gotEcosystem, gotDistro string
	apiClient := &MockAPIClient{
		AnalyzeOsPackagesFunc: func(_ context.Context, endpoint, ecosystem, distroVersion string, _ []rootio.Package) (*rootio.OsAnalyzeResponse, error) {
			gotEndpoint = endpoint
			gotEcosystem = ecosystem
			gotDistro = distroVersion
			return &rootio.OsAnalyzeResponse{}, nil
		},
	}
	app := newTestApp(alpineScanner(), apiClient, &MockRunner{}, true)
	require.NoError(t, app.Run(context.Background()))
	assert.Equal(t, "apk", gotEndpoint)
	assert.Equal(t, "alpine", gotEcosystem)
	assert.Equal(t, "3.18", gotDistro)
}

// --- Dry-run: no commands executed ---

func TestApp_Run_DryRun_NoCommands(t *testing.T) {
	runner := &MockRunner{}
	apiClient := &MockAPIClient{
		AnalyzeOsPackagesFunc: func(_ context.Context, _, _, _ string, _ []rootio.Package) (*rootio.OsAnalyzeResponse, error) {
			return &rootio.OsAnalyzeResponse{
				Patches: []rootio.PackagePatch{
					{
						PackageName: "curl",
						Version:     "8.5.0-r0",
						PatchAlias:  rootio.PatchInfo{Name: "rootio-curl", Version: "8.5.0-r0.rootio.1"},
						CVEIDs:      []string{"CVE-2024-1234"},
					},
				},
				Upgradeable: []rootio.UpgradeableOsPackage{
					{PackageName: "openssl", CurrentVersion: "3.1.4-r5", UpgradeVersion: "3.1.5-r0"},
				},
			}, nil
		},
	}
	app := newTestApp(alpineScanner(), apiClient, runner, true)
	require.NoError(t, app.Run(context.Background()))
	assert.Empty(t, runner.Calls, "dry-run must not execute any commands")
}

// --- Apply: only upgrades (no Root.io repo setup) ---

func TestApp_Run_OnlyUpgrades(t *testing.T) {
	runner := &MockRunner{}
	spy := &spyFS{}
	// No patches: broad upgrade set is computed from installed packages (curl, openssl).
	apiClient := &MockAPIClient{
		AnalyzeOsPackagesFunc: func(_ context.Context, _, _, _ string, _ []rootio.Package) (*rootio.OsAnalyzeResponse, error) {
			return &rootio.OsAnalyzeResponse{}, nil
		},
	}
	executor := NewExecutor("test-api-key", "https://pkg.root.io", logger(), runner)
	executor.fs = spy
	app := NewAppWithServices("test-api-key", "https://pkg.root.io", false, true, false, false, nil, logger(), alpineScanner(), apiClient, executor)
	require.NoError(t, app.Run(context.Background()))

	assert.True(t, runner.calledWith("apk", "apk", "update"), "apk update must run")
	assert.True(t, runner.calledWith("apk", "apk", "add", "--upgrade", "curl", "openssl"), "must install upgrades")
	for _, p := range spy.OpenedPaths {
		assert.NotEqual(t, apkRepoFile, p, "Root.io repo must not be written for upgrades-only")
	}
}

// --- Apply: patches → Root.io repo is set up and cleaned up ---

func TestApp_Run_WithPatches_SetupAndCleanup(t *testing.T) {
	runner := &MockRunner{}
	apiClient := &MockAPIClient{
		AnalyzeOsPackagesFunc: func(_ context.Context, _, _, _ string, _ []rootio.Package) (*rootio.OsAnalyzeResponse, error) {
			return &rootio.OsAnalyzeResponse{
				Patches: []rootio.PackagePatch{
					{
						PackageName: "curl",
						Version:     "8.5.0-r0",
						PatchAlias:  rootio.PatchInfo{Name: "rootio-curl", Version: "8.5.0-r0.rootio.1"},
					},
				},
			}, nil
		},
	}
	app := newTestApp(alpineScanner(), apiClient, runner, false)
	require.NoError(t, app.Run(context.Background()))

	// apk update must run
	assert.True(t, runner.calledWith("apk", "apk", "update"), "apk update must run")
	// alias must be installed via apk add --upgrade
	assert.True(t, runner.calledWith("apk", "apk", "add", "--upgrade", "rootio-curl"), "alias must be installed")
	// broad upgrade runs by name (curl is patched, so only openssl remains)
	assert.True(t, runner.calledWith("apk", "apk", "add", "--upgrade", "openssl"), "non-patched package must be upgraded")
}

// --- Blacklisted package is skipped during upgrades ---

func TestApp_Run_BlacklistedPackageSkipped(t *testing.T) {
	runner := &MockRunner{}
	// The blacklist is now applied client-side in computeUpgradeSet against the
	// installed package list, so alpine-baselayout must be installed to exercise it.
	scanner := &MockScanner{
		DetectOSFunc: func(_ context.Context) (*OSInfo, error) {
			return &OSInfo{DistroVersion: "3.18"}, nil
		},
		ListPackagesFunc: func(_ context.Context) ([]InstalledPackage, error) {
			return []InstalledPackage{
				{Name: "alpine-baselayout", Version: "3.4.0-r0"},
				{Name: "openssl", Version: "3.1.4-r5"},
			}, nil
		},
	}
	apiClient := &MockAPIClient{
		AnalyzeOsPackagesFunc: func(_ context.Context, _, _, _ string, _ []rootio.Package) (*rootio.OsAnalyzeResponse, error) {
			return &rootio.OsAnalyzeResponse{}, nil
		},
	}
	app := newTestApp(scanner, apiClient, runner, false)
	require.NoError(t, app.Run(context.Background()))

	for _, c := range runner.Calls {
		if c.Name == "apk" {
			for _, a := range c.Args {
				assert.NotEqual(t, "alpine-baselayout", a, "blacklisted package must not be passed to apk")
			}
		}
	}
	assert.True(t, runner.calledWith("apk", "apk", "add", "--upgrade", "openssl"), "non-blacklisted upgrade must still run")
}

// --- Non-aliased: installs original names ---

func TestApp_Run_NonAliased_InstallsOriginalNames(t *testing.T) {
	runner := &MockRunner{}
	apiClient := &MockAPIClient{
		AnalyzeOsPackagesFunc: func(_ context.Context, _, _, _ string, _ []rootio.Package) (*rootio.OsAnalyzeResponse, error) {
			return &rootio.OsAnalyzeResponse{
				Patches: []rootio.PackagePatch{
					{
						PackageName: "curl",
						Version:     "8.5.0-r0",
						Patch:       rootio.PatchInfo{Name: "curl", Version: "8.5.0-r007"},
						PatchAlias:  rootio.PatchInfo{Name: "rootio-curl", Version: "8.5.0-r007"},
					},
					{
						PackageName: "openssl",
						Version:     "3.1.4-r5",
						Patch:       rootio.PatchInfo{Name: "openssl", Version: "3.1.4-r5007"},
						PatchAlias:  rootio.PatchInfo{Name: "rootio-openssl", Version: "3.1.4-r5007"},
					},
				},
			}, nil
		},
	}
	app := newTestAppFull(alpineScanner(), apiClient, runner, false, false, false, nil)
	require.NoError(t, app.Run(context.Background()))

	// Must install under original names
	assert.True(t, runner.calledWith("apk", "apk", "add", "--upgrade", "curl", "openssl"),
		"non-aliased must install original package names")

	// Must NOT install any rootio-* aliased name
	for _, c := range runner.Calls {
		for _, a := range c.Args {
			assert.False(t, strings.HasPrefix(a, "rootio-"), "non-aliased mode must not use rootio-* package names, got %q", a)
		}
	}
}

func TestApp_Run_NonAliased_DryRun_NoCommands(t *testing.T) {
	runner := &MockRunner{}
	apiClient := &MockAPIClient{
		AnalyzeOsPackagesFunc: func(_ context.Context, _, _, _ string, _ []rootio.Package) (*rootio.OsAnalyzeResponse, error) {
			return &rootio.OsAnalyzeResponse{
				Patches: []rootio.PackagePatch{
					{
						PackageName: "curl",
						Version:     "8.5.0-r0",
						Patch:       rootio.PatchInfo{Name: "curl", Version: "8.5.0-r007"},
						PatchAlias:  rootio.PatchInfo{Name: "rootio-curl", Version: "8.5.0-r007"},
					},
				},
			}, nil
		},
	}
	app := newTestAppFull(alpineScanner(), apiClient, runner, true, false, false, nil)
	require.NoError(t, app.Run(context.Background()))
	assert.Empty(t, runner.Calls, "dry-run must not execute any commands")
}

// recordingFS records WriteFile and Remove calls and serves a repositories file
// that contains both an official entry and a Root.io entry.
type recordingFS struct {
	mockFS
	written      map[string]string
	removed      []string
	repoContents string
}

func newRecordingFS() *recordingFS {
	return &recordingFS{
		written:      map[string]string{},
		repoContents: "https://dl-cdn.alpinelinux.org/alpine/v3.19/main\nhttps://root:key@pkg.root.io/alpine/3.19\n",
	}
}

func (r *recordingFS) ReadFile(_ string) ([]byte, error) { return []byte(r.repoContents), nil }
func (r *recordingFS) WriteFile(name string, data []byte, _ os.FileMode) error {
	r.written[name] = string(data)
	return nil
}
func (r *recordingFS) Remove(name string) error {
	r.removed = append(r.removed, name)
	return nil
}

// --- Executor: RemoveRootioRepo strips the Root.io line and refreshes the index ---

func TestExecutor_RemoveRootioRepo(t *testing.T) {
	runner := &MockRunner{}
	fs := newRecordingFS()
	e := NewExecutor("test-api-key", "https://pkg.root.io", logger(), runner)
	e.fs = fs

	require.NoError(t, e.RemoveRootioRepo(context.Background()))

	written, ok := fs.written[apkRepoFile]
	require.True(t, ok, "repositories file must be rewritten")
	assert.NotContains(t, written, apkRepoMark, "Root.io repo line must be removed")
	assert.Contains(t, written, "dl-cdn.alpinelinux.org", "official repo line must be preserved")
	assert.True(t, runner.calledWith("apk", "apk", "update"), "index must be refreshed")
	assert.NotContains(t, fs.removed, apkKeyPath, "RemoveRootioRepo must not remove the public key")
}

// --- Executor: Cleanup removes ONLY the public key, never the repo line ---

func TestExecutor_Cleanup_OnlyRemovesKey(t *testing.T) {
	runner := &MockRunner{}
	fs := newRecordingFS()
	e := NewExecutor("test-api-key", "https://pkg.root.io", logger(), runner)
	e.fs = fs

	require.NoError(t, e.Cleanup(context.Background()))

	assert.Equal(t, []string{apkKeyPath}, fs.removed, "Cleanup must remove only the public key")
	assert.Empty(t, fs.written, "Cleanup must not rewrite the repositories file")
}

// --- Executor: InstallUpgrades passes names straight through (no blacklist filtering) ---

func TestExecutor_InstallUpgrades_NoInternalFiltering(t *testing.T) {
	runner := &MockRunner{}
	e := NewExecutor("test-api-key", "https://pkg.root.io", logger(), runner)
	e.fs = mockFS{}
	// alpine-baselayout would previously be filtered here; that now happens upstream,
	// so whatever names are passed are installed verbatim.
	require.NoError(t, e.InstallUpgrades(context.Background(), []string{"alpine-baselayout", "openssl"}))
	assert.True(t, runner.calledWith("apk", "apk", "add", "--upgrade", "alpine-baselayout", "openssl"))
}

func TestExecutor_InstallUpgrades_EmptyIsNoop(t *testing.T) {
	runner := &MockRunner{}
	e := NewExecutor("test-api-key", "https://pkg.root.io", logger(), runner)
	e.fs = mockFS{}
	require.NoError(t, e.InstallUpgrades(context.Background(), nil))
	assert.Empty(t, runner.Calls, "empty upgrade set must run no commands")
}

// tmpFS opens a real temp file for OpenFile so addRepo's written line is readable.
type tmpFS struct {
	mockFS
	path string
}

func (t *tmpFS) OpenFile(_ string, _ int, _ os.FileMode) (*os.File, error) {
	return os.OpenFile(t.path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
}

// addRepo must embed credentials while PRESERVING the URL scheme: https stays
// https, http (local env) stays http — never force-upgraded or mis-parsed.
func TestExecutor_AddRepo_PreservesScheme(t *testing.T) {
	cases := map[string]string{
		"https://pkg.root.io/alpine/3.19":                             "https://root:test-api-key@pkg.root.io/alpine/3.19",
		"http://artrepo-service.artrepo.svc.cluster.local/alpine/3.19": "http://root:test-api-key@artrepo-service.artrepo.svc.cluster.local/alpine/3.19",
	}
	for registryURL, want := range cases {
		f, err := os.CreateTemp(t.TempDir(), "apkrepo")
		require.NoError(t, err)
		f.Close()
		e := NewExecutor("test-api-key", "https://pkg.root.io", logger(), &MockRunner{})
		e.fs = &tmpFS{path: f.Name()}
		require.NoError(t, e.addRepo(registryURL))
		got, err := os.ReadFile(f.Name())
		require.NoError(t, err)
		if strings.TrimSpace(string(got)) != want {
			t.Errorf("addRepo(%q) wrote %q, want %q", registryURL, strings.TrimSpace(string(got)), want)
		}
	}
}
