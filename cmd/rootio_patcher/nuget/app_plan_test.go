package nuget

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/cmd/rootio_patcher/upgradeplan"
	"rootio_patcher/pkg/rootio"
)

func planFixture(t *testing.T) (csproj, dir string) {
	t.Helper()
	dir = t.TempDir()
	csproj = filepath.Join(dir, "MyApp.csproj")
	if err := os.WriteFile(csproj, []byte("<Project/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	return csproj, dir
}

func installedPair() func(context.Context, string) ([]common.PackageInfo, error) {
	return func(context.Context, string) ([]common.PackageInfo, error) {
		return []common.PackageInfo{
			{Name: "Newtonsoft.Json", Version: "12.0.3"},
			{Name: "Serilog", Version: "2.0.0"},
		}, nil
	}
}

func TestNuGetApp_PlannedPackageIsPatchedVerbatimAndIgnoredForAPI(t *testing.T) {
	for _, useAlias := range []bool{false, true} {
		name := "no alias"
		if useAlias {
			name = "alias"
		}
		t.Run(name, func(t *testing.T) {
			csproj, dir := planFixture(t)
			planPath := filepath.Join(dir, "plan.json")
			if err := os.WriteFile(planPath, []byte(`{"packages": [
			  {"ecosystem": "nuget", "name": "newtonsoft.json", "action": "upgrade", "version": "13.0.3"}]}`), 0o644); err != nil {
				t.Fatal(err)
			}

			var gotPackages, gotIgnore []rootio.Package
			var updates map[string]string
			parser := &MockParser{
				ParseFunc: installedPair(),
				UpdateFunc: func(_ context.Context, _ string, u map[string]string) (string, error) {
					updates = u
					return "<Project/>", nil
				},
			}
			client := &MockAPIClient{AnalyzePackagesFunc: func(_ context.Context, pkgs, ignore []rootio.Package, eco string) (*rootio.AnalyzePackagesResponse, error) {
				gotPackages, gotIgnore = pkgs, ignore
				if eco != "nuget" {
					t.Errorf("ecosystem = %q", eco)
				}
				return &rootio.AnalyzePackagesResponse{Patches: []rootio.PackagePatch{{
					PackageName: "Serilog", Version: "2.0.0",
					Patch:      rootio.PatchInfo{Name: "Serilog", Version: "2.0.1"},
					PatchAlias: rootio.PatchInfo{Name: "Serilog", Version: "2.0.1"},
				}}}, nil
			}}

			app := NewAppWithServices("key", "https://api.root.io", csproj, false, useAlias, nil, testAppLogger(), parser, client).
				WithUpgradePlan(planPath)
			if err := app.Run(context.Background()); err != nil {
				t.Fatalf("Run: %v", err)
			}

			if len(gotPackages) != 2 {
				t.Fatalf("API packages = %+v", gotPackages)
			}
			if want := []rootio.Package{{Name: "Newtonsoft.Json", Version: "12.0.3"}}; !reflect.DeepEqual(gotIgnore, want) {
				t.Fatalf("API ignore = %+v, want %+v", gotIgnore, want)
			}
			want := map[string]string{"Newtonsoft.Json": "Newtonsoft.Json:13.0.3", "Serilog": "Serilog:2.0.1"}
			if !reflect.DeepEqual(updates, want) {
				t.Fatalf("updates = %+v, want %+v", updates, want)
			}
		})
	}
}

func TestNuGetApp_ExplicitMissingPlanIsError(t *testing.T) {
	csproj, dir := planFixture(t)
	app := NewAppWithServices("key", "https://api.root.io", csproj, true, false, nil, testAppLogger(),
		&MockParser{ParseFunc: installedPair()}, &MockAPIClient{}).WithUpgradePlan(filepath.Join(dir, "missing.json"))

	err := app.Run(context.Background())
	if err == nil || !errors.Is(err, os.ErrNotExist) || !strings.Contains(err.Error(), "read upgrade plan") {
		t.Fatalf("err = %v, want read upgrade plan error", err)
	}
}

func TestNuGetApp_DefaultPlanIsReadFromManifestDirectory(t *testing.T) {
	csproj, dir := planFixture(t)
	var gotIgnore []rootio.Package
	client := &MockAPIClient{AnalyzePackagesFunc: func(_ context.Context, _, ignore []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
		gotIgnore = ignore
		return &rootio.AnalyzePackagesResponse{}, nil
	}}
	plan := []byte(`{"packages": [{"ecosystem": "nuget", "name": "Serilog", "action": "upgrade", "version": "2.1.0"}]}`)

	// Both a manifest file path and its directory resolve to the same default plan file.
	for _, path := range []string{csproj, dir} {
		gotIgnore = nil
		app := NewAppWithServices("key", "https://api.root.io", path, true, false, nil, testAppLogger(),
			&MockParser{ParseFunc: installedPair()}, client)
		_ = os.Remove(filepath.Join(dir, upgradeplan.DefaultFileName))
		if err := app.Run(context.Background()); err != nil || len(gotIgnore) != 0 {
			t.Fatalf("%s without plan: err = %v ignore = %+v", path, err, gotIgnore)
		}

		if err := os.WriteFile(filepath.Join(dir, upgradeplan.DefaultFileName), plan, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := app.Run(context.Background()); !errors.Is(err, common.ErrPatchesAvailable) {
			t.Fatalf("%s with plan: err = %v, want ErrPatchesAvailable", path, err)
		}
		if want := []rootio.Package{{Name: "Serilog", Version: "2.0.0"}}; !reflect.DeepEqual(gotIgnore, want) {
			t.Fatalf("%s ignore = %+v, want %+v", path, gotIgnore, want)
		}
	}
}
