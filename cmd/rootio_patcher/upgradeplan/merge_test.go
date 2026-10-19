package upgradeplan

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"rootio_patcher/pkg/rootio"
)

type fakeClient struct {
	calls    int
	packages []rootio.Package
	ignore   []rootio.Package
	ecosys   string
	resp     *rootio.AnalyzePackagesResponse
	err      error
}

func (f *fakeClient) AnalyzePackages(_ context.Context, packages, ignore []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error) {
	f.calls++
	f.packages, f.ignore, f.ecosys = packages, ignore, ecosystem
	return f.resp, f.err
}

func apiPatch(name, from, to string) rootio.PackagePatch {
	return rootio.PackagePatch{PackageName: name, Version: from, Patch: rootio.PatchInfo{Name: name, Version: to}}
}

func testLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, nil)), &buf
}

func TestAnalyze(t *testing.T) {
	installed := []rootio.Package{{Name: "lodash", Version: "4.17.20"}, {Name: "express", Version: "4.18.2"}, {Name: "ms", Version: "2.0.0"}}
	user := []rootio.Package{{Name: "debug", Version: "1.0.0"}}
	api := &rootio.AnalyzePackagesResponse{
		Patches: []rootio.PackagePatch{apiPatch("express", "4.18.2", "4.18.2-root.io.1")},
		Skipped: []rootio.SkippedPackage{{PackageName: "ms", Reason: "no fix"}},
	}

	t.Run("no plan", func(t *testing.T) {
		client := &fakeClient{resp: api}
		logger, _ := testLogger()
		got, err := Analyze(context.Background(), logger, client, nil, "npm", installed, user)
		require.NoError(t, err)
		require.Equal(t, 1, client.calls)
		require.Equal(t, "npm", client.ecosys)
		require.Equal(t, installed, client.packages)
		require.Equal(t, user, client.ignore)
		require.Equal(t, api, got)
	})

	t.Run("merges plan ahead of api", func(t *testing.T) {
		client := &fakeClient{resp: api}
		logger, logs := testLogger()
		plan := &Plan{Packages: []Entry{{Ecosystem: "npm", Name: "lodash", Action: "upgrade", Version: "4.17.21-aikido.2"}}}
		got, err := Analyze(context.Background(), logger, client, plan, "npm", installed, user)
		require.NoError(t, err)
		require.Equal(t, []rootio.Package{user[0], {Name: "lodash", Version: "4.17.20"}}, client.ignore)
		require.Equal(t, []rootio.PackagePatch{
			planned("lodash", "4.17.20", "4.17.21-aikido.2"),
			apiPatch("express", "4.18.2", "4.18.2-root.io.1"),
		}, got.Patches)
		require.Equal(t, api.Skipped, got.Skipped)
		require.Contains(t, logs.String(), `msg="Package upgraded" package=lodash from=4.17.20 to=4.17.21-aikido.2`)
		require.Contains(t, logs.String(), `msg="Package patched" package=express from=4.18.2 to=4.18.2-root.io.1`)
	})

	t.Run("planned package drops the api patch for that version", func(t *testing.T) {
		client := &fakeClient{resp: &rootio.AnalyzePackagesResponse{Patches: []rootio.PackagePatch{
			apiPatch("lodash", "4.17.20", "4.17.20-root.io.1"),
			apiPatch("lodash", "3.10.1", "3.10.1-root.io.1"),
		}}}
		logger, _ := testLogger()
		plan := &Plan{Packages: []Entry{{Ecosystem: "npm", Name: "lodash", Action: "upgrade", Version: "4.17.21"}}}
		got, err := Analyze(context.Background(), logger, client, plan, "npm", []rootio.Package{{Name: "lodash", Version: "4.17.20"}}, nil)
		require.NoError(t, err)
		require.Equal(t, []rootio.PackagePatch{
			planned("lodash", "4.17.20", "4.17.21"),
			apiPatch("lodash", "3.10.1", "3.10.1-root.io.1"),
		}, got.Patches)
	})

	t.Run("ignore is deduped and a current version is not patched", func(t *testing.T) {
		client := &fakeClient{resp: &rootio.AnalyzePackagesResponse{}}
		logger, _ := testLogger()
		plan := &Plan{Packages: []Entry{{Ecosystem: "npm", Name: "lodash", Action: "upgrade", Version: "4.17.20"}}}
		user := []rootio.Package{{Name: "lodash", Version: "4.17.20"}}
		got, err := Analyze(context.Background(), logger, client, plan, "npm", user, user)
		require.NoError(t, err)
		require.Equal(t, user, client.ignore)
		require.Empty(t, got.Patches)
	})

	t.Run("errors", func(t *testing.T) {
		logger, logs := testLogger()
		installed := []rootio.Package{{Name: "lodash", Version: "4.17.20"}}
		apiErr := errors.New("boom")
		client := &fakeClient{err: apiErr}
		plan := &Plan{Packages: []Entry{{Ecosystem: "npm", Name: "lodash", Action: "upgrade", Version: "4.17.21"}}}
		_, err := Analyze(context.Background(), logger, client, plan, "npm", installed, nil)
		require.ErrorIs(t, err, apiErr)

		client = &fakeClient{resp: &rootio.AnalyzePackagesResponse{}}
		bad := &Plan{Packages: []Entry{{Ecosystem: "npm", Name: "lodash", Action: "pin", Version: "1"}}}
		_, err = Analyze(context.Background(), logger, client, bad, "npm", installed, nil)
		require.Error(t, err)
		require.Zero(t, client.calls)

		client = &fakeClient{resp: &rootio.AnalyzePackagesResponse{}}
		ghost := &Plan{Packages: []Entry{{Ecosystem: "npm", Name: "ghost", From: "4.0.0", Action: "upgrade", Version: "1.0.0"}}}
		got, err := Analyze(context.Background(), logger, client, ghost, "npm", []rootio.Package{{Name: "ms", Version: "2.0.0"}}, nil)
		require.NoError(t, err)
		require.Equal(t, 1, client.calls)
		require.Empty(t, client.ignore)
		require.Empty(t, got.Patches)
		require.Contains(t, logs.String(), `msg="Package skipped" package=ghost from=4.0.0 to=1.0.0 source=plan reason="no installed version matches"`)
	})
}

