package maven

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
	"rootio_patcher/cmd/rootio_patcher/upgradeplan"
	"rootio_patcher/pkg/rootio"
)

func planTestApp(t *testing.T, useAlias bool, parser *MockParser, client *MockAPIClient) (*App, string) {
	t.Helper()
	pom := filepath.Join(t.TempDir(), "pom.xml")
	if err := os.WriteFile(pom, []byte("<project/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewAppWithServices("key", "https://api.root.io", pom, false, useAlias, nil, logger, parser, client), pom
}

func TestMavenApp_PlannedPackageIsPatchedVerbatimAndIgnoredForAPI(t *testing.T) {
	for _, useAlias := range []bool{false, true} {
		name := "no alias"
		if useAlias {
			name = "alias"
		}
		t.Run(name, func(t *testing.T) {
			var gotPackages, gotIgnore []rootio.Package
			var updates map[string]string
			parser := &MockParser{
				ParseFunc: func(context.Context, string) ([]common.PackageInfo, error) {
					return []common.PackageInfo{
						{Name: "junit:junit", Version: "4.12"},
						{Name: "commons-lang:commons-lang", Version: "2.6"},
					}, nil
				},
				UpdateFunc: func(_ context.Context, _ string, u map[string]string) (string, error) {
					updates = u
					return "<project/>", nil
				},
			}
			client := &MockAPIClient{AnalyzePackagesFunc: func(_ context.Context, pkgs, ignore []rootio.Package, eco string) (*rootio.AnalyzePackagesResponse, error) {
				gotPackages, gotIgnore = pkgs, ignore
				if eco != "maven" {
					t.Errorf("ecosystem = %q", eco)
				}
				return &rootio.AnalyzePackagesResponse{Patches: []rootio.PackagePatch{{
					PackageName: "commons-lang:commons-lang", Version: "2.6",
					Patch:      rootio.PatchInfo{Name: "commons-lang:commons-lang", Version: "2.6-root.io.1"},
					PatchAlias: rootio.PatchInfo{Name: "commons-lang:commons-lang", Version: "2.6-root.io.1"},
				}}}, nil
			}}
			app, pom := planTestApp(t, useAlias, parser, client)
			planPath := filepath.Join(filepath.Dir(pom), "plan.json")
			if err := os.WriteFile(planPath, []byte(`{"packages": [
			  {"ecosystem": "maven", "name": "junit:junit", "action": "upgrade", "version": "4.13.2"}]}`), 0o644); err != nil {
				t.Fatal(err)
			}

			if err := app.WithUpgradePlan(planPath).Run(context.Background()); err != nil {
				t.Fatalf("Run: %v", err)
			}

			if len(gotPackages) != 2 {
				t.Fatalf("API packages = %+v", gotPackages)
			}
			if want := []rootio.Package{{Name: "junit:junit", Version: "4.12"}}; !reflect.DeepEqual(gotIgnore, want) {
				t.Fatalf("API ignore = %+v, want %+v", gotIgnore, want)
			}
			want := map[string]string{
				"junit:junit":               "junit:junit:4.13.2",
				"commons-lang:commons-lang": "commons-lang:commons-lang:2.6-root.io.1",
			}
			if !reflect.DeepEqual(updates, want) {
				t.Fatalf("updates = %+v, want %+v", updates, want)
			}
		})
	}
}

func TestMavenApp_ExplicitMissingPlanIsError(t *testing.T) {
	parser := &MockParser{ParseFunc: func(context.Context, string) ([]common.PackageInfo, error) {
		return []common.PackageInfo{{Name: "junit:junit", Version: "4.12"}}, nil
	}}
	app, pom := planTestApp(t, false, parser, &MockAPIClient{})

	err := app.WithUpgradePlan(filepath.Join(filepath.Dir(pom), "missing.json")).Run(context.Background())
	if err == nil || !errors.Is(err, os.ErrNotExist) || !strings.Contains(err.Error(), "read upgrade plan") {
		t.Fatalf("err = %v, want read upgrade plan error", err)
	}
}

func TestMavenApp_DefaultPlanNextToPomIsUsedAndMissingMeansNoPlan(t *testing.T) {
	var gotIgnore []rootio.Package
	parser := &MockParser{
		ParseFunc: func(context.Context, string) ([]common.PackageInfo, error) {
			return []common.PackageInfo{{Name: "junit:junit", Version: "4.12"}}, nil
		},
		UpdateFunc: func(context.Context, string, map[string]string) (string, error) { return "<project/>", nil },
	}
	client := &MockAPIClient{AnalyzePackagesFunc: func(_ context.Context, _, ignore []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
		gotIgnore = ignore
		return &rootio.AnalyzePackagesResponse{}, nil
	}}
	app, pom := planTestApp(t, false, parser, client)

	if err := app.Run(context.Background()); err != nil || len(gotIgnore) != 0 {
		t.Fatalf("no default plan: err = %v ignore = %+v", err, gotIgnore)
	}

	planPath := filepath.Join(filepath.Dir(pom), upgradeplan.DefaultFileName)
	if err := os.WriteFile(planPath, []byte(`{"packages": [{"ecosystem": "maven", "name": "junit:junit", "action": "upgrade", "version": "4.13.2"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("with default plan: %v", err)
	}
	if want := []rootio.Package{{Name: "junit:junit", Version: "4.12"}}; !reflect.DeepEqual(gotIgnore, want) {
		t.Fatalf("ignore = %+v, want %+v", gotIgnore, want)
	}
}
