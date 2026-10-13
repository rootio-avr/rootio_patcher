package npm

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/cmd/rootio_patcher/upgradeplan"
	"rootio_patcher/pkg/rootio"
)

func TestNpmApp_UpgradePlanSkipsAPI(t *testing.T) {
	dir := t.TempDir()
	lockFile := writeMinimalLock(t, dir)
	writePlan(t, filepath.Join(dir, upgradeplan.DefaultFileName), `{
	  "packages": [
	    {"ecosystem": "npm", "name": "lodash", "action": "upgrade", "version": "4.17.21", "reason": "no breaking API use found"},
	    {"ecosystem": "npm", "name": "left-pad", "action": "leave", "reason": "upgrade decision unknown"},
	    {"ecosystem": "pypi", "name": "requests", "action": "upgrade", "version": "2.32.0"}
	  ]
	}`)

	apiCalls := 0
	app := newPlanApp(t, lockFile, true, &MockParser{
		ParseFunc: func(context.Context, string) ([]common.PackageInfo, error) {
			return []common.PackageInfo{
				{Name: "lodash", Version: "4.17.20"},
				{Name: "left-pad", Version: "1.3.0"},
			}, nil
		},
	}, &MockAPIClient{
		AnalyzePackagesFunc: func(context.Context, []rootio.Package, []rootio.Package, string) (*rootio.AnalyzePackagesResponse, error) {
			apiCalls++
			return nil, errors.New("API should not be called")
		},
	})

	err := app.Run(context.Background())
	if !errors.Is(err, common.ErrPatchesAvailable) {
		t.Fatalf("err = %v, want ErrPatchesAvailable", err)
	}
	if apiCalls != 0 {
		t.Fatalf("AnalyzePackages calls = %d, want 0", apiCalls)
	}
}

func TestNpmApp_UpgradePlanLeaveOnlyDoesNotPatch(t *testing.T) {
	dir := t.TempDir()
	lockFile := writeMinimalLock(t, dir)
	writePlan(t, filepath.Join(dir, upgradeplan.DefaultFileName), `{
	  "packages": [
	    {"ecosystem": "npm", "name": "left-pad", "action": "leave", "reason": "upgrade decision unknown"}
	  ]
	}`)

	apiCalls := 0
	app := newPlanApp(t, lockFile, false, &MockParser{
		ParseFunc: func(context.Context, string) ([]common.PackageInfo, error) {
			return []common.PackageInfo{{Name: "left-pad", Version: "1.3.0"}}, nil
		},
	}, &MockAPIClient{
		AnalyzePackagesFunc: func(context.Context, []rootio.Package, []rootio.Package, string) (*rootio.AnalyzePackagesResponse, error) {
			apiCalls++
			return nil, errors.New("API should not be called")
		},
	})

	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if apiCalls != 0 {
		t.Fatalf("AnalyzePackages calls = %d, want 0", apiCalls)
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

func TestNpmApp_UpgradePlanAppliesVersion(t *testing.T) {
	dir := t.TempDir()
	lockFile := filepath.Join(dir, "package-lock.json")
	lock := `{
	  "name": "test",
	  "lockfileVersion": 3,
	  "packages": {
	    "": {"dependencies": {"lodash": "^4.17.20", "left-pad": "1.3.0"}},
	    "node_modules/lodash": {"version": "4.17.20"},
	    "node_modules/left-pad": {"version": "1.3.0"}
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
	    "left-pad": "1.3.0"
	  }
	}`
	if err := os.WriteFile(packageJSON, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	writePlan(t, filepath.Join(dir, upgradeplan.DefaultFileName), `{
	  "packages": [
	    {"ecosystem": "npm", "name": "lodash", "action": "upgrade", "version": "4.17.21", "reason": "no breaking API use found"},
	    {"ecosystem": "npm", "name": "left-pad", "action": "leave", "reason": "upgrade decision unknown"}
	  ]
	}`)

	app := newPlanApp(t, lockFile, false, &MockNpmParser{
		NpmParser: NewNpmParser(),
		ParseFunc: func(context.Context, string) ([]common.PackageInfo, error) {
			return []common.PackageInfo{
				{Name: "lodash", Version: "4.17.20"},
				{Name: "left-pad", Version: "1.3.0"},
			}, nil
		},
	}, &MockAPIClient{
		AnalyzePackagesFunc: func(context.Context, []rootio.Package, []rootio.Package, string) (*rootio.AnalyzePackagesResponse, error) {
			return nil, errors.New("API should not be called")
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
	if !strings.Contains(content, `"lodash": "4.17.21"`) {
		t.Errorf("package.json should install lodash@4.17.21, got:\n%s", content)
	}
	if !strings.Contains(content, `"lodash@4.17.20": "4.17.21"`) {
		t.Errorf("package.json should override lodash@4.17.20 to 4.17.21, got:\n%s", content)
	}
	if !strings.Contains(content, `"left-pad": "1.3.0"`) {
		t.Errorf("left-pad should stay unchanged, got:\n%s", content)
	}
	if strings.Contains(content, "left-pad@") {
		t.Errorf("left-pad should not get an override, got:\n%s", content)
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
