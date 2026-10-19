package npm

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

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
	require.ErrorIs(t, err, common.ErrPatchesAvailable)
	require.Equal(t, 1, apiCalls)
	require.Len(t, gotPackages, 2)
	require.Equal(t, []rootio.Package{{Name: "lodash", Version: "4.17.20"}}, gotIgnore)
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

	err := app.Run(context.Background())
	require.ErrorIs(t, err, common.ErrPatchesAvailable)
	sort.Slice(gotIgnore, func(i, j int) bool { return gotIgnore[i].Name < gotIgnore[j].Name })
	require.Equal(t, []rootio.Package{{Name: "debug", Version: "4.3.4"}, {Name: "lodash", Version: "4.17.20"}}, gotIgnore)
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

	require.NoError(t, app.Run(context.Background()))
	require.Equal(t, 1, apiCalls)
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
			require.Fail(t, "AnalyzePackages called")
			return &rootio.AnalyzePackagesResponse{}, nil
		},
	})

	err := app.Run(context.Background())
	require.ErrorContains(t, err, `action "leave"`)
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

	require.NoError(t, app.Run(context.Background()))
	require.Equal(t, 1, apiCalls)
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
	require.ErrorIs(t, err, os.ErrNotExist)
	require.Zero(t, apiCalls)
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
			require.Fail(t, "AnalyzePackages called")
			return nil, errors.New("API should not be called")
		},
	})

	require.Error(t, app.Run(context.Background()))
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
	require.NoError(t, os.WriteFile(lockFile, []byte(lock), 0o644))
	packageJSON := filepath.Join(dir, "package.json")
	manifest := `{
	  "name": "test",
	  "version": "1.0.0",
	  "dependencies": {
	    "lodash": "^4.17.20",
	    "express": "4.18.2"
	  }
	}`
	require.NoError(t, os.WriteFile(packageJSON, []byte(manifest), 0o644))
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
				require.NotEqual(t, "express", pkg.Name)
			}
			return &rootio.AnalyzePackagesResponse{Patches: []rootio.PackagePatch{{
				PackageName: "express",
				Version:     "4.18.2",
				Patch:       rootio.PatchInfo{Name: "express", Version: "4.18.2-root.io.1"},
			}}}, nil
		},
	})

	require.NoError(t, app.Run(context.Background()))

	updated, err := os.ReadFile(packageJSON)
	require.NoError(t, err)
	content := string(updated)
	require.Contains(t, content, `"lodash": "4.17.21-aikido.2"`)
	require.Contains(t, content, `"lodash@4.17.20": "4.17.21-aikido.2"`)
	require.Contains(t, content, `"express": "4.18.2-root.io.1"`)
}

func newPlanApp(t *testing.T, lockFile string, dryRun bool, parser npmParser, client *MockAPIClient) *App {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewAppWithServices("test-key", "https://api.root.io", lockFile, dryRun, nil, logger, parser, client)
}

func writeMinimalLock(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "package-lock.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"name":"test","lockfileVersion":3,"packages":{"":{}}}`), 0o644))
	return path
}

func writePlan(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}
