package upgradeplan

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"rootio_patcher/pkg/rootio"
)

func TestLoad_ExamplePlan(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, DefaultFileName)
	body := `{
	  "source": "fixer",
	  "packages": [
	    {"ecosystem": "npm", "name": "lodash", "action": "upgrade", "version": "4.17.21", "reason": "no breaking API use found"},
	    {"ecosystem": "npm", "name": "ms", "action": "upgrade", "version": "2.2.2+root.io.1"}
	  ]
	}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	plan, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(plan.Packages) != 2 {
		t.Fatalf("packages = %d, want 2", len(plan.Packages))
	}
	if plan.Packages[0].Name != "lodash" || plan.Packages[0].Action != ActionUpgrade || plan.Packages[0].Version != "4.17.21" {
		t.Fatalf("lodash entry = %+v", plan.Packages[0])
	}
	if plan.Packages[1].Name != "ms" || plan.Packages[1].Version != "2.2.2+root.io.1" {
		t.Fatalf("ms entry = %+v", plan.Packages[1])
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if !os.IsNotExist(err) {
		t.Fatalf("err = %v, want not exist", err)
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultFileName)
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestLoad_EmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultFileName)
	if err := os.WriteFile(path, []byte(" \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected empty-file error")
	}
}

func TestSplit_InvalidEntryForEcosystemFails(t *testing.T) {
	cases := []struct {
		name    string
		entry   Entry
		wantErr string
	}{
		{
			name:    "unknown action",
			entry:   Entry{Ecosystem: "npm", Name: "lodash", Action: "pin", Version: "1.0.0"},
			wantErr: `upgrade plan package "lodash": action "pin" is not supported, use "upgrade"`,
		},
		{
			name:    "leave is no longer an action",
			entry:   Entry{Ecosystem: "npm", Name: "lodash", Action: "leave"},
			wantErr: `upgrade plan package "lodash": action "leave" is not supported, use "upgrade"`,
		},
		{
			name:    "upgrade missing version",
			entry:   Entry{Ecosystem: "npm", Name: "lodash", Action: "upgrade"},
			wantErr: `upgrade plan package "lodash": upgrade requires a version`,
		},
		{
			name:    "empty name",
			entry:   Entry{Ecosystem: "npm", Name: "  ", Action: "upgrade", Version: "1"},
			wantErr: "upgrade plan entry 0: package name is empty",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := (&Plan{Packages: []Entry{tc.entry}}).Split("npm", nil)
			if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestSplit_EcosystemAndActionAreCaseInsensitive(t *testing.T) {
	plan := &Plan{Packages: []Entry{{Ecosystem: "NPM", Name: "lodash", Action: "Upgrade", Version: " 4.17.21 "}}}
	got, err := plan.Split(" Npm ", []Package{{Name: "lodash", Version: "4.17.20"}})
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(got.Patches) != 1 || got.Patches[0].Patch.Version != "4.17.21" {
		t.Fatalf("patches = %+v", got.Patches)
	}
}

func TestSplit_EmptyPlan(t *testing.T) {
	got, err := (&Plan{}).Split("npm", []Package{{Name: "lodash", Version: "1.0.0"}})
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(got.Patches) != 0 || len(got.Ignore) != 0 || len(got.Absent) != 0 {
		t.Fatalf("result = %+v, want empty", got)
	}
}

func TestSplit_PlannedPatchesAndIgnore(t *testing.T) {
	plan := &Plan{Packages: []Entry{
		{Ecosystem: "npm", Name: "lodash", Action: "upgrade", Version: "4.17.21-aikido.2"},
		{Ecosystem: "npm", Name: "ms", Action: "upgrade", Version: "2.1.3", Reason: "safe"},
		{Ecosystem: "pypi", Name: "requests", Action: "upgrade", Version: "2.32.0"},
		{Ecosystem: "npm", Name: "absent", Action: "upgrade", Version: "1.0.0"},
	}}
	installed := []Package{
		{Name: "lodash", Version: "4.17.20"},
		{Name: "ms", Version: "2.0.0"},
		{Name: "express", Version: "4.18.2"},
		{Name: "requests", Version: "2.0.0"},
	}

	got, err := plan.Split("npm", installed)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}

	wantPatches := []rootio.PackagePatch{
		{PackageName: "lodash", Version: "4.17.20", Patch: rootio.PatchInfo{Name: "lodash", Version: "4.17.21-aikido.2"}},
		{PackageName: "ms", Version: "2.0.0", Patch: rootio.PatchInfo{Name: "ms", Version: "2.1.3"}},
	}
	if !reflect.DeepEqual(got.Patches, wantPatches) {
		t.Fatalf("patches = %+v, want %+v", got.Patches, wantPatches)
	}
	wantIgnore := []rootio.Package{{Name: "lodash", Version: "4.17.20"}, {Name: "ms", Version: "2.0.0"}}
	if !reflect.DeepEqual(got.Ignore, wantIgnore) {
		t.Fatalf("ignore = %+v, want %+v", got.Ignore, wantIgnore)
	}
	if !reflect.DeepEqual(got.Absent, []string{"absent"}) {
		t.Fatalf("absent = %v", got.Absent)
	}
}

func TestSplit_VersionFormsAreKeptExactly(t *testing.T) {
	for _, version := range []string{"1.6.0", "4.17.21-aikido.2", "2.2.2+root.io.1", "v1.2.3", "1.0.0rc1"} {
		plan := &Plan{Packages: []Entry{{Ecosystem: "npm", Name: "pkg", Action: "upgrade", Version: version}}}
		got, err := plan.Split("npm", []Package{{Name: "pkg", Version: "0.0.1"}})
		if err != nil {
			t.Fatalf("Split(%s): %v", version, err)
		}
		if len(got.Patches) != 1 || got.Patches[0].Patch.Version != version {
			t.Fatalf("version %q patches = %+v", version, got.Patches)
		}
	}
}

func TestSplit_MultipleInstalledVersionsAndDuplicates(t *testing.T) {
	plan := &Plan{Packages: []Entry{
		{Ecosystem: "npm", Name: "lodash", Action: "upgrade", Version: "4.17.20"},
		{Ecosystem: "npm", Name: "lodash", Action: "upgrade", Version: "4.17.21"},
	}}
	installed := []Package{
		{Name: "lodash", Version: "4.17.15"},
		{Name: "lodash", Version: "4.17.15"},
		{Name: "lodash", Version: "3.10.1"},
	}
	got, err := plan.Split("npm", installed)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(got.Patches) != 2 || got.Patches[0].Version != "4.17.15" || got.Patches[1].Version != "3.10.1" {
		t.Fatalf("patches = %+v", got.Patches)
	}
	for _, patch := range got.Patches {
		if patch.Patch.Version != "4.17.21" {
			t.Fatalf("last duplicate entry should win, got %+v", patch)
		}
	}
	if len(got.Ignore) != 2 {
		t.Fatalf("ignore = %+v", got.Ignore)
	}
}

func TestSplit_AlreadyAtPlanVersionIsIgnoredWithoutPatch(t *testing.T) {
	plan := &Plan{Packages: []Entry{{Ecosystem: "npm", Name: "ms", Action: "upgrade", Version: "2.1.3"}}}
	got, err := plan.Split("npm", []Package{{Name: "ms", Version: "2.1.3"}})
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(got.Patches) != 0 {
		t.Fatalf("patches = %+v, want none", got.Patches)
	}
	if !reflect.DeepEqual(got.Ignore, []rootio.Package{{Name: "ms", Version: "2.1.3"}}) {
		t.Fatalf("ignore = %+v", got.Ignore)
	}
	if !reflect.DeepEqual(got.Current, []string{"ms"}) {
		t.Fatalf("current = %v", got.Current)
	}
}

func TestSplit_OtherEcosystemEntriesAreNotValidated(t *testing.T) {
	plan := &Plan{Packages: []Entry{{Ecosystem: "maven", Name: "org.example:lib", Action: "pin"}}}
	got, err := plan.Split("npm", []Package{{Name: "lodash", Version: "1.0.0"}})
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(got.Patches) != 0 || len(got.Ignore) != 0 {
		t.Fatalf("result = %+v, want empty", got)
	}
}

func TestSplit_NilPlan(t *testing.T) {
	if _, err := (*Plan)(nil).Split("npm", nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestSplit_FromScopesEntryToInstalledVersion(t *testing.T) {
	plan := &Plan{Packages: []Entry{
		{Ecosystem: "npm", Name: "lodash", From: "4.17.15", Action: "upgrade", Version: "4.17.21"},
	}}
	installed := []Package{
		{Name: "lodash", Version: "3.10.1"},
		{Name: "lodash", Version: "4.17.15"},
	}
	got, err := plan.Split("npm", installed)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	wantPatches := []rootio.PackagePatch{
		{PackageName: "lodash", Version: "4.17.15", Patch: rootio.PatchInfo{Name: "lodash", Version: "4.17.21"}},
	}
	if !reflect.DeepEqual(got.Patches, wantPatches) {
		t.Fatalf("patches = %+v, want %+v", got.Patches, wantPatches)
	}
	if !reflect.DeepEqual(got.Ignore, []rootio.Package{{Name: "lodash", Version: "4.17.15"}}) {
		t.Fatalf("ignore = %+v, want only the from version", got.Ignore)
	}
	if len(got.Absent) != 0 {
		t.Fatalf("absent = %v, want none", got.Absent)
	}
}

func TestSplit_FromNotInstalledIsAbsent(t *testing.T) {
	plan := &Plan{Packages: []Entry{
		{Ecosystem: "npm", Name: "lodash", From: "4.17.15", Action: "upgrade", Version: "4.17.21"},
	}}
	got, err := plan.Split("npm", []Package{{Name: "lodash", Version: "3.10.1"}})
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(got.Patches) != 0 || len(got.Ignore) != 0 {
		t.Fatalf("result = %+v, want nothing planned", got)
	}
	if !reflect.DeepEqual(got.Absent, []string{"lodash"}) {
		t.Fatalf("absent = %v", got.Absent)
	}
}

func TestSplit_SameNameDifferentFromAllApply(t *testing.T) {
	plan := &Plan{Packages: []Entry{
		{Ecosystem: "npm", Name: "lodash", From: "4.17.15", Action: "upgrade", Version: "4.17.21"},
		{Ecosystem: "npm", Name: "lodash", From: "3.10.1", Action: "upgrade", Version: "3.10.2"},
	}}
	installed := []Package{
		{Name: "lodash", Version: "3.10.1"},
		{Name: "lodash", Version: "4.17.15"},
		{Name: "lodash", Version: "2.0.0"},
	}
	got, err := plan.Split("npm", installed)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	wantPatches := []rootio.PackagePatch{
		{PackageName: "lodash", Version: "3.10.1", Patch: rootio.PatchInfo{Name: "lodash", Version: "3.10.2"}},
		{PackageName: "lodash", Version: "4.17.15", Patch: rootio.PatchInfo{Name: "lodash", Version: "4.17.21"}},
	}
	if !reflect.DeepEqual(got.Patches, wantPatches) {
		t.Fatalf("patches = %+v, want %+v", got.Patches, wantPatches)
	}
	wantIgnore := []rootio.Package{{Name: "lodash", Version: "3.10.1"}, {Name: "lodash", Version: "4.17.15"}}
	if !reflect.DeepEqual(got.Ignore, wantIgnore) {
		t.Fatalf("ignore = %+v, want %+v", got.Ignore, wantIgnore)
	}
}

func TestSplit_DuplicateNameAndFromLastWins(t *testing.T) {
	plan := &Plan{Packages: []Entry{
		{Ecosystem: "npm", Name: "lodash", From: "4.17.15", Action: "upgrade", Version: "4.17.20"},
		{Ecosystem: "npm", Name: "lodash", From: "4.17.15", Action: "upgrade", Version: "4.17.21"},
	}}
	got, err := plan.Split("npm", []Package{{Name: "lodash", Version: "4.17.15"}})
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(got.Patches) != 1 || got.Patches[0].Patch.Version != "4.17.21" {
		t.Fatalf("patches = %+v, want the last entry", got.Patches)
	}
}

func TestSplit_FromEntryBeatsAllVersionsEntry(t *testing.T) {
	plan := &Plan{Packages: []Entry{
		{Ecosystem: "npm", Name: "lodash", Action: "upgrade", Version: "4.17.21"},
		{Ecosystem: "npm", Name: "lodash", From: "3.10.1", Action: "upgrade", Version: "3.10.2"},
	}}
	installed := []Package{{Name: "lodash", Version: "3.10.1"}, {Name: "lodash", Version: "4.17.15"}}
	got, err := plan.Split("npm", installed)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(got.Patches) != 2 || got.Patches[0].Patch.Version != "3.10.2" || got.Patches[1].Patch.Version != "4.17.21" {
		t.Fatalf("patches = %+v", got.Patches)
	}
}

func TestSplit_FromIsTrimmedAndEmptyFromAppliesToAll(t *testing.T) {
	plan := &Plan{Packages: []Entry{{Ecosystem: "npm", Name: "lodash", From: " 4.17.15 ", Action: "upgrade", Version: "4.17.21"}}}
	got, err := plan.Split("npm", []Package{{Name: "lodash", Version: "4.17.15"}})
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(got.Patches) != 1 {
		t.Fatalf("patches = %+v", got.Patches)
	}
}

func TestLoad_FromField(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultFileName)
	body := `{"packages":[{"ecosystem":"pypi","name":"ms","from":"2.0.0","action":"upgrade","version":"2.1.3"}]}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	plan, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if plan.Packages[0].From != "2.0.0" {
		t.Fatalf("entry = %+v", plan.Packages[0])
	}
}

