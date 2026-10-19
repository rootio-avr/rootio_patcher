package composer

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

func planFixture(t *testing.T) (composerFile, dir string) {
	t.Helper()
	dir = t.TempDir()
	composerFile = filepath.Join(dir, "composer.json")
	require.NoError(t, os.WriteFile(composerFile, []byte(`{"require":{}}`), 0o644))
	return composerFile, dir
}

func installedPair() *MockParser {
	return &MockParser{ParseFunc: func(context.Context, string) ([]common.PackageInfo, error) {
		return []common.PackageInfo{
			{Name: "monolog/monolog", Version: "2.0.0"},
			{Name: "guzzlehttp/guzzle", Version: "7.0.0"},
		}, nil
	}}
}

func TestComposerApp_PlannedPackageIsPatchedVerbatimAndIgnoredForAPI(t *testing.T) {
	for _, useAlias := range []bool{false, true} {
		name := "no alias"
		if useAlias {
			name = "alias"
		}
		t.Run(name, func(t *testing.T) {
			composerFile, dir := planFixture(t)
			planPath := filepath.Join(dir, "plan.json")
			require.NoError(t, os.WriteFile(planPath, []byte(`{"packages": [
			  {"ecosystem": "composer", "name": "Monolog/Monolog", "action": "upgrade", "version": "2.9.3"}]}`), 0o644))

			var gotPackages, gotIgnore []rootio.Package
			var updates map[string]string
			parser := installedPair()
			parser.UpdateFunc = func(_ context.Context, _ string, u map[string]string) (string, error) {
				updates = u
				return "{}", nil
			}
			api := &MockAPIClient{AnalyzePackagesFunc: func(_ context.Context, pkgs, ignore []rootio.Package, eco string) (*rootio.AnalyzePackagesResponse, error) {
				gotPackages, gotIgnore = pkgs, ignore
				require.Equal(t, "composer", eco)
				return &rootio.AnalyzePackagesResponse{Patches: []rootio.PackagePatch{{
					PackageName: "guzzlehttp/guzzle", Version: "7.0.0",
					Patch:      rootio.PatchInfo{Name: "guzzlehttp/guzzle", Version: "7.0.1"},
					PatchAlias: rootio.PatchInfo{Name: "guzzlehttp/guzzle", Version: "7.0.1"},
				}}}, nil
			}}
			cmd := &MockCommandRunner{}

			app := newTestApp(composerFile, false, useAlias, parser, api, cmd).WithUpgradePlan(planPath)
			require.NoError(t, app.Run(context.Background()))

			require.Len(t, gotPackages, 2)
			require.Equal(t, []rootio.Package{{Name: "monolog/monolog", Version: "2.0.0"}}, gotIgnore)
			require.Equal(t, map[string]string{
				"monolog/monolog":   "monolog/monolog:2.9.3",
				"guzzlehttp/guzzle": "guzzlehttp/guzzle:7.0.1",
			}, updates)
			require.Len(t, cmd.Calls, 1)
			require.Contains(t, cmd.Calls[0].Args, "monolog/monolog")
			require.Contains(t, cmd.Calls[0].Args, "guzzlehttp/guzzle")
		})
	}
}

func TestComposerApp_ExplicitMissingPlanIsError(t *testing.T) {
	composerFile, dir := planFixture(t)
	app := newTestApp(composerFile, true, false, installedPair(), &MockAPIClient{}, &MockCommandRunner{}).
		WithUpgradePlan(filepath.Join(dir, "missing.json"))

	err := app.Run(context.Background())
	require.Error(t, err)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.Contains(t, err.Error(), "read upgrade plan")
}

func TestComposerApp_DefaultPlanNextToComposerJSONIsUsedAndMissingMeansNoPlan(t *testing.T) {
	composerFile, dir := planFixture(t)
	var gotIgnore []rootio.Package
	api := &MockAPIClient{AnalyzePackagesFunc: func(_ context.Context, _, ignore []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
		gotIgnore = ignore
		return &rootio.AnalyzePackagesResponse{}, nil
	}}
	app := newTestApp(composerFile, true, false, installedPair(), api, &MockCommandRunner{})

	require.NoError(t, app.Run(context.Background()))
	require.Empty(t, gotIgnore)

	require.NoError(t, os.WriteFile(filepath.Join(dir, upgradeplan.DefaultFileName),
		[]byte(`{"packages": [{"ecosystem": "composer", "name": "monolog/monolog", "action": "upgrade", "version": "2.9.3"}]}`), 0o644))
	require.ErrorIs(t, app.Run(context.Background()), common.ErrPatchesAvailable)
	require.Equal(t, []rootio.Package{{Name: "monolog/monolog", Version: "2.0.0"}}, gotIgnore)
}
