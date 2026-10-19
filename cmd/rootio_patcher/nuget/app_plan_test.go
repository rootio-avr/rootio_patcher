package nuget

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/cmd/rootio_patcher/upgradeplan"
	"rootio_patcher/pkg/rootio"
)

func planFixture(t *testing.T) (csproj, dir string) {
	t.Helper()
	dir = t.TempDir()
	csproj = filepath.Join(dir, "MyApp.csproj")
	require.NoError(t, os.WriteFile(csproj, []byte("<Project/>"), 0o644))
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
			require.NoError(t, os.WriteFile(planPath, []byte(`{"packages": [
			  {"ecosystem": "nuget", "name": "newtonsoft.json", "action": "upgrade", "version": "13.0.3"}]}`), 0o644))

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
				require.Equal(t, "nuget", eco)
				return &rootio.AnalyzePackagesResponse{Patches: []rootio.PackagePatch{{
					PackageName: "Serilog", Version: "2.0.0",
					Patch:      rootio.PatchInfo{Name: "Serilog", Version: "2.0.1"},
					PatchAlias: rootio.PatchInfo{Name: "Serilog", Version: "2.0.1"},
				}}}, nil
			}}

			app := NewAppWithServices("key", "https://api.root.io", csproj, false, useAlias, nil, testAppLogger(), parser, client).
				WithUpgradePlan(planPath)
			require.NoError(t, app.Run(context.Background()))

			require.Len(t, gotPackages, 2)
			require.Equal(t, []rootio.Package{{Name: "Newtonsoft.Json", Version: "12.0.3"}}, gotIgnore)
			require.Equal(t, map[string]string{"Newtonsoft.Json": "Newtonsoft.Json:13.0.3", "Serilog": "Serilog:2.0.1"}, updates)
		})
	}
}

func TestNuGetApp_ExplicitMissingPlanIsError(t *testing.T) {
	csproj, dir := planFixture(t)
	app := NewAppWithServices("key", "https://api.root.io", csproj, true, false, nil, testAppLogger(),
		&MockParser{ParseFunc: installedPair()}, &MockAPIClient{}).WithUpgradePlan(filepath.Join(dir, "missing.json"))

	err := app.Run(context.Background())
	require.ErrorIs(t, err, os.ErrNotExist)
	require.ErrorContains(t, err, "read upgrade plan")
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
		require.NoError(t, app.Run(context.Background()))
		require.Empty(t, gotIgnore)

		require.NoError(t, os.WriteFile(filepath.Join(dir, upgradeplan.DefaultFileName), plan, 0o644))
		require.ErrorIs(t, app.Run(context.Background()), common.ErrPatchesAvailable)
		require.Equal(t, []rootio.Package{{Name: "Serilog", Version: "2.0.0"}}, gotIgnore)
	}
}
