package pip

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/cmd/rootio_patcher/config"
	"rootio_patcher/cmd/rootio_patcher/upgradeplan"
	"rootio_patcher/pkg/rootio"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func writePlan(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func TestPipApp_PlannedPackageIsIgnoredForAPIAndPatchedVerbatim(t *testing.T) {
	planPath := filepath.Join(t.TempDir(), "plan.json")
	writePlan(t, planPath, `{"packages": [
	  {"ecosystem": "pip", "name": "jinja2", "action": "upgrade", "version": "3.1.4"},
	  {"ecosystem": "npm", "name": "flask", "action": "upgrade", "version": "9.9.9"}
	]}`)

	var gotPackages, gotIgnore []rootio.Package
	var applied []rootio.PackagePatch
	svc := &MockPipService{
		ListPackagesFunc: func(context.Context) ([]common.InstalledPackage, error) {
			return []common.InstalledPackage{{Name: "Jinja2", Version: "3.0.0"}, {Name: "flask", Version: "2.0.0"}}, nil
		},
		ApplyPatchFunc: func(_ context.Context, p rootio.PackagePatch) error {
			applied = append(applied, p)
			return nil
		},
	}
	client := &MockAPIClient{AnalyzePackagesFunc: func(_ context.Context, pkgs, ignore []rootio.Package, eco string) (*rootio.AnalyzePackagesResponse, error) {
		gotPackages, gotIgnore = pkgs, ignore
		require.Equal(t, "pypi", eco)
		return &rootio.AnalyzePackagesResponse{Patches: []rootio.PackagePatch{
			{PackageName: "flask", Version: "2.0.0", Patch: rootio.PatchInfo{Name: "flask", Version: "2.0.1+root.io.1"}},
		}}, nil
	}}

	app := NewAppWithServices(&config.Config{}, "python", false, nil, discardLogger(), svc, client,
		common.NewReporter("https://pkg.root.io", discardLogger())).WithUpgradePlan(planPath)
	require.NoError(t, app.Run(context.Background()))

	require.Len(t, gotPackages, 2)
	require.Equal(t, []rootio.Package{{Name: "Jinja2", Version: "3.0.0"}}, gotIgnore)
	require.Len(t, applied, 2)
	plannedInfo := rootio.PatchInfo{Name: "Jinja2", Version: "3.1.4"}
	require.Equal(t, rootio.PackagePatch{
		PackageName: "Jinja2", Version: "3.0.0", Planned: true,
		Patch: plannedInfo, PatchAlias: plannedInfo,
	}, applied[0])
	require.Equal(t, rootio.PackagePatch{
		PackageName: "flask", Version: "2.0.0",
		Patch: rootio.PatchInfo{Name: "flask", Version: "2.0.1+root.io.1"},
	}, applied[1])
}

func TestPipApp_ExplicitMissingPlanIsError(t *testing.T) {
	app := NewAppWithServices(&config.Config{}, "python", true, nil, discardLogger(), &MockPipService{
		ListPackagesFunc: func(context.Context) ([]common.InstalledPackage, error) {
			return []common.InstalledPackage{{Name: "django", Version: "4.0.0"}}, nil
		},
	}, &MockAPIClient{}, nil).WithUpgradePlan(filepath.Join(t.TempDir(), "missing.json"))

	err := app.Run(context.Background())
	require.ErrorIs(t, err, os.ErrNotExist)
	require.ErrorContains(t, err, "read upgrade plan")
}

func TestPipApp_DefaultPlanInWorkingDirectoryIsUsedAndMissingMeansNoPlan(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	var gotIgnore []rootio.Package
	run := func() error {
		app := NewAppWithServices(&config.Config{}, "python", true, nil, discardLogger(), &MockPipService{
			ListPackagesFunc: func(context.Context) ([]common.InstalledPackage, error) {
				return []common.InstalledPackage{{Name: "django", Version: "4.0.0"}}, nil
			},
		}, &MockAPIClient{AnalyzePackagesFunc: func(_ context.Context, _, ignore []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
			gotIgnore = ignore
			return &rootio.AnalyzePackagesResponse{}, nil
		}}, common.NewReporter("https://pkg.root.io", discardLogger()))
		return app.Run(context.Background())
	}

	require.NoError(t, run())
	require.Empty(t, gotIgnore)

	writePlan(t, filepath.Join(dir, upgradeplan.DefaultFileName),
		`{"packages": [{"ecosystem": "pip", "name": "django", "action": "upgrade", "version": "4.2.0"}]}`)
	require.ErrorIs(t, run(), common.ErrPatchesAvailable)
	require.Equal(t, []rootio.Package{{Name: "django", Version: "4.0.0"}}, gotIgnore)
}

// fakePython records every invocation's arguments, one line per call, and prints a site-packages path.
func fakePython(t *testing.T) (path string, calls func() []string) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")
	path = filepath.Join(dir, "python")
	script := "#!/bin/sh\necho \"$*\" >> " + log + "\necho " + dir + "\n"
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	return path, func() []string {
		data, err := os.ReadFile(log)
		require.NoError(t, err)
		return strings.Split(strings.TrimSpace(string(data)), "\n")
	}
}

