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
	return newTestAppWithAlias(scanner, apiClient, runner, dryRun, true)
}

func newTestAppWithAlias(scanner *MockScanner, apiClient *MockAPIClient, runner *MockRunner, dryRun, useAlias bool) *App {
	return newTestAppFull(scanner, apiClient, runner, dryRun, useAlias, false, nil)
}

func newTestAppFull(scanner *MockScanner, apiClient *MockAPIClient, runner *MockRunner, dryRun, useAlias, skipUpgrades bool, ignoreSet map[string]struct{}) *App {
	executor := NewExecutor("test-api-key", "https://pkg.root.io", false, runner)
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
	// SkipUpgrades=true so that with no patches there is genuinely nothing to do.
	app := newTestAppFull(debianScanner(), apiClient, runner, false, true, true, nil)
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
	// No patches: the broad upgrade set is computed from the installed packages
	// (curl, openssl) client-side.
	apiClient := &MockAPIClient{
		AnalyzeOsPackagesFunc: func(_ context.Context, _, _, _ string, _ []rootio.Package) (*rootio.OsAnalyzeResponse, error) {
			return &rootio.OsAnalyzeResponse{}, nil
		},
	}
	app := newTestApp(debianScanner(), apiClient, runner, false)
	require.NoError(t, app.Run(context.Background()))

	assert.True(t, runner.calledWith("apt-get", "apt-get", "update"), "apt-get update must run")
	// Broad upgrade installs all non-patched installed packages by name (sorted), no --allow-downgrades.
	assert.True(t, runner.calledWith("apt-get", "apt-get", "install", "-y", "curl", "openssl"), "must install upgrades")
	// GPG key / repo setup must NOT happen for upgrades-only
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
	// broad upgrade runs by name (curl is patched, so only openssl remains)
	assert.True(t, runner.calledWith("apt-get", "apt-get", "install", "-y", "openssl"), "broad upgrade must install non-patched packages")
	// repo source list + pin AND the remaining Root.io files must all be removed
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

	// Sequencing: RemoveRootioRepo (rm rootio.list) must come BEFORE the broad
	// upgrade install, which must come BEFORE final cleanup (rm gpg key).
	idxRepoRemoved := indexOfCall(runner.Calls, "rm", "-f", sourcesListDir+"/rootio.list")
	idxUpgrade := indexOfCall(runner.Calls, "apt-get", "install", "-y", "openssl")
	idxKeyRemoved := indexOfCall(runner.Calls, "rm", "-f", gpgKeyPath)
	require.GreaterOrEqual(t, idxRepoRemoved, 0)
	require.GreaterOrEqual(t, idxUpgrade, 0)
	require.GreaterOrEqual(t, idxKeyRemoved, 0)
	assert.Less(t, idxRepoRemoved, idxUpgrade, "repo must be removed before the broad upgrade")
	assert.Less(t, idxUpgrade, idxKeyRemoved, "broad upgrade must run before final cleanup")
}

