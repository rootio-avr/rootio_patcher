package npm

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/cmd/rootio_patcher/upgradeplan"
	"rootio_patcher/pkg/rootio"
)

func TestNpmApp_PlannedPackageIsIgnoredForAPIAndPatched(t *testing.T) {
	dir := t.TempDir()
	lockFile := writeMinimalLock(t, dir)
	writePlan(t, filepath.Join(dir, upgradeplan.DefaultFileName), `{
	  "packages": [
	    {"ecosystem": "npm", "name": "lodash", "action": "upgrade", "version": "4.17.21-aikido.2", "reason": "safe"},
	    {"ecosystem": "pypi", "name": "requests", "action": "upgrade", "version": "2.32.0"}
	  ]
	}`)

	var gotPackages, gotIgnore []rootio.Package
	apiCalls := 0
	app := newPlanApp(t, lockFile, true, &MockParser{
		ParseFunc: func(context.Context, string) ([]common.PackageInfo, error) {
			return []common.PackageInfo{
				{Name: "lodash", Version: "4.17.20"},
				{Name: "express", Version: "4.18.2"},
			}, nil
		},
	}, &MockAPIClient{
		AnalyzePackagesFunc: func(_ context.Context, packages, ignore []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
			apiCalls++
			gotPackages, gotIgnore = packages, ignore
			return &rootio.AnalyzePackagesResponse{}, nil
		},
	})

	err := app.Run(context.Background())
	if !errors.Is(err, common.ErrPatchesAvailable) {
		t.Fatalf("err = %v, want ErrPatchesAvailable from the planned patch", err)
	}
	if apiCalls != 1 {
		t.Fatalf("AnalyzePackages calls = %d, want 1", apiCalls)
	}
	if len(gotPackages) != 2 {
		t.Fatalf("API packages = %+v, want both installed packages", gotPackages)
	}
	if want := []rootio.Package{{Name: "lodash", Version: "4.17.20"}}; !reflect.DeepEqual(gotIgnore, want) {
		t.Fatalf("API ignore = %+v, want %+v", gotIgnore, want)
	}
}

func TestNpmApp_PlannedPackageKeepsUserIgnore(t *testing.T) {
	dir := t.TempDir()
	lockFile := writeMinimalLock(t, dir)
	writePlan(t, filepath.Join(dir, upgradeplan.DefaultFileName), `{
	  "packages": [{"ecosystem": "npm", "name": "lodash", "action": "upgrade", "version": "4.17.21"}]
	}`)

	var gotIgnore []rootio.Package
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	app := NewAppWithServices("test-key", "https://api.root.io", lockFile, true,
		map[string]struct{}{"debug@4.3.4": {}}, logger,
		&MockParser{ParseFunc: func(context.Context, string) ([]common.PackageInfo, error) {
			return []common.PackageInfo{{Name: "lodash", Version: "4.17.20"}, {Name: "debug", Version: "4.3.4"}}, nil
		}},
		&MockAPIClient{AnalyzePackagesFunc: func(_ context.Context, _, ignore []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
			gotIgnore = ignore
			return &rootio.AnalyzePackagesResponse{}, nil
		}})

	if err := app.Run(context.Background()); !errors.Is(err, common.ErrPatchesAvailable) {
		t.Fatalf("err = %v", err)
	}
	sort.Slice(gotIgnore, func(i, j int) bool { return gotIgnore[i].Name < gotIgnore[j].Name })
	want := []rootio.Package{{Name: "debug", Version: "4.3.4"}, {Name: "lodash", Version: "4.17.20"}}
	if !reflect.DeepEqual(gotIgnore, want) {
		t.Fatalf("API ignore = %+v, want %+v", gotIgnore, want)
	}
}