func installArgs(t *testing.T, apply func(*PipService) error, python string, calls func() []string) string {
	t.Helper()
	svc := NewService(python, "https://pkg.root.io", "key", "https://idx.example/simple/", discardLogger())
	require.NoError(t, apply(svc))
	all := calls()
	return all[len(all)-1]
}

func TestPipService_ApplyPatchUsesArbitraryEqualityOnlyForPlannedPatches(t *testing.T) {
	cases := []struct {
		name    string
		planned bool
		want    string
	}{
		{"planned", true, "-m pip install --no-deps --no-cache-dir --index-url https://idx.example/simple/ jinja2===3.1.4"},
		{"api", false, "-m pip install --no-deps --no-cache-dir --index-url https://idx.example/simple/ jinja2==3.1.4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			python, calls := fakePython(t)
			patch := rootio.PackagePatch{PackageName: "Jinja2", Version: "3.0.0", Patch: rootio.PatchInfo{Name: "jinja2", Version: "3.1.4"}, Planned: tc.planned}
			got := installArgs(t, func(s *PipService) error { return s.ApplyPatch(context.Background(), patch) }, python, calls)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestPipService_ApplyPatchInstallsBeforeItRemovesAnything(t *testing.T) {
	index := "https://idx.example/simple/"
	install := "-m pip install --no-deps --no-cache-dir --index-url " + index + " idna===99.0.0"

	t.Run("missing version", func(t *testing.T) {
		python, calls := fakePythonExit(t, "install")
		svc := NewService(python, "https://pkg.root.io", "key", index, discardLogger())
		err := svc.ApplyPatch(context.Background(), rootio.PackagePatch{
			PackageName: "idna", Version: "3.4", Planned: true,
			Patch: rootio.PatchInfo{Name: "idna", Version: "99.0.0"},
		})
		require.ErrorContains(t, err, "install failed")
		require.Equal(t, []string{install}, calls())
	})

	t.Run("alias", func(t *testing.T) {
		python, calls := fakePython(t)
		svc := NewService(python, "https://pkg.root.io", "key", index, discardLogger())
		require.NoError(t, svc.ApplyPatch(context.Background(), rootio.PackagePatch{
			PackageName: "idna", Version: "3.4",
			Patch: rootio.PatchInfo{Name: "rootio-idna", Version: "3.4.0"},
		}))
		require.Equal(t, []string{
			"-m pip install --no-deps --no-cache-dir --index-url " + index + " rootio-idna==3.4.0",
			"-m pip uninstall -y idna",
		}, calls())
	})

	t.Run("same distribution", func(t *testing.T) {
		python, calls := fakePython(t)
		svc := NewService(python, "https://pkg.root.io", "key", index, discardLogger())
		require.NoError(t, svc.ApplyPatch(context.Background(), rootio.PackagePatch{
			PackageName: "typing_extensions", Version: "4.0.0", Planned: true,
			Patch: rootio.PatchInfo{Name: "Typing-Extensions", Version: "4.12.0"},
		}))
		require.Equal(t, []string{
			"-m pip install --no-deps --no-cache-dir --index-url " + index + " Typing-Extensions===4.12.0",
		}, calls())
	})
}

// fakePythonExit is fakePython that exits 1 when an argument word equals failOn.
func fakePythonExit(t *testing.T, failOn string) (path string, calls func() []string) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")
	path = filepath.Join(dir, "python")
	script := "#!/bin/sh\necho \"$*\" >> " + log + "\ncase \" $* \" in *\" " + failOn + " \"*) echo \"No matching distribution\" >&2; exit 1;; esac\necho " + dir + "\n"
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	return path, func() []string {
		data, err := os.ReadFile(log)
		require.NoError(t, err)
		return strings.Split(strings.TrimSpace(string(data)), "\n")
	}
}

func TestPipService_ApplyPatchForPipUsesArbitraryEqualityOnlyForPlannedPatches(t *testing.T) {
	cases := []struct {
		name    string
		planned bool
		want    string
	}{
		{"planned", true, "-m pip install --no-deps --no-cache-dir --upgrade --index-url https://idx.example/simple/ pip===24.0"},
		{"api", false, "-m pip install --no-deps --no-cache-dir --upgrade --index-url https://idx.example/simple/ pip==24.0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			python, calls := fakePython(t)
			patch := rootio.PackagePatch{PackageName: "pip", Version: "21.0", Patch: rootio.PatchInfo{Name: "pip", Version: "24.0"}, Planned: tc.planned}
			svc := NewService(python, "https://pkg.root.io", "key", "https://idx.example/simple/", discardLogger())
			require.NoError(t, svc.ApplyPatchForPip(context.Background(), patch))
			require.Equal(t, tc.want, calls()[0])
		})
	}
}

func TestPipApp_LogsEachPackageOnce(t *testing.T) {
	planPath := filepath.Join(t.TempDir(), "plan.json")
	writePlan(t, planPath, `{"packages": [
	  {"ecosystem": "pip", "name": "jinja2", "action": "upgrade", "version": "3.1.4"},
	  {"ecosystem": "pip", "name": "six", "action": "upgrade", "version": "1.16.0"},
	  {"ecosystem": "pip", "name": "absent", "action": "upgrade", "version": "1.0.0"}
	]}`)
	var logs strings.Builder
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	svc := &MockPipService{ListPackagesFunc: func(context.Context) ([]common.InstalledPackage, error) {
		return []common.InstalledPackage{{Name: "Jinja2", Version: "3.0.0"}, {Name: "six", Version: "1.16.0"}, {Name: "flask", Version: "2.0.0"}}, nil
	}}
	client := &MockAPIClient{AnalyzePackagesFunc: func(context.Context, []rootio.Package, []rootio.Package, string) (*rootio.AnalyzePackagesResponse, error) {
		return &rootio.AnalyzePackagesResponse{Patches: []rootio.PackagePatch{
			{PackageName: "flask", Version: "2.0.0", Patch: rootio.PatchInfo{Name: "flask", Version: "2.0.1"}},
		}}, nil
	}}
	app := NewAppWithServices(&config.Config{}, "python", false, nil, logger, svc, client, nil).WithUpgradePlan(planPath)
	require.NoError(t, app.Run(context.Background()))

	for _, line := range []string{
		`msg="Package upgraded" package=Jinja2 from=3.0.0 to=3.1.4 source=plan`,
		`msg="Package patched" package=flask from=2.0.0 to=2.0.1 source=api`,
		`msg="Package skipped" package=six from=1.16.0 to=1.16.0 source=plan reason="already at plan version"`,
		`msg="Package skipped" package=absent from="" to=1.0.0 source=plan reason="not installed"`,
	} {
		require.Equal(t, 1, strings.Count(logs.String(), line), logs.String())
	}
}