func TestResolve(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, DefaultFileName)

	plan, err := Resolve("", dir)
	require.NoError(t, err)
	require.Nil(t, plan)

	_, err = Resolve(filepath.Join(dir, "nope.json"), dir)
	require.ErrorIs(t, err, os.ErrNotExist)

	require.NoError(t, os.WriteFile(path, []byte(`{"packages":[{"ecosystem":"npm","name":"a","action":"upgrade","version":"1"}]}`), 0o644))
	plan, err = Resolve("", dir)
	require.NoError(t, err)
	require.Len(t, plan.Packages, 1)

	require.NoError(t, os.WriteFile(path, []byte(`{`), 0o644))
	_, err = Resolve("", dir)
	require.Error(t, err)
}

func TestAnalyze_LogsOneLinePerPackageWithFromAndTo(t *testing.T) {
	client := &fakeClient{resp: &rootio.AnalyzePackagesResponse{
		Patches: []rootio.PackagePatch{apiPatch("express", "4.18.2", "4.18.2-root.io.1")},
	}}
	logger, logs := testLogger()
	plan := &Plan{Packages: []Entry{
		{Ecosystem: "npm", Name: "lodash", Action: "upgrade", Version: "4.17.21"},
		{Ecosystem: "npm", Name: "ms", Action: "upgrade", Version: "2.1.3"},
	}}
	installed := []rootio.Package{
		{Name: "lodash", Version: "4.17.20"},
		{Name: "ms", Version: "2.1.3"},
		{Name: "express", Version: "4.18.2"},
	}
	_, err := Analyze(context.Background(), logger, client, plan, "npm", installed, nil)
	require.NoError(t, err)
	for msg, want := range map[string]string{
		"Package upgraded": `package=lodash from=4.17.20 to=4.17.21 source=plan`,
		"Package patched":  `package=express from=4.18.2 to=4.18.2-root.io.1 source=api`,
		"Package skipped":  `package=ms from=2.1.3 to=2.1.3 source=plan reason="already at plan version"`,
	} {
		line := `msg="` + msg + `" ` + want
		require.Equal(t, 1, strings.Count(logs.String(), line), logs.String())
	}
}
