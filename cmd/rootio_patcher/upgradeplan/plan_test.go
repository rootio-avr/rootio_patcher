package upgradeplan

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoad_ExamplePlan(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, DefaultFileName)
	body := `{
	  "source": "fixer",
	  "packages": [
	    {"ecosystem": "npm", "name": "lodash", "action": "upgrade", "version": "4.17.21", "reason": "no breaking API use found"},
	    {"ecosystem": "npm", "name": "left-pad", "action": "leave", "reason": "upgrade decision unknown"}
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
	if plan.Packages[1].Name != "left-pad" || plan.Packages[1].Action != ActionLeave {
		t.Fatalf("left-pad entry = %+v", plan.Packages[1])
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

func TestDecide_UpgradeAndLeave(t *testing.T) {
	plan := &Plan{Packages: []Entry{
		{Ecosystem: "npm", Name: "lodash", Action: "upgrade", Version: "4.17.21", Reason: "no breaking API use found"},
		{Ecosystem: "npm", Name: "left-pad", Action: "leave", Reason: "upgrade decision unknown"},
		{Ecosystem: "pypi", Name: "requests", Action: "not-an-action"},
		{Ecosystem: "maven", Name: "org.example:lib", Action: "upgrade"},
	}}
	installed := []Package{
		{Name: "lodash", Version: "4.17.20"},
		{Name: "left-pad", Version: "1.3.0"},
		{Name: "express", Version: "4.18.2"},
	}

	got, err := plan.Decide("npm", installed)
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}

	wantUpgrades := []Upgrade{{
		Name: "lodash", InstalledVersion: "4.17.20", Version: "4.17.21", Reason: "no breaking API use found",
	}}
	if !reflect.DeepEqual(got.Upgrades, wantUpgrades) {
		t.Fatalf("upgrades = %+v, want %+v", got.Upgrades, wantUpgrades)
	}
	wantLeaves := []Leave{{
		Name: "left-pad", InstalledVersion: "1.3.0", Reason: "upgrade decision unknown",
	}}
	if !reflect.DeepEqual(got.Leaves, wantLeaves) {
		t.Fatalf("leaves = %+v, want %+v", got.Leaves, wantLeaves)
	}
	if !reflect.DeepEqual(got.IgnoredEcosystems, []string{"pypi", "maven"}) {
		t.Fatalf("ignored = %v", got.IgnoredEcosystems)
	}

	resp := got.AnalyzeResponse()
	if len(resp.Patches) != 1 {
		t.Fatalf("patches = %d, want 1", len(resp.Patches))
	}
	patch := resp.Patches[0]
	if patch.PackageName != "lodash" || patch.Version != "4.17.20" || patch.Patch.Name != "lodash" || patch.Patch.Version != "4.17.21" {
		t.Fatalf("patch = %+v", patch)
	}
	if len(resp.Skipped) != 1 || resp.Skipped[0].PackageName != "left-pad" || resp.Skipped[0].Reason != "leave: upgrade decision unknown" {
		t.Fatalf("skipped = %+v", resp.Skipped)
	}
}

func TestDecide_UnknownEcosystemDoesNotFail(t *testing.T) {
	plan := &Plan{Packages: []Entry{
		{Ecosystem: "hex", Name: "poison", Action: "explode", Version: ""},
		{Ecosystem: "NPM", Name: "lodash", Action: "Upgrade", Version: " 4.17.21 "},
	}}
	got, err := plan.Decide("npm", []Package{{Name: "lodash", Version: "4.17.20"}})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if len(got.Upgrades) != 1 || got.Upgrades[0].Version != "4.17.21" {
		t.Fatalf("upgrades = %+v", got.Upgrades)
	}
	if !reflect.DeepEqual(got.IgnoredEcosystems, []string{"hex"}) {
		t.Fatalf("ignored = %v", got.IgnoredEcosystems)
	}
}

func TestDecide_InvalidNpmEntryFails(t *testing.T) {
	cases := []struct {
		name    string
		entry   Entry
		wantErr string
	}{
		{
			name:    "unknown action",
			entry:   Entry{Ecosystem: "npm", Name: "lodash", Action: "pin", Version: "1.0.0"},
			wantErr: `upgrade plan package "lodash": action "pin" is not upgrade or leave`,
		},
		{
			name:    "upgrade missing version",
			entry:   Entry{Ecosystem: "npm", Name: "lodash", Action: "upgrade"},
			wantErr: `upgrade plan package "lodash": upgrade requires a version`,
		},
		{
			name:    "empty name",
			entry:   Entry{Ecosystem: "npm", Name: "  ", Action: "leave"},
			wantErr: "upgrade plan entry 0: package name is empty",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := (&Plan{Packages: []Entry{tc.entry}}).Decide("npm", nil)
			if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestDecide_OtherEcosystemInvalidActionIsIgnored(t *testing.T) {
	plan := &Plan{Packages: []Entry{
		{Ecosystem: "maven", Name: "org.example:lib", Action: "pin"},
	}}
	got, err := plan.Decide("npm", []Package{{Name: "lodash", Version: "1.0.0"}})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if len(got.Upgrades) != 0 || len(got.Leaves) != 0 {
		t.Fatalf("result = %+v, want empty decisions", got)
	}
	if !reflect.DeepEqual(got.IgnoredEcosystems, []string{"maven"}) {
		t.Fatalf("ignored = %v", got.IgnoredEcosystems)
	}
}

func TestDecide_AlreadyAtVersionAndAbsentAndLastWins(t *testing.T) {
	plan := &Plan{Packages: []Entry{
		{Ecosystem: "npm", Name: "lodash", Action: "upgrade", Version: "4.17.20", Reason: "first"},
		{Ecosystem: "npm", Name: "lodash", Action: "upgrade", Version: "4.17.21", Reason: "last"},
		{Ecosystem: "npm", Name: "ms", Action: "upgrade", Version: "2.1.3"},
		{Ecosystem: "npm", Name: "left-pad", Action: "upgrade", Version: "1.3.0"},
		{Ecosystem: "npm", Name: "debug", Action: "leave"},
	}}
	installed := []Package{
		{Name: "lodash", Version: "4.17.20"},
		{Name: "lodash", Version: "4.17.20"},
		{Name: "lodash", Version: "4.17.15"},
		{Name: "left-pad", Version: "1.3.0"},
		{Name: "debug", Version: "4.3.4"},
		{Name: "debug", Version: "4.3.4"},
	}

	got, err := plan.Decide("npm", installed)
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}

	wantUpgrades := []Upgrade{
		{Name: "lodash", InstalledVersion: "4.17.20", Version: "4.17.21", Reason: "last"},
		{Name: "lodash", InstalledVersion: "4.17.15", Version: "4.17.21", Reason: "last"},
	}
	if !reflect.DeepEqual(got.Upgrades, wantUpgrades) {
		t.Fatalf("upgrades = %+v, want %+v", got.Upgrades, wantUpgrades)
	}
	wantLeaves := []Leave{{Name: "debug", InstalledVersion: "4.3.4"}}
	if !reflect.DeepEqual(got.Leaves, wantLeaves) {
		t.Fatalf("leaves = %+v, want %+v", got.Leaves, wantLeaves)
	}
	if !reflect.DeepEqual(got.AbsentPackageNames, []string{"ms"}) {
		t.Fatalf("absent = %v", got.AbsentPackageNames)
	}
	if len(got.AnalyzeResponse().Patches) != 2 {
		t.Fatalf("patches = %+v", got.AnalyzeResponse().Patches)
	}
}

func TestDecide_EmptyPlan(t *testing.T) {
	got, err := (&Plan{}).Decide("npm", []Package{{Name: "lodash", Version: "1.0.0"}})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if len(got.Upgrades) != 0 || len(got.Leaves) != 0 || len(got.IgnoredEcosystems) != 0 {
		t.Fatalf("result = %+v, want empty", got)
	}
}

func TestDecide_NilPlan(t *testing.T) {
	_, err := (*Plan)(nil).Decide("npm", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}
