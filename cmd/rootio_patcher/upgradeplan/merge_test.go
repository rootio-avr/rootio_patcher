package upgradeplan

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

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

func (f *fakeClient) AnalyzePackages(_ context.Context, packages []rootio.Package, ignore []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error) {
	f.calls++
	f.packages = packages
	f.ignore = ignore
	f.ecosys = ecosystem
	return f.resp, f.err
}

func apiPatch(name, from, to string) rootio.PackagePatch {
	return rootio.PackagePatch{PackageName: name, Version: from, Patch: rootio.PatchInfo{Name: name, Version: to}}
}

func testLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, nil)), &buf
}

func TestAnalyze_NoPlanCallsAPIAsToday(t *testing.T) {
	client := &fakeClient{resp: &rootio.AnalyzePackagesResponse{
		Patches: []rootio.PackagePatch{apiPatch("express", "4.18.2", "4.18.2-root.io.1")},
		Skipped: []rootio.SkippedPackage{{PackageName: "ms", Reason: "no fix"}},
	}}
	logger, _ := testLogger()
	installed := []rootio.Package{{Name: "express", Version: "4.18.2"}, {Name: "ms", Version: "2.0.0"}}
	user := []rootio.Package{{Name: "debug", Version: "1.0.0"}}

	got, err := Analyze(context.Background(), logger, client, nil, "npm", installed, user)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if client.calls != 1 || client.ecosys != "npm" {
		t.Fatalf("calls = %d ecosystem = %q", client.calls, client.ecosys)
	}
	if !reflect.DeepEqual(client.packages, installed) || !reflect.DeepEqual(client.ignore, user) {
		t.Fatalf("api got packages %+v ignore %+v", client.packages, client.ignore)
	}
	if !reflect.DeepEqual(got, client.resp) {
		t.Fatalf("response = %+v, want %+v", got, client.resp)
	}
}

func TestAnalyze_MergesPlannedAndAPIPatches(t *testing.T) {
	client := &fakeClient{resp: &rootio.AnalyzePackagesResponse{
		Patches: []rootio.PackagePatch{apiPatch("express", "4.18.2", "4.18.2-root.io.1")},
		Skipped: []rootio.SkippedPackage{{PackageName: "ms", Reason: "no fix"}},
	}}
	logger, logs := testLogger()
	plan := &Plan{Packages: []Entry{{Ecosystem: "npm", Name: "lodash", Action: "upgrade", Version: "4.17.21-aikido.2"}}}
	installed := []rootio.Package{
		{Name: "lodash", Version: "4.17.20"},
		{Name: "express", Version: "4.18.2"},
		{Name: "ms", Version: "2.0.0"},
	}
	user := []rootio.Package{{Name: "debug", Version: "1.0.0"}}

	got, err := Analyze(context.Background(), logger, client, plan, "npm", installed, user)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	wantIgnore := []rootio.Package{{Name: "debug", Version: "1.0.0"}, {Name: "lodash", Version: "4.17.20"}}
	if !reflect.DeepEqual(client.ignore, wantIgnore) {
		t.Fatalf("api ignore = %+v, want %+v", client.ignore, wantIgnore)
	}
	wantPatches := []rootio.PackagePatch{
		apiPatch("lodash", "4.17.20", "4.17.21-aikido.2"),
		apiPatch("express", "4.18.2", "4.18.2-root.io.1"),
	}
	if !reflect.DeepEqual(got.Patches, wantPatches) {
		t.Fatalf("patches = %+v, want %+v", got.Patches, wantPatches)
	}
	if !reflect.DeepEqual(got.Skipped, client.resp.Skipped) {
		t.Fatalf("skipped = %+v", got.Skipped)
	}
	out := logs.String()
	for _, want := range []string{
		`msg="Package upgraded" package=lodash from=4.17.20 to=4.17.21-aikido.2`,
		`msg="Package patched" package=express from=4.18.2 to=4.18.2-root.io.1`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("logs missing %q:\n%s", want, out)
		}
	}
}

func TestAnalyze_PlannedPackageNeverComesFromAPI(t *testing.T) {
	client := &fakeClient{resp: &rootio.AnalyzePackagesResponse{
		Patches: []rootio.PackagePatch{
			apiPatch("lodash", "4.17.20", "4.17.20-root.io.1"),
			apiPatch("lodash", "3.10.1", "3.10.1-root.io.1"),
		},
	}}
	logger, _ := testLogger()
	plan := &Plan{Packages: []Entry{{Ecosystem: "npm", Name: "lodash", Action: "upgrade", Version: "4.17.21"}}}
	installed := []rootio.Package{{Name: "lodash", Version: "4.17.20"}}

	got, err := Analyze(context.Background(), logger, client, plan, "npm", installed, nil)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	// The planned 4.17.20 patch wins. The 3.10.1 copy is not planned, so the API result stays.
	want := []rootio.PackagePatch{
		apiPatch("lodash", "4.17.20", "4.17.21"),
		apiPatch("lodash", "3.10.1", "3.10.1-root.io.1"),
	}
	if !reflect.DeepEqual(got.Patches, want) {
		t.Fatalf("patches = %+v, want %+v", got.Patches, want)
	}
}

