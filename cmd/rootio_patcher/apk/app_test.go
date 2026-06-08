package apk

import (
	"context"
	"errors"
	"log/slog"
	"os"
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
				{Name: "curl", Version: "8.5.0-r0"},
				{Name: "openssl", Version: "3.1.4-r5"},
			}, nil
		},
	}
}

func newTestApp(scanner *MockScanner, apiClient *MockAPIClient, runner *MockRunner, dryRun bool) *App {
	executor := NewExecutor("test-api-key", "https://pkg.root.io", false, runner)
	executor.fs = mockFS{}
	return NewAppWithServices(
		"test-api-key", "https://pkg.root.io",
		dryRun, false,
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
	app := newTestApp(alpineScanner(), apiClient, runner, true)
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
	apiClient := &MockAPIClient{
		AnalyzeOsPackagesFunc: func(_ context.Context, _, _, _ string, _ []rootio.Package) (*rootio.OsAnalyzeResponse, error) {
			return &rootio.OsAnalyzeResponse{
				Upgradeable: []rootio.UpgradeableOsPackage{
					{PackageName: "openssl", CurrentVersion: "3.1.4-r5", UpgradeVersion: "3.1.5-r0"},
				},
			}, nil
		},
	}
	app := newTestApp(alpineScanner(), apiClient, runner, false)
	require.NoError(t, app.Run(context.Background()))

	assert.True(t, runner.calledWith("apk", "apk", "update"), "apk update must run")
	assert.True(t, runner.calledWith("apk", "apk", "add", "--upgrade", "openssl"), "must install upgrade")
	// Root.io repo must NOT be set up for upgrades-only
	for _, c := range runner.Calls {
		if c.Name == "sh" {
			for _, a := range c.Args {
				assert.NotContains(t, a, apkRepoMark, "Root.io repo must not be added for upgrades-only")
			}
		}
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
}

// --- Blacklisted package is skipped during upgrades ---

func TestApp_Run_BlacklistedPackageSkipped(t *testing.T) {
	runner := &MockRunner{}
	apiClient := &MockAPIClient{
		AnalyzeOsPackagesFunc: func(_ context.Context, _, _, _ string, _ []rootio.Package) (*rootio.OsAnalyzeResponse, error) {
			return &rootio.OsAnalyzeResponse{
				Upgradeable: []rootio.UpgradeableOsPackage{
					{PackageName: "alpine-baselayout", CurrentVersion: "3.4.0-r0", UpgradeVersion: "3.5.0-r0"},
					{PackageName: "openssl", CurrentVersion: "3.1.4-r5", UpgradeVersion: "3.1.5-r0"},
				},
			}, nil
		},
	}
	app := newTestApp(alpineScanner(), apiClient, runner, false)
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
