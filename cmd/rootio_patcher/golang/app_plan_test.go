package golang

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rootio_patcher/cmd/rootio_patcher/upgradeplan"
	"rootio_patcher/pkg/rootio"
)

const planGoMod = `module example.com/app

go 1.21

require (
	github.com/google/uuid v1.3.0
	golang.org/x/net v0.16.0
)
`

func planApp(goModPath string, useAlias bool, api *MockAPIClient, cmd *MockCommandRunner) *App {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewApp("test-key", "https://api.root.io", "https://pkg.root.io", goModPath, false, useAlias, nil, logger,
		NewGoModParser(logger), api, cmd)
}

func uuidAPIPatch(ignore *[]rootio.Package, packages *[]rootio.Package) *MockAPIClient {
	return &MockAPIClient{AnalyzePackagesFunc: func(_ context.Context, pkgs, ign []rootio.Package, eco string) (*rootio.AnalyzePackagesResponse, error) {
		*packages, *ignore = pkgs, ign
		if eco != "golang" {
			panic("ecosystem " + eco)
		}
		return &rootio.AnalyzePackagesResponse{Patches: []rootio.PackagePatch{{
			PackageName: "github.com/google/uuid", Version: "v1.3.0",
			Patch:      rootio.PatchInfo{Name: "github.com/google/uuid", Version: "v1.3.1-rootio.1"},
			PatchAlias: rootio.PatchInfo{Name: "pkg.root.io/golang/github.com/google/uuid", Version: "v1.3.1-rootio.1"},
		}}}, nil
	}}
}

func TestGoLangApp_PlannedModuleIsReplacedVerbatimAndIgnoredForAPI(t *testing.T) {
	cases := []struct {
		name        string
		version     string
		wantReplace string
	}{
		{"upstream release", "v0.17.0", "replace golang.org/x/net v0.16.0 => golang.org/x/net v0.17.0"},
		{"root build", "v0.17.0-aikido.2", "replace golang.org/x/net v0.16.0 => golang.org/x/net v0.17.0-aikido.2"},
	}
	for _, tc := range cases {
		for _, useAlias := range []bool{false, true} {
			t.Run(tc.name+"/alias="+map[bool]string{false: "off", true: "on"}[useAlias], func(t *testing.T) {
				dir := t.TempDir()
				goModPath := writeGoMod(t, dir, planGoMod)
				planPath := filepath.Join(dir, "plan.json")
				require.NoError(t, os.WriteFile(planPath, []byte(`{"packages": [
				  {"ecosystem": "go", "name": "golang.org/x/net", "action": "upgrade", "version": "`+tc.version+`"}]}`), 0o644))

				var gotPackages, gotIgnore []rootio.Package
				cmd := &MockCommandRunner{}
				app := planApp(goModPath, useAlias, uuidAPIPatch(&gotIgnore, &gotPackages), cmd).WithUpgradePlan(planPath)
				require.NoError(t, app.Run(context.Background()))

				assert.Len(t, gotPackages, 2)
				assert.Equal(t, []rootio.Package{{Name: "golang.org/x/net", Version: "v0.16.0"}}, gotIgnore)

				content, err := os.ReadFile(goModPath)
				require.NoError(t, err)
				assert.True(t, containsLine(string(content), tc.wantReplace), string(content))
				assert.True(t, strings.Contains(string(content), "require (\n\tgithub.com/google/uuid v1.3.0\n\tgolang.org/x/net v0.16.0\n)"), "requires must not change:\n"+string(content))
				apiReplace := "replace github.com/google/uuid v1.3.0 => github.com/google/uuid v1.3.1-rootio.1"
				if useAlias {
					apiReplace = "replace github.com/google/uuid v1.3.0 => pkg.root.io/golang/github.com/google/uuid v1.3.1-rootio.1"
				}
				assert.True(t, containsLine(string(content), apiReplace), "unplanned module still comes from the API:\n"+string(content))
				require.Len(t, cmd.Calls, 1)
				assert.Equal(t, "mod tidy", strings.Join(cmd.Calls[0].Args, " "))
			})
		}
	}
}

func TestGoLangApp_PlannedModuleIsExemptFromSumDBLikeAPIPatches(t *testing.T) {
	dir := t.TempDir()
	goModPath := writeGoMod(t, dir, planGoMod)
	planPath := filepath.Join(dir, "plan.json")
	require.NoError(t, os.WriteFile(planPath, []byte(`{"packages": [{"ecosystem": "go", "name": "golang.org/x/net", "action": "upgrade", "version": "v0.17.0-aikido.2"}]}`), 0o644))

	var ignore, packages []rootio.Package
	cmd := &MockCommandRunner{}
	require.NoError(t, planApp(goModPath, false, uuidAPIPatch(&ignore, &packages), cmd).WithUpgradePlan(planPath).Run(context.Background()))

	require.Len(t, cmd.Calls, 1)
	assert.Contains(t, cmd.Calls[0].Env, "GONOSUMDB=golang.org/x/net,github.com/google/uuid")
}

func TestGoLangApp_ReportShowsPlannedVersion(t *testing.T) {
	dir := t.TempDir()
	goModPath := writeGoMod(t, dir, planGoMod)
	planPath := filepath.Join(dir, "plan.json")
	reportPath := filepath.Join(dir, "report.json")
	require.NoError(t, os.WriteFile(planPath, []byte(`{"packages": [{"ecosystem": "golang", "name": "golang.org/x/net", "action": "upgrade", "version": "v0.17.0"}]}`), 0o644))

	var ignore, packages []rootio.Package
	app := planApp(goModPath, false, uuidAPIPatch(&ignore, &packages), &MockCommandRunner{}).WithUpgradePlan(planPath).WithReport(reportPath)
	require.NoError(t, app.Run(context.Background()))

	data, err := os.ReadFile(reportPath)
	require.NoError(t, err)
	var entries []ReportEntry
	require.NoError(t, json.Unmarshal(data, &entries))
	require.Len(t, entries, 2)
	assert.Equal(t, ReportEntry{Name: "golang.org/x/net", OldVersion: "v0.16.0", NewVersion: "v0.17.0", CVEIDs: []string{}}, entries[0])
}

func TestGoLangApp_ExplicitMissingPlanIsError(t *testing.T) {
	dir := t.TempDir()
	goModPath := writeGoMod(t, dir, planGoMod)
	app := planApp(goModPath, false, &MockAPIClient{}, &MockCommandRunner{}).WithUpgradePlan(filepath.Join(dir, "missing.json"))

	err := app.Run(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrNotExist)
	assert.Contains(t, err.Error(), "read upgrade plan")
}

func TestGoLangApp_DefaultPlanNextToGoModIsUsedAndMissingMeansNoPlan(t *testing.T) {
	dir := t.TempDir()
	goModPath := writeGoMod(t, dir, planGoMod)
	var ignore, packages []rootio.Package
	app := planApp(goModPath, false, uuidAPIPatch(&ignore, &packages), &MockCommandRunner{})

	require.NoError(t, app.Run(context.Background()))
	assert.Empty(t, ignore)

	writeGoMod(t, dir, planGoMod) // fresh go.mod: the first run added a replace
	require.NoError(t, os.WriteFile(filepath.Join(dir, upgradeplan.DefaultFileName),
		[]byte(`{"packages": [{"ecosystem": "go", "name": "golang.org/x/net", "action": "upgrade", "version": "v0.17.0"}]}`), 0o644))
	require.NoError(t, app.Run(context.Background()))
	assert.Equal(t, []rootio.Package{{Name: "golang.org/x/net", Version: "v0.16.0"}}, ignore)
}