func TestSplit_NamesMatchPerEcosystem(t *testing.T) {
	cases := []struct {
		ecosystem string
		planName  string
		installed string
		matches   bool
	}{
		{"pypi", "jinja2", "Jinja2", true},
		{"pypi", "Jinja2", "jinja2", true},
		{"pypi", "typing_extensions", "typing-extensions", true},
		{"pypi", "ruamel.yaml", "ruamel-yaml", true},
		{"pypi", "a-_.b", "a--b", true},
		{"pypi", "rootio_django", "rootio-django", true},
		{"pypi", "jinja2", "jinja3", false},
		{"nuget", "newtonsoft.json", "Newtonsoft.Json", true},
		{"nuget", "Newtonsoft.Json", "Newtonsoft.Xml", false},
		{"composer", "Monolog/Monolog", "monolog/monolog", true},
		{"composer", "monolog/monolog", "monolog/other", false},
		{"npm", "Lodash", "lodash", false},
		{"npm", "lodash", "lodash", true},
		{"golang", "github.com/Azure/go", "github.com/azure/go", false},
		{"golang", "github.com/Azure/go", "github.com/Azure/go", true},
		{"maven", "Junit:junit", "junit:junit", false},
		{"maven", "junit:junit", "junit:junit", true},
		{"ruby", "Rack", "rack", false},
		{"ruby", "rack", "rack", true},
	}
	for _, tc := range cases {
		t.Run(tc.ecosystem+"/"+tc.planName+"/"+tc.installed, func(t *testing.T) {
			plan := &Plan{Packages: []Entry{{Ecosystem: tc.ecosystem, Name: tc.planName, Action: "upgrade", Version: "9.9.9"}}}
			got, err := plan.Split(tc.ecosystem, []Package{{Name: tc.installed, Version: "1.0.0"}})
			if err != nil {
				t.Fatalf("Split: %v", err)
			}
			if !tc.matches {
				if len(got.Patches) != 0 || len(got.Ignore) != 0 {
					t.Fatalf("result = %+v, want no match", got)
				}
				return
			}
			want := []rootio.PackagePatch{{
				PackageName: tc.installed, Version: "1.0.0",
				Patch: rootio.PatchInfo{Name: tc.installed, Version: "9.9.9"},
			}}
			if !reflect.DeepEqual(got.Patches, want) {
				t.Fatalf("patches = %+v, want %+v (installed name emitted)", got.Patches, want)
			}
			if !reflect.DeepEqual(got.Ignore, []rootio.Package{{Name: tc.installed, Version: "1.0.0"}}) {
				t.Fatalf("ignore = %+v, want installed name", got.Ignore)
			}
		})
	}
}

