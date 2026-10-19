package maven

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/cmd/rootio_patcher/upgradeplan"
	"rootio_patcher/pkg/rootio"
)

func planTestApp(t *testing.T, useAlias bool, parser *MockParser, client *MockAPIClient) (*App, string) {
	t.Helper()
	pom := filepath.Join(t.TempDir(), "pom.xml")
	require.NoError(t, os.WriteFile(pom, []byte("<project/>"), 0o644))
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
				require.Equal(t, "maven", eco)
				return &rootio.AnalyzePackagesResponse{Patches: []rootio.PackagePatch{{
					PackageName: "commons-lang:commons-lang", Version: "2.6",
					Patch:      rootio.PatchInfo{Name: "commons-lang:commons-lang", Version: "2.6-root.io.1"},
					PatchAlias: rootio.PatchInfo{Name: "commons-lang:commons-lang", Version: "2.6-root.io.1"},
				}}}, nil
			}}
			app, pom := planTestApp(t, useAlias, parser, client)
			planPath := filepath.Join(filepath.Dir(pom), "plan.json")
			require.NoError(t, os.WriteFile(planPath, []byte(`{"packages": [
			  {"ecosystem": "maven", "name": "junit:junit", "action": "upgrade", "version": "4.13.2"}]}`), 0o644))

			require.NoError(t, app.WithUpgradePlan(planPath).Run(context.Background()))

			require.Len(t, gotPackages, 2)
			require.Equal(t, []rootio.Package{{Name: "junit:junit", Version: "4.12"}}, gotIgnore)
			require.Equal(t, map[string]string{
				"junit:junit":               "junit:junit:4.13.2",
				"commons-lang:commons-lang": "commons-lang:commons-lang:2.6-root.io.1",
			}, updates)
		})
	}
}

func TestMavenApp_ExplicitMissingPlanIsError(t *testing.T) {
	parser := &MockParser{ParseFunc: func(context.Context, string) ([]common.PackageInfo, error) {
		return []common.PackageInfo{{Name: "junit:junit", Version: "4.12"}}, nil
	}}
	app, pom := planTestApp(t, false, parser, &MockAPIClient{})

	err := app.WithUpgradePlan(filepath.Join(filepath.Dir(pom), "missing.json")).Run(context.Background())
	require.ErrorIs(t, err, os.ErrNotExist)
	require.ErrorContains(t, err, "read upgrade plan")
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

	require.NoError(t, app.Run(context.Background()))
	require.Empty(t, gotIgnore)

	planPath := filepath.Join(filepath.Dir(pom), upgradeplan.DefaultFileName)
	require.NoError(t, os.WriteFile(planPath, []byte(`{"packages": [{"ecosystem": "maven", "name": "junit:junit", "action": "upgrade", "version": "4.13.2"}]}`), 0o644))
	require.NoError(t, app.Run(context.Background()))
	require.Equal(t, []rootio.Package{{Name: "junit:junit", Version: "4.12"}}, gotIgnore)
}