// indexOfCall returns the index of the first runner call whose name+args exactly
// match, or -1 if none.
func indexOfCall(calls []CommandCall, name string, args ...string) int {
	want := append([]string{name}, args...)
	for i, c := range calls {
		got := append([]string{c.Name}, c.Args...)
		if len(got) != len(want) {
			continue
		}
		match := true
		for j := range want {
			if got[j] != want[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
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

// --- Non-aliased: installs original names, no alias dance ---

func TestApp_Run_NonAliased_InstallsOriginalNames(t *testing.T) {
	runner := &MockRunner{}
	apiClient := &MockAPIClient{
		AnalyzeOsPackagesFunc: func(_ context.Context, _, _, _ string, _ []rootio.Package) (*rootio.OsAnalyzeResponse, error) {
			return &rootio.OsAnalyzeResponse{
				Patches: []rootio.PackagePatch{
					{
						PackageName: "curl",
						Version:     "7.88.1-10+deb12u5",
						Patch:       rootio.PatchInfo{Name: "curl", Version: "7.88.1-10+deb12u5+root.io.1"},
						PatchAlias:  rootio.PatchInfo{Name: "rootio-curl", Version: "7.88.1-10+deb12u5.root.io.1"},
					},
					{
						PackageName: "openssl",
						Version:     "3.0.11-1",
						Patch:       rootio.PatchInfo{Name: "openssl", Version: "3.0.11-1+root.io.1"},
						PatchAlias:  rootio.PatchInfo{Name: "rootio-openssl", Version: "3.0.11-1.root.io.1"},
					},
				},
			}, nil
		},
	}
	app := newTestAppWithAlias(debianScanner(), apiClient, runner, false, false /* useAlias=false */)
	require.NoError(t, app.Run(context.Background()))

	// Must install under original names
	assert.True(t, runner.calledWith("apt-get", "apt-get", "install", "-y", "--allow-downgrades", "curl", "openssl"),
		"non-aliased must install original package names")

	// Must NOT install any rootio-* aliased name
	for _, c := range runner.Calls {
		for _, a := range c.Args {
			assert.False(t, strings.HasPrefix(a, "rootio-"), "non-aliased mode must not use rootio-* package names, got %q", a)
		}
	}
}

func TestApp_Run_NonAliased_NoRemoveOriginals(t *testing.T) {
	runner := &MockRunner{}
	apiClient := &MockAPIClient{
		AnalyzeOsPackagesFunc: func(_ context.Context, _, _, _ string, _ []rootio.Package) (*rootio.OsAnalyzeResponse, error) {
			return &rootio.OsAnalyzeResponse{
				Patches: []rootio.PackagePatch{
					{
						PackageName: "curl",
						Version:     "7.88.1-10+deb12u5",
						Patch:       rootio.PatchInfo{Name: "curl", Version: "7.88.1-10+deb12u5+root.io.1"},
						PatchAlias:  rootio.PatchInfo{Name: "rootio-curl", Version: "7.88.1-10+deb12u5.root.io.1"},
					},
				},
			}, nil
		},
	}
	app := newTestAppWithAlias(debianScanner(), apiClient, runner, false, false /* useAlias=false */)
	require.NoError(t, app.Run(context.Background()))

	// Non-aliased path must not remove originals (they are the installed packages)
	for _, c := range runner.Calls {
		if c.Name == "env" {
			joined := strings.Join(c.Args, " ")
			assert.NotContains(t, joined, "remove", "non-aliased path must not remove original packages")
		}
	}
}

func TestApp_Run_NonAliased_DryRun_ShowsOriginalNames(t *testing.T) {
	runner := &MockRunner{}
	apiClient := &MockAPIClient{
		AnalyzeOsPackagesFunc: func(_ context.Context, _, _, _ string, _ []rootio.Package) (*rootio.OsAnalyzeResponse, error) {
			return &rootio.OsAnalyzeResponse{
				Patches: []rootio.PackagePatch{
					{
						PackageName: "curl",
						Version:     "7.88.1-10+deb12u5",
						Patch:       rootio.PatchInfo{Name: "curl", Version: "7.88.1-10+deb12u5+root.io.1"},
						PatchAlias:  rootio.PatchInfo{Name: "rootio-curl", Version: "7.88.1-10+deb12u5.root.io.1"},
					},
				},
			}, nil
		},
	}
	app := newTestAppWithAlias(debianScanner(), apiClient, runner, true /* dry-run */, false /* useAlias=false */)
	require.NoError(t, app.Run(context.Background()))
	assert.Empty(t, runner.Calls, "dry-run must not execute any commands")
}

// --- Executor: RemoveRootioRepo removes repo + pin and refreshes the index ---

func TestExecutor_RemoveRootioRepo(t *testing.T) {
	runner := &MockRunner{}
	e := NewExecutor("test-api-key", "https://pkg.root.io", false, runner)
	require.NoError(t, e.RemoveRootioRepo(context.Background()))

	assert.True(t, runner.calledWith("rm", "rm", "-f", sourcesListDir+"/rootio.list"), "source list must be removed")
	assert.True(t, runner.calledWith("rm", "rm", "-f", prefsDir+"/rootio"), "pin must be removed")
	assert.True(t, runner.calledWith("apt-get", "apt-get", "update"), "index must be refreshed")
}

// --- Executor: Cleanup no longer removes the source list or pin ---

func TestExecutor_Cleanup_DoesNotRemoveRepoOrPin(t *testing.T) {
	runner := &MockRunner{}
	e := NewExecutor("test-api-key", "https://pkg.root.io", false, runner)
	require.NoError(t, e.Cleanup(context.Background()))

	assert.False(t, runner.calledWith("rm", "rm", "-f", sourcesListDir+"/rootio.list"), "Cleanup must not remove the source list")
	assert.False(t, runner.calledWith("rm", "rm", "-f", prefsDir+"/rootio"), "Cleanup must not remove the pin")
	assert.True(t, runner.calledWith("rm", "rm", "-f", gpgKeyPath), "Cleanup must remove the GPG key")
	assert.True(t, runner.calledWith("rm", "rm", "-f", authConfDir+"/rootio.conf"), "Cleanup must remove the auth config")
	assert.True(t, runner.calledWith("rm", "rm", "-rf", "/var/lib/apt/lists/*"), "Cleanup must clear apt lists")
}

// --- Executor: InstallUpgrades drops --allow-downgrades ---

func TestExecutor_InstallUpgrades_NoAllowDowngrades(t *testing.T) {
	runner := &MockRunner{}
	e := NewExecutor("test-api-key", "https://pkg.root.io", false, runner)
	require.NoError(t, e.InstallUpgrades(context.Background(), []string{"curl", "openssl"}))

	assert.True(t, runner.calledWith("apt-get", "apt-get", "install", "-y", "curl", "openssl"))
	for _, c := range runner.Calls {
		assert.NotContains(t, c.Args, "--allow-downgrades", "InstallUpgrades must not use --allow-downgrades")
	}
}

func TestExecutor_InstallUpgrades_EmptyIsNoop(t *testing.T) {
	runner := &MockRunner{}
	e := NewExecutor("test-api-key", "https://pkg.root.io", false, runner)
	require.NoError(t, e.InstallUpgrades(context.Background(), nil))
	assert.Empty(t, runner.Calls, "empty upgrade set must run no commands")
}