func TestSplit_PipDuplicateEntriesUnderNormalizationLastWins(t *testing.T) {
	plan := &Plan{Packages: []Entry{
		{Ecosystem: "pypi", Name: "Jinja2", Action: "upgrade", Version: "3.1.3"},
		{Ecosystem: "pypi", Name: "jinja2", Action: "upgrade", Version: "3.1.4"},
	}}
	got, err := plan.Split("pypi", []Package{{Name: "Jinja2", Version: "3.0.0"}})
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(got.Patches) != 1 || got.Patches[0].Patch.Version != "3.1.4" {
		t.Fatalf("patches = %+v, want the last entry", got.Patches)
	}
}

func TestSplit_SubcommandEcosystemNamesAreAliases(t *testing.T) {
	cases := []struct{ entryEcosystem, splitEcosystem string }{
		{"pip", "pypi"},
		{"pypi", "pypi"},
		{"go", "golang"},
		{"golang", "golang"},
		{"PIP", " pypi "},
	}
	for _, tc := range cases {
		plan := &Plan{Packages: []Entry{{Ecosystem: tc.entryEcosystem, Name: "pkg", Action: "upgrade", Version: "2"}}}
		got, err := plan.Split(tc.splitEcosystem, []Package{{Name: "pkg", Version: "1"}})
		if err != nil || len(got.Patches) != 1 {
			t.Fatalf("entry %q split %q: patches = %+v err = %v", tc.entryEcosystem, tc.splitEcosystem, got.Patches, err)
		}
	}
	other := &Plan{Packages: []Entry{{Ecosystem: "go", Name: "pkg", Action: "upgrade", Version: "2"}}}
	got, err := other.Split("pypi", []Package{{Name: "pkg", Version: "1"}})
	if err != nil || len(got.Patches) != 0 {
		t.Fatalf("go entry applied to pypi: %+v err = %v", got.Patches, err)
	}
}
