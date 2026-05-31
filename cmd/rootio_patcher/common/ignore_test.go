package common_test

import (
	"os"
	"path/filepath"
	"testing"

	"rootio_patcher/cmd/rootio_patcher/common"
)

func TestLoadIgnoreList_EmptyFileAndNoFlags(t *testing.T) {
	set := common.LoadIgnoreList("nonexistent/.rootioignore", nil)
	if len(set) != 0 {
		t.Fatalf("expected empty set, got %d entries", len(set))
	}
}

func TestLoadIgnoreList_FileOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".rootioignore")
	content := "# comment\n\nexpress@4.17.1\nlodash@4.17.20\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	set := common.LoadIgnoreList(path, nil)
	if len(set) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(set))
	}
	if _, ok := set["express@4.17.1"]; !ok {
		t.Error("expected express@4.17.1 in set")
	}
	if _, ok := set["lodash@4.17.20"]; !ok {
		t.Error("expected lodash@4.17.20 in set")
	}
}

func TestLoadIgnoreList_FlagsOnly(t *testing.T) {
	set := common.LoadIgnoreList("nonexistent/.rootioignore", []string{"django@3.2.0"})
	if _, ok := set["django@3.2.0"]; !ok {
		t.Error("expected django@3.2.0 in set")
	}
}

func TestLoadIgnoreList_MergeDeduplicates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".rootioignore")
	if err := os.WriteFile(path, []byte("express@4.17.1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	set := common.LoadIgnoreList(path, []string{"express@4.17.1", "lodash@4.17.20"})
	if len(set) != 2 {
		t.Fatalf("expected 2 entries after dedup, got %d", len(set))
	}
}

func TestIgnoreListToPackages_Empty(t *testing.T) {
	result := common.IgnoreListToPackages(map[string]struct{}{})
	if len(result) != 0 {
		t.Fatalf("expected empty slice, got %d entries", len(result))
	}
}

func TestIgnoreListToPackages_SingleEntry(t *testing.T) {
	set := map[string]struct{}{"express@4.17.1": {}}
	result := common.IgnoreListToPackages(set)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	if result[0].Name != "express" || result[0].Version != "4.17.1" {
		t.Errorf("unexpected package: %+v", result[0])
	}
}

func TestIgnoreListToPackages_MultipleEntries(t *testing.T) {
	set := map[string]struct{}{
		"express@4.17.1": {},
		"lodash@4.17.20": {},
	}
	result := common.IgnoreListToPackages(set)
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}
	found := map[string]bool{}
	for _, p := range result {
		found[p.Name+"@"+p.Version] = true
	}
	if !found["express@4.17.1"] || !found["lodash@4.17.20"] {
		t.Errorf("missing expected entries in result: %+v", result)
	}
}