func TestNpmApp_PlanForUninstalledPackageIsSkipped(t *testing.T) {
	dir := t.TempDir()
	lockFile := writeMinimalLock(t, dir)
	writePlan(t, filepath.Join(dir, upgradeplan.DefaultFileName), `{
	  "packages": [{"ecosystem": "npm", "name": "ghost", "action": "upgrade", "version": "1.0.0"}]
	}`)

	apiCalls := 0
	app := newPlanApp(t, lockFile, false, &MockParser{
		ParseFunc: func(context.Context, string) ([]common.PackageInfo, error) {
			return []common.PackageInfo{{Name: "left-pad", Version: "1.3.0"}}, nil
		},
	}, &MockAPIClient{
		AnalyzePackagesFunc: func(context.Context, []rootio.Package, []rootio.Package, string) (*rootio.AnalyzePackagesResponse, error) {
			apiCalls++
			return &rootio.AnalyzePackagesResponse{}, nil
		},
	})

	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if apiCalls != 1 {
		t.Fatalf("AnalyzePackages calls = %d, want 1", apiCalls)
	}
}

func TestNpmApp_LeaveActionIsRejected(t *testing.T) {
	dir := t.TempDir()
	lockFile := writeMinimalLock(t, dir)
	writePlan(t, filepath.Join(dir, upgradeplan.DefaultFileName), `{
	  "packages": [{"ecosystem": "npm", "name": "left-pad", "action": "leave"}]
	}`)

	app := newPlanApp(t, lockFile, true, &MockParser{
		ParseFunc: func(context.Context, string) ([]common.PackageInfo, error) {
			return []common.PackageInfo{{Name: "left-pad", Version: "1.3.0"}}, nil
		},
	}, &MockAPIClient{
		AnalyzePackagesFunc: func(context.Context, []rootio.Package, []rootio.Package, string) (*rootio.AnalyzePackagesResponse, error) {
			t.Error("AnalyzePackages called")
			return &rootio.AnalyzePackagesResponse{}, nil
		},
	})

	err := app.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), `action "leave"`) {
		t.Fatalf("err = %v, want action leave error", err)
	}
}

func TestNpmApp_MissingPlanKeepsAPI(t *testing.T) {
	dir := t.TempDir()
	lockFile := writeMinimalLock(t, dir)

	apiCalls := 0
	app := newPlanApp(t, lockFile, true, &MockParser{
		ParseFunc: func(context.Context, string) ([]common.PackageInfo, error) {
			return []common.PackageInfo{{Name: "lodash", Version: "4.17.20"}}, nil
		},
	}, &MockAPIClient{
		AnalyzePackagesFunc: func(context.Context, []rootio.Package, []rootio.Package, string) (*rootio.AnalyzePackagesResponse, error) {
			apiCalls++
			return &rootio.AnalyzePackagesResponse{}, nil
		},
	})

	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("err = %v", err)
	}
	if apiCalls != 1 {
		t.Fatalf("AnalyzePackages calls = %d, want 1", apiCalls)
	}
}

func TestNpmApp_ExplicitMissingPlanErrors(t *testing.T) {
	dir := t.TempDir()
	lockFile := writeMinimalLock(t, dir)
	apiCalls := 0
	app := newPlanApp(t, lockFile, true, &MockParser{
		ParseFunc: func(context.Context, string) ([]common.PackageInfo, error) {
			return []common.PackageInfo{{Name: "lodash", Version: "4.17.20"}}, nil
		},
	}, &MockAPIClient{
		AnalyzePackagesFunc: func(context.Context, []rootio.Package, []rootio.Package, string) (*rootio.AnalyzePackagesResponse, error) {
			apiCalls++
			return nil, errors.New("API should not be called")
		},
	}).WithUpgradePlan(filepath.Join(dir, "no-such-plan.json"))

	err := app.Run(context.Background())
	if underlyingNotExist(err) == nil {
		t.Fatalf("err = %v, want not exist", err)
	}
	if apiCalls != 0 {
		t.Fatalf("AnalyzePackages calls = %d, want 0", apiCalls)
	}
}

