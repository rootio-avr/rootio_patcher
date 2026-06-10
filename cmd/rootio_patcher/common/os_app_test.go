package common

import (
	"reflect"
	"testing"

	rootio "rootio_patcher/pkg/rootio"
)

// helpers

func installedPkgs(names ...string) []InstalledPackage {
	pkgs := make([]InstalledPackage, len(names))
	for i, n := range names {
		pkgs[i] = InstalledPackage{Name: n, Version: "1.0"}
	}
	return pkgs
}

func patchFor(names ...string) []rootio.PackagePatch {
	patches := make([]rootio.PackagePatch, len(names))
	for i, n := range names {
		patches[i] = rootio.PackagePatch{PackageName: n, Version: "1.0"}
	}
	return patches
}

func makeIgnoreSet(entries ...string) map[string]struct{} {
	m := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		m[e] = struct{}{}
	}
	return m
}

// ── computeUpgradeSet ────────────────────────────────────────────────────────

func TestComputeUpgradeSet(t *testing.T) {
	tests := []struct {
		name      string
		installed []InstalledPackage
		patches   []rootio.PackagePatch
		blacklist map[string]bool
		ignore    map[string]struct{}
		want      []string
	}{
		{
			name:      "empty inputs",
			installed: nil,
			patches:   nil,
			blacklist: nil,
			ignore:    nil,
			want:      []string{},
		},
		{
			name:      "all packages qualify",
			installed: installedPkgs("curl", "bash"),
			patches:   nil,
			blacklist: nil,
			ignore:    nil,
			want:      []string{"bash", "curl"}, // sorted
		},
		{
			name:      "patched package excluded",
			installed: installedPkgs("curl", "bash", "openssl"),
			patches:   patchFor("openssl"),
			blacklist: nil,
			ignore:    nil,
			want:      []string{"bash", "curl"},
		},
		{
			name:      "blacklisted package excluded",
			installed: installedPkgs("curl", "bash", "libc6"),
			patches:   nil,
			blacklist: map[string]bool{"libc6": true},
			ignore:    nil,
			want:      []string{"bash", "curl"},
		},
		{
			name:      "ignored by name@version",
			installed: installedPkgs("curl", "bash"),
			patches:   nil,
			blacklist: nil,
			ignore:    makeIgnoreSet("bash@1.0"),
			want:      []string{"curl"},
		},
		{
			name:      "ignored by bare name (no @)",
			installed: installedPkgs("curl", "bash"),
			patches:   nil,
			blacklist: nil,
			ignore:    makeIgnoreSet("bash"),
			want:      []string{"curl"},
		},
		{
			// installedPkgs sets version "1.0"; ignoring "bash@2.0" still
			// excludes the installed bash@1.0 because ignore is by NAME only
			// (the version in the ignore key is stripped and not matched).
			name:      "ignore matches by name regardless of version",
			installed: installedPkgs("curl", "bash"),
			patches:   nil,
			blacklist: nil,
			ignore:    makeIgnoreSet("bash@2.0"),
			want:      []string{"curl"},
		},
		{
			name:      "ignore key with multiple @ uses last",
			installed: installedPkgs("pkg@extra", "other"),
			patches:   nil,
			blacklist: nil,
			ignore:    makeIgnoreSet("pkg@extra@2.0"), // name = "pkg@extra", version = "2.0"
			want:      []string{"other"},
		},
		{
			name:      "deduplication: same name listed twice",
			installed: []InstalledPackage{{Name: "curl", Version: "1.0"}, {Name: "curl", Version: "1.0"}},
			patches:   nil,
			blacklist: nil,
			ignore:    nil,
			want:      []string{"curl"},
		},
		{
			name:      "sorted ascending",
			installed: installedPkgs("zzz", "aaa", "mmm"),
			patches:   nil,
			blacklist: nil,
			ignore:    nil,
			want:      []string{"aaa", "mmm", "zzz"},
		},
		{
			name:      "all excluded: patch+blacklist+ignore",
			installed: installedPkgs("openssl", "libc6", "bash"),
			patches:   patchFor("openssl"),
			blacklist: map[string]bool{"libc6": true},
			ignore:    makeIgnoreSet("bash@1.0"),
			want:      []string{},
		},
		{
			name:      "patched package not offered for upgrade",
			installed: installedPkgs("curl"),
			patches:   patchFor("curl"),
			blacklist: nil,
			ignore:    nil,
			want:      []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := computeUpgradeSet(tc.installed, tc.patches, tc.blacklist, tc.ignore)
			if got == nil {
				got = []string{}
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("computeUpgradeSet() = %v, want %v", got, tc.want)
			}
		})
	}
}

// ── filterPatches ────────────────────────────────────────────────────────────

func TestFilterPatches(t *testing.T) {
	tests := []struct {
		name    string
		patches []rootio.PackagePatch
		ignore  map[string]struct{}
		want    []rootio.PackagePatch
	}{
		{
			name:    "nil patches",
			patches: nil,
			ignore:  makeIgnoreSet("bash@1.0"),
			want:    nil,
		},
		{
			name:    "empty ignore set returns all patches unchanged",
			patches: patchFor("curl", "bash"),
			ignore:  nil,
			want:    patchFor("curl", "bash"),
		},
		{
			name:    "drop patch matching name@version ignore",
			patches: patchFor("bash", "curl"),
			ignore:  makeIgnoreSet("bash@1.0"),
			want:    patchFor("curl"),
		},
		{
			name:    "drop patch matching bare-name ignore",
			patches: patchFor("bash", "curl"),
			ignore:  makeIgnoreSet("bash"),
			want:    patchFor("curl"),
		},
		{
			// patchFor sets version "1.0"; ignoring "bash@9.9" still drops the
			// bash patch because matching is by NAME only (version stripped).
			name:    "drop patch by name regardless of ignore version",
			patches: patchFor("bash", "curl"),
			ignore:  makeIgnoreSet("bash@9.9"),
			want:    patchFor("curl"),
		},
		{
			name:    "preserve input order",
			patches: patchFor("zzz", "aaa", "mmm"),
			ignore:  makeIgnoreSet("aaa"),
			want:    patchFor("zzz", "mmm"),
		},
		{
			name:    "drop multiple ignored patches",
			patches: patchFor("a", "b", "c", "d"),
			ignore:  makeIgnoreSet("b@1.0", "d"),
			want:    patchFor("a", "c"),
		},
		{
			name:    "all patches ignored returns empty slice",
			patches: patchFor("curl", "bash"),
			ignore:  makeIgnoreSet("curl", "bash"),
			want:    []rootio.PackagePatch{},
		},
		{
			name:    "none match ignore set — all returned in order",
			patches: patchFor("curl", "bash"),
			ignore:  makeIgnoreSet("openssl@2.0"),
			want:    patchFor("curl", "bash"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := filterPatches(tc.patches, tc.ignore)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("filterPatches() = %v, want %v", got, tc.want)
			}
		})
	}
}