func TestAnalyze_PlanWithNoInstalledMatchStillCallsAPI(t *testing.T) {
	client := &fakeClient{resp: &rootio.AnalyzePackagesResponse{}}
	logger, logs := testLogger()
	plan := &Plan{Packages: []Entry{{Ecosystem: "npm", Name: "ghost", Action: "upgrade", Version: "1.0.0"}}}

	got, err := Analyze(context.Background(), logger, client, plan, "npm", []rootio.Package{{Name: "ms", Version: "2.0.0"}}, nil)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if client.calls != 1 || len(client.ignore) != 0 || len(got.Patches) != 0 {
		t.Fatalf("calls = %d ignore = %+v patches = %+v", client.calls, client.ignore, got.Patches)
	}
	if !strings.Contains(logs.String(), "not installed") || !strings.Contains(logs.String(), "package=ghost") {
		t.Errorf("expected skip log for ghost:\n%s", logs.String())
	}
}

func TestAnalyze_IgnoreIsDeduplicated(t *testing.T) {
	client := &fakeClient{resp: &rootio.AnalyzePackagesResponse{}}
	logger, _ := testLogger()
	plan := &Plan{Packages: []Entry{{Ecosystem: "npm", Name: "lodash", Action: "upgrade", Version: "4.17.21"}}}
	user := []rootio.Package{{Name: "lodash", Version: "4.17.20"}}

	if _, err := Analyze(context.Background(), logger, client, plan, "npm", []rootio.Package{{Name: "lodash", Version: "4.17.20"}}, user); err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if !reflect.DeepEqual(client.ignore, user) {
		t.Fatalf("ignore = %+v, want one entry", client.ignore)
	}
}

func TestAnalyze_AlreadyAtPlanVersionIsNotPatched(t *testing.T) {
	client := &fakeClient{resp: &rootio.AnalyzePackagesResponse{}}
	logger, _ := testLogger()
	plan := &Plan{Packages: []Entry{{Ecosystem: "npm", Name: "ms", Action: "upgrade", Version: "2.1.3"}}}

	got, err := Analyze(context.Background(), logger, client, plan, "npm", []rootio.Package{{Name: "ms", Version: "2.1.3"}}, nil)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(got.Patches) != 0 || !reflect.DeepEqual(client.ignore, []rootio.Package{{Name: "ms", Version: "2.1.3"}}) {
		t.Fatalf("patches = %+v ignore = %+v", got.Patches, client.ignore)
	}
}

func TestAnalyze_Errors(t *testing.T) {
	logger, _ := testLogger()
	plan := &Plan{Packages: []Entry{{Ecosystem: "npm", Name: "lodash", Action: "upgrade", Version: "4.17.21"}}}
	installed := []rootio.Package{{Name: "lodash", Version: "4.17.20"}}

	apiErr := errors.New("boom")
	client := &fakeClient{err: apiErr}
	if _, err := Analyze(context.Background(), logger, client, plan, "npm", installed, nil); !errors.Is(err, apiErr) {
		t.Fatalf("err = %v, want wrapped API error", err)
	}

	bad := &Plan{Packages: []Entry{{Ecosystem: "npm", Name: "lodash", Action: "pin", Version: "1"}}}
	client = &fakeClient{resp: &rootio.AnalyzePackagesResponse{}}
	if _, err := Analyze(context.Background(), logger, client, bad, "npm", installed, nil); err == nil || client.calls != 0 {
		t.Fatalf("err = %v calls = %d, want plan error before any API call", err, client.calls)
	}
}

func TestResolve(t *testing.T) {
	dir := t.TempDir()
	defaultPath := filepath.Join(dir, DefaultFileName)

	t.Run("default missing means no plan", func(t *testing.T) {
		plan, err := Resolve("", dir)
		if err != nil || plan != nil {
			t.Fatalf("plan = %v err = %v, want nil nil", plan, err)
		}
	})
	t.Run("explicit missing is an error", func(t *testing.T) {
		_, err := Resolve(filepath.Join(dir, "nope.json"), dir)
		if err == nil || !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("err = %v, want not exist", err)
		}
	})
	t.Run("default present is loaded", func(t *testing.T) {
		if err := os.WriteFile(defaultPath, []byte(`{"packages":[{"ecosystem":"npm","name":"a","action":"upgrade","version":"1"}]}`), 0o644); err != nil {
			t.Fatal(err)
		}
		plan, err := Resolve("", dir)
		if err != nil || plan == nil || len(plan.Packages) != 1 {
			t.Fatalf("plan = %+v err = %v", plan, err)
		}
	})
	t.Run("default invalid is an error", func(t *testing.T) {
		if err := os.WriteFile(defaultPath, []byte(`{`), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Resolve("", dir); err == nil {
			t.Fatal("expected parse error")
		}
	})
}
