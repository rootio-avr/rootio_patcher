package pip

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/cmd/rootio_patcher/config"
	"rootio_patcher/cmd/rootio_patcher/upgradeplan"
	"rootio_patcher/pkg/rootio"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func writePlan(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
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
		if eco != "pypi" {
			t.Errorf("ecosystem = %q, want pypi", eco)
		}
		return &rootio.AnalyzePackagesResponse{Patches: []rootio.PackagePatch{
			{PackageName: "flask", Version: "2.0.0", Patch: rootio.PatchInfo{Name: "flask", Version: "2.0.1+root.io.1"}},
		}}, nil
	}}

	app := NewAppWithServices(&config.Config{}, "python", false, nil, discardLogger(), svc, client,
		common.NewReporter("https://pkg.root.io", discardLogger())).WithUpgradePlan(planPath)
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(gotPackages) != 2 {
		t.Fatalf("API packages = %+v, want both installed packages", gotPackages)
	}
	if want := []rootio.Package{{Name: "Jinja2", Version: "3.0.0"}}; !reflect.DeepEqual(gotIgnore, want) {
		t.Fatalf("API ignore = %+v, want %+v", gotIgnore, want)
	}
	if len(applied) != 2 {
		t.Fatalf("applied = %+v, want planned and API patch", applied)
	}
	if p := applied[0]; p.PackageName != "Jinja2" || p.Patch.Name != "Jinja2" || p.Patch.Version != "3.1.4" || !p.Planned {
		t.Fatalf("planned patch = %+v", p)
	}
	if p := applied[1]; p.PackageName != "flask" || p.Patch.Version != "2.0.1+root.io.1" || p.Planned {
		t.Fatalf("api patch = %+v", p)
	}
}

func TestPipApp_ExplicitMissingPlanIsError(t *testing.T) {
	app := NewAppWithServices(&config.Config{}, "python", true, nil, discardLogger(), &MockPipService{
		ListPackagesFunc: func(context.Context) ([]common.InstalledPackage, error) {
			return []common.InstalledPackage{{Name: "django", Version: "4.0.0"}}, nil
		},
	}, &MockAPIClient{}, nil).WithUpgradePlan(filepath.Join(t.TempDir(), "missing.json"))

	err := app.Run(context.Background())
	if err == nil || !errors.Is(err, os.ErrNotExist) || !strings.Contains(err.Error(), "read upgrade plan") {
		t.Fatalf("err = %v, want read upgrade plan error", err)
	}
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

	if err := run(); err != nil || len(gotIgnore) != 0 {
		t.Fatalf("no default plan: err = %v ignore = %+v", err, gotIgnore)
	}

	writePlan(t, filepath.Join(dir, upgradeplan.DefaultFileName),
		`{"packages": [{"ecosystem": "pip", "name": "django", "action": "upgrade", "version": "4.2.0"}]}`)
	if err := run(); !errors.Is(err, common.ErrPatchesAvailable) {
		t.Fatalf("with default plan: err = %v, want ErrPatchesAvailable from the planned patch", err)
	}
	if want := []rootio.Package{{Name: "django", Version: "4.0.0"}}; !reflect.DeepEqual(gotIgnore, want) {
		t.Fatalf("ignore = %+v, want %+v", gotIgnore, want)
	}
}

// fakePython records every invocation's arguments, one line per call, and prints a site-packages path.
func fakePython(t *testing.T) (path string, calls func() []string) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")
	path = filepath.Join(dir, "python")
	script := "#!/bin/sh\necho \"$*\" >> " + log + "\necho " + dir + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path, func() []string {
		data, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		return strings.Split(strings.TrimSpace(string(data)), "\n")
	}
}

func installArgs(t *testing.T, apply func(*PipService) error, python string, calls func() []string) string {
	t.Helper()
	svc := NewService(python, "https://pkg.root.io", "key", "https://idx.example/simple/", discardLogger())
	if err := apply(svc); err != nil {
		t.Fatalf("apply: %v", err)
	}
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
			if got != tc.want {
				t.Fatalf("install args = %q, want %q", got, tc.want)
			}
		})
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
			if err := svc.ApplyPatchForPip(context.Background(), patch); err != nil {
				t.Fatalf("ApplyPatchForPip: %v", err)
			}
			if got := calls()[0]; got != tc.want {
				t.Fatalf("install args = %q, want %q", got, tc.want)
			}
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
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, line := range []string{
		`msg="Package upgraded" package=Jinja2 from=3.0.0 to=3.1.4 source=plan`,
		`msg="Package patched" package=flask from=2.0.0 to=2.0.1 source=api`,
		`msg="Package skipped" package=six from=1.16.0 to=1.16.0 source=plan reason="already at plan version"`,
		`msg="Package skipped" package=absent from="" to=1.0.0 source=plan reason="not installed"`,
	} {
		if got := strings.Count(logs.String(), line); got != 1 {
			t.Errorf("%q logged %d times, want 1:\n%s", line, got, logs.String())
		}
	}
}