func TestNpmApp_InvalidPlanErrors(t *testing.T) {
	dir := t.TempDir()
	lockFile := writeMinimalLock(t, dir)
	writePlan(t, filepath.Join(dir, upgradeplan.DefaultFileName), `{`)

	app := newPlanApp(t, lockFile, true, &MockParser{
		ParseFunc: func(context.Context, string) ([]common.PackageInfo, error) {
			return []common.PackageInfo{{Name: "lodash", Version: "4.17.20"}}, nil
		},
	}, &MockAPIClient{
		AnalyzePackagesFunc: func(context.Context, []rootio.Package, []rootio.Package, string) (*rootio.AnalyzePackagesResponse, error) {
			t.Error("AnalyzePackages called")
			return nil, errors.New("API should not be called")
		},
	})

	if err := app.Run(context.Background()); err == nil {
		t.Fatal("expected invalid plan error")
	}
}

func TestNpmApp_MergeAppliesPlanVersionAndAPIPatchTogether(t *testing.T) {
	dir := t.TempDir()
	lockFile := filepath.Join(dir, "package-lock.json")
	lock := `{
	  "name": "test",
	  "lockfileVersion": 3,
	  "packages": {
	    "": {"dependencies": {"lodash": "^4.17.20", "express": "4.18.2"}},
	    "node_modules/lodash": {"version": "4.17.20"},
	    "node_modules/express": {"version": "4.18.2"}
	  }
	}`
	if err := os.WriteFile(lockFile, []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}
	packageJSON := filepath.Join(dir, "package.json")
	manifest := `{
	  "name": "test",
	  "version": "1.0.0",
	  "dependencies": {
	    "lodash": "^4.17.20",
	    "express": "4.18.2"
	  }
	}`
	if err := os.WriteFile(packageJSON, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	writePlan(t, filepath.Join(dir, upgradeplan.DefaultFileName), `{
	  "packages": [
	    {"ecosystem": "npm", "name": "lodash", "action": "upgrade", "version": "4.17.21-aikido.2", "reason": "safe"}
	  ]
	}`)

	app := newPlanApp(t, lockFile, false, &MockNpmParser{
		NpmParser: NewNpmParser(),
		ParseFunc: func(context.Context, string) ([]common.PackageInfo, error) {
			return []common.PackageInfo{
				{Name: "lodash", Version: "4.17.20"},
				{Name: "express", Version: "4.18.2"},
			}, nil
		},
	}, &MockAPIClient{
		AnalyzePackagesFunc: func(_ context.Context, _, ignore []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
			for _, pkg := range ignore {
				if pkg.Name == "express" {
					t.Errorf("unplanned express must not be ignored, got %+v", ignore)
				}
			}
			return &rootio.AnalyzePackagesResponse{Patches: []rootio.PackagePatch{{
				PackageName: "express",
				Version:     "4.18.2",
				Patch:       rootio.PatchInfo{Name: "express", Version: "4.18.2-root.io.1"},
			}}}, nil
		},
	})

	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	updated, err := os.ReadFile(packageJSON)
	if err != nil {
		t.Fatal(err)
	}
	content := string(updated)
	if !strings.Contains(content, `"lodash": "4.17.21-aikido.2"`) {
		t.Errorf("package.json should install lodash at the exact plan version, got:\n%s", content)
	}
	if !strings.Contains(content, `"lodash@4.17.20": "4.17.21-aikido.2"`) {
		t.Errorf("package.json should override lodash@4.17.20 to the plan version, got:\n%s", content)
	}
	if !strings.Contains(content, `"express": "4.18.2-root.io.1"`) {
		t.Errorf("package.json should install express from the API patch, got:\n%s", content)
	}
}

func newPlanApp(t *testing.T, lockFile string, dryRun bool, parser npmParser, client *MockAPIClient) *App {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewAppWithServices("test-key", "https://api.root.io", lockFile, dryRun, nil, logger, parser, client)
}

func writeMinimalLock(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "package-lock.json")
	if err := os.WriteFile(path, []byte(`{"name":"test","lockfileVersion":3,"packages":{"":{}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writePlan(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func underlyingNotExist(err error) error {
	for err != nil {
		if os.IsNotExist(err) {
			return err
		}
		err = errors.Unwrap(err)
	}
	return nil
}
