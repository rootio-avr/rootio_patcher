package apt

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

func debianScanner() *MockScanner {
	return &MockScanner{
		DetectOSFunc: func(_ context.Context) (*OSInfo, error) {
			return &OSInfo{Ecosystem: "debian", DistroVersion: "12", Codename: "bookworm"}, nil
		},
		ListPackagesFunc: func(_ context.Context) ([]InstalledPackage, error) {
			return []InstalledPackage{
				{Name: "curl", Version: "7.88.1-10+deb12u5"},
				{Name: "openssl", Version: "3.0.11-1"},
			}, nil
		},
	}
}

func newTestApp(scanner *MockScanner, apiClient *MockAPIClient, runner *MockRunner, dryRun bool) *App {
	executor := NewExecutor("test-api-key", "https://pkg.root.io", false, runner)
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
			return nil, errors.New("not a debian system")
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
			return &OSInfo{Ecosystem: "debian", DistroVersion: "12", Codename: "bookworm"}, nil
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
	apiErr := errors.New("connection refused")
	apiClient := &MockAPIClient{
		AnalyzeOsPackagesFunc: func(_ context.Context, _, _, _ string, _ []rootio.Package) (*rootio.OsAnalyzeResponse, error) {
			return nil, apiErr
		},
	}
	app := newTestApp(debianScanner(), apiClient, &MockRunner{}, true)
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
	app := newTestApp(debianScanner(), apiClient, runner, true)
	require.NoError(t, app.Run(context.Background()))
	assert.Empty(t, runner.Calls, "no commands should run when there is nothing to patch")
}

// --- API receives correct ecosystem + distro ---

func TestApp_Run_APICalledWithCorrectEcosystem(t *testing.T) {
	var gotEcosystem, gotDistro string
	apiClient := &MockAPIClient{
		AnalyzeOsPackagesFunc: func(_ context.Context, _, ecosystem, distroVersion string, _ []rootio.Package) (*rootio.OsAnalyzeResponse, error) {
			gotEcosystem = ecosystem
			gotDistro = distroVersion
			return &rootio.OsAnalyzeResponse{}, nil
		},
	}
	scanner := &MockScanner{
		DetectOSFunc: func(_ context.Context) (*OSInfo, error) {
			return &OSInfo{Ecosystem: "ubuntu", DistroVersion: "22.04", Codename: "jammy"}, nil
		},
		ListPackagesFunc: func(_ context.Context) ([]InstalledPackage, error) {
			return []InstalledPackage{{Name: "curl", Version: "7.81.0-1ubuntu1.16"}}, nil
		},
	}
	app := newTestApp(scanner, apiClient, &MockRunner{}, true)
	require.NoError(t, app.Run(context.Background()))
	assert.Equal(t, "ubuntu", gotEcosystem)
	assert.Equal(t, "22.04", gotDistro)
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
						Version:     "7.88.1-10+deb12u5",
						PatchAlias:  rootio.PatchInfo{Name: "rootio-curl", Version: "7.88.1-10+deb12u5.root.io.1"},
						CVEIDs:      []string{"CVE-2024-1234"},
					},
				},
				Upgradeable: []rootio.UpgradeableOsPackage{
					{PackageName: "openssl", CurrentVersion: "3.0.11-1", UpgradeVersion: "3.0.15-1"},
				},
			}, nil
		},
	}
	app := newTestApp(debianScanner(), apiClient, runner, true /* dry-run */)
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
					{PackageName: "openssl", CurrentVersion: "3.0.11-1", UpgradeVersion: "3.0.15-1"},
				},
			}, nil
		},
	}
	app := newTestApp(debianScanner(), apiClient, runner, false)
	require.NoError(t, app.Run(context.Background()))

	assert.True(t, runner.calledWith("apt-get", "apt-get", "update"), "apt-get update must run")
	assert.True(t, runner.calledWith("apt-get", "apt-get", "install", "-y", "openssl"), "must install upgrade")
	// GPG key setup must NOT happen for upgrades-only
	for _, c := range runner.Calls {
		joined := c.Name + " " + strings.Join(c.Args, " ")
		assert.NotContains(t, joined, "rootio.list", "Root.io repo files must not be written for upgrades-only")
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
						Version:     "7.88.1-10+deb12u5",
						PatchAlias:  rootio.PatchInfo{Name: "rootio-curl", Version: "7.88.1-10+deb12u5.root.io.1"},
					},
				},
			}, nil
		},
	}
	app := newTestApp(debianScanner(), apiClient, runner, false)
	require.NoError(t, app.Run(context.Background()))

	// GPG key must be installed
	assert.True(t, runner.calledWith("sh"), "GPG key write via sh -c must run")
	// apt-get update must run
	assert.True(t, runner.calledWith("apt-get", "apt-get", "update"), "apt-get update must run")
	// alias must be installed
	assert.True(t, runner.calledWith("apt-get", "apt-get", "-o", "Dpkg::Options::=--force-overwrite", "install", "--allow-remove-essential", "--no-install-recommends", "-y", "rootio-curl"))
	// original curl must be removed
	assert.True(t, runner.calledWith("env"), "original package removal must run")
	// cleanup: repo list must be removed
	cleanupFiles := []string{
		sourcesListDir + "/rootio.list",
		prefsDir + "/rootio",
		gpgKeyPath,
		authConfDir + "/rootio.conf",
	}
	for _, f := range cleanupFiles {
		found := false
		for _, c := range runner.Calls {
			if strings.Join(append([]string{c.Name}, c.Args...), " ") == "rm -f "+f {
				found = true
				break
			}
		}
		assert.True(t, found, "cleanup must remove %s", f)
	}
}

// --- Apply: low-level package uses dpkg path ---

func TestApp_Run_LowLevelPackage_UsesDpkg(t *testing.T) {
	runner := &MockRunner{}
	apiClient := &MockAPIClient{
		AnalyzeOsPackagesFunc: func(_ context.Context, _, _, _ string, _ []rootio.Package) (*rootio.OsAnalyzeResponse, error) {
			return &rootio.OsAnalyzeResponse{
				Patches: []rootio.PackagePatch{
					{
						PackageName: "util-linux",
						Version:     "2.38.1-5",
						PatchAlias:  rootio.PatchInfo{Name: "rootio-util-linux", Version: "2.38.1-5.root.io.1"},
					},
				},
			}, nil
		},
	}
	app := newTestApp(debianScanner(), apiClient, runner, false)
	require.NoError(t, app.Run(context.Background()))

	// Must call apt-get download (not apt-get install) for the low-level package
	assert.True(t, runner.calledWith("apt-get", "apt-get", "download", "rootio-util-linux"))
	// Must use dpkg -i
	dpkgFound := false
	for _, c := range runner.Calls {
		if c.Name == "sh" && len(c.Args) >= 2 && strings.Contains(strings.Join(c.Args, " "), "dpkg -i") {
			dpkgFound = true
			break
		}
	}
	assert.True(t, dpkgFound, "dpkg -i must be used for low-level packages")
	// Must NOT call normal apt-get install for this package
	for _, c := range runner.Calls {
		if c.Name == "apt-get" && len(c.Args) > 0 && c.Args[0] == "install" {
			assert.NotContains(t, c.Args, "rootio-util-linux", "low-level package must not go through apt-get install")
		}
	}
}
