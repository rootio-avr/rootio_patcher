package ruby

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/cmd/rootio_patcher/upgradeplan"
	"rootio_patcher/pkg/rootio"
)

type recordingAPI struct {
	packages, ignore []rootio.Package
	patches          []rootio.PackagePatch
}

func (r *recordingAPI) AnalyzePackages(_ context.Context, pkgs, ignore []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
	r.packages, r.ignore = pkgs, ignore
	return &rootio.AnalyzePackagesResponse{Patches: r.patches}, nil
}

func nokogiriRootPatch() rootio.PackagePatch {
	return rootio.PackagePatch{
		PackageName: "nokogiri", Version: "1.15.0",
		Patch:      rootio.PatchInfo{Name: "nokogiri", Version: "1.15.0.41610.1"},
		PatchAlias: rootio.PatchInfo{Name: "nokogiri", Version: "1.15.0.41610.1"},
	}
}

func writePlan(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func planApp(dir string, dry bool, api common.APIClient, r CommandRunner) *App {
	return NewApp("secret-key", "https://pkg.root.io", dir, dry, nil, slog.Default(), api, r)
}

func lockAfter(replacements ...string) string {
	out := lock
	for i := 0; i < len(replacements); i += 2 {
		out = strings.Replace(out, replacements[i], replacements[i+1], 1)
	}
	return out
}

func TestRun_PlannedGemIsIgnoredForAPIAndPatchedVerbatim(t *testing.T) {
	dir := setup(t)
	writePlan(t, filepath.Join(dir, "plan.json"), `{"packages": [{"ecosystem": "ruby", "name": "rack", "action": "upgrade", "version": "2.2.4"}]}`)
	api := &recordingAPI{patches: []rootio.PackagePatch{nokogiriRootPatch()}}
	r := &fakeRunner{lockOut: lockAfter("rack (2.2.3)", "rack (2.2.4)", "    nokogiri (1.15.0-x86_64-linux)\n", "", "nokogiri (1.15.0)\n", "nokogiri (1.15.0.41610.1)\n")}

	require.NoError(t, planApp(dir, false, api, r).WithUpgradePlan(filepath.Join(dir, "plan.json")).Run(context.Background()))

	assert.Len(t, api.packages, len(ParseLockfile(lock)))
	assert.Equal(t, []rootio.Package{{Name: "rack", Version: "2.2.3"}}, api.ignore)
	assert.Equal(t, []string{"bundle", "update", "rack", "nokogiri", "--conservative"}, r.args)
	gemfile, _ := os.ReadFile(filepath.Join(dir, "Gemfile"))
	assert.Contains(t, string(gemfile), "gem 'rack', '>= 2.2.4'")
	assert.Contains(t, string(gemfile), "gem 'nokogiri', '>= 1.15.0.41610.1', source: 'https://pkg.root.io/ruby'")
}

func TestRun_PlannedNameMatchesTheLockfileNameExactly(t *testing.T) {
	dir := setup(t)
	writePlan(t, filepath.Join(dir, "plan.json"), `{"packages": [{"ecosystem": "ruby", "name": "Rack", "action": "upgrade", "version": "2.2.4"}]}`)
	api := &recordingAPI{}

	require.NoError(t, planApp(dir, true, api, &fakeRunner{}).WithUpgradePlan(filepath.Join(dir, "plan.json")).Run(context.Background()))
	assert.Empty(t, api.ignore, "ruby gem names are case-sensitive")
}

func TestRun_ExplicitMissingPlanIsError(t *testing.T) {
	dir := setup(t)
	err := planApp(dir, true, &recordingAPI{}, &fakeRunner{}).WithUpgradePlan(filepath.Join(dir, "missing.json")).Run(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrNotExist)
	assert.Contains(t, err.Error(), "read upgrade plan")
}

func TestRun_DefaultPlanInProjectDirIsUsedAndMissingMeansNoPlan(t *testing.T) {
	dir := setup(t)
	api := &recordingAPI{}
	a := planApp(dir, true, api, &fakeRunner{})

	require.NoError(t, a.Run(context.Background()))
	assert.Empty(t, api.ignore)

	writePlan(t, filepath.Join(dir, upgradeplan.DefaultFileName), `{"packages": [{"ecosystem": "ruby", "name": "rack", "action": "upgrade", "version": "2.2.4"}]}`)
	assert.ErrorIs(t, a.Run(context.Background()), common.ErrPatchesAvailable)
	assert.Equal(t, []rootio.Package{{Name: "rack", Version: "2.2.3"}}, api.ignore)
}
