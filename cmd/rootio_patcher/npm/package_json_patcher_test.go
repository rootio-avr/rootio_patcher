package npm

import (
	"context"
	"os"
	"strings"
	"testing"

	"rootio_patcher/pkg/rootio"
)

func TestPackageJSONPatcher_SetsValuesAtPaths(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	pkgPath := tmpDir + "/package.json"

	pkg := `{
  "name": "test-app",
  "version": "1.0.0",
  "dependencies": {
    "lodash": "^4.17.20",
    "express": "^4.18.0"
  }
}`
	if err := os.WriteFile(pkgPath, []byte(pkg), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	patcher := NewPackageJSONPatcher()
	err := patcher.Patch(ctx, PatchOptions{
		PackageJSONPath: pkgPath,
		Sets: map[string]string{
			"overrides." + escapeSjsonKey("lodash"): "npm:@rootio/lodash@4.17.21",
		},
	})
	if err != nil {
		t.Fatalf("Patch failed: %v", err)
	}

	got, _ := os.ReadFile(pkgPath)
	content := string(got)

	if !strings.Contains(content, `"overrides"`) {
		t.Error("expected overrides field")
	}
	if !strings.Contains(content, "npm:@rootio/lodash@4.17.21") {
		t.Error("expected lodash override value")
	}

	// Direct dependencies must be untouched.
	if !strings.Contains(content, `"lodash": "^4.17.20"`) {
		t.Error("expected lodash direct dependency to be unchanged")
	}
	if !strings.Contains(content, `"express": "^4.18.0"`) {
		t.Error("expected express direct dependency to be unchanged")
	}
}

func TestPackageJSONPatcher_PreserveIndentation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "2 spaces",
			input: `{
  "name": "test",
  "version": "1.0.0"
}`,
			expected: "  ",
		},
		{
			name: "4 spaces",
			input: `{
    "name": "test",
    "version": "1.0.0"
}`,
			expected: "    ",
		},
		{
			name: "tab",
			input: `{
	"name": "test",
	"version": "1.0.0"
}`,
			expected: "\t",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			tmpDir := t.TempDir()
			pkgPath := tmpDir + "/package.json"
			if err := os.WriteFile(pkgPath, []byte(tt.input), 0644); err != nil {
				t.Fatalf("Failed to create package.json: %v", err)
			}

			err := NewPackageJSONPatcher().Patch(ctx, PatchOptions{
				PackageJSONPath: pkgPath,
				Sets: map[string]string{
					"overrides." + escapeSjsonKey("lodash"): "npm:@rootio/lodash@4.17.21",
				},
			})
			if err != nil {
				t.Fatalf("Patch failed: %v", err)
			}

			got, _ := os.ReadFile(pkgPath)
			if detected := detectIndentation(got); detected != tt.expected {
				t.Errorf("expected indentation %q, got %q", tt.expected, detected)
			}
		})
	}
}

func TestPackageJSONPatcher_AppendsToExistingOverrides(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	pkgPath := tmpDir + "/package.json"

	pkg := `{
  "name": "test-app",
  "version": "1.0.0",
  "dependencies": {
    "lodash": "^4.17.20"
  },
  "overrides": {
    "axios": "^1.2.0"
  }
}`
	if err := os.WriteFile(pkgPath, []byte(pkg), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	err := NewPackageJSONPatcher().Patch(ctx, PatchOptions{
		PackageJSONPath: pkgPath,
		Sets: map[string]string{
			"overrides." + escapeSjsonKey("lodash"): "npm:@rootio/lodash@4.17.21",
		},
	})
	if err != nil {
		t.Fatalf("Patch failed: %v", err)
	}

	got, _ := os.ReadFile(pkgPath)
	content := string(got)

	if !strings.Contains(content, `"axios": "^1.2.0"`) {
		t.Error("existing axios override should be preserved")
	}
	if !strings.Contains(content, "npm:@rootio/lodash@4.17.21") {
		t.Error("expected lodash override")
	}
}

// TestNpmParser_IsDirectVulnerable verifies the lockfile-based detection
// of "is the user's direct dep at this vulnerable version".
func TestNpmParser_IsDirectVulnerable(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	pkgPath := tmpDir + "/package.json"
	lockPath := tmpDir + "/package-lock.json"

	if err := os.WriteFile(pkgPath, []byte(`{
  "dependencies": { "uuid": "^11.0.3", "dockerode": "^4.0.12" }
}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, []byte(`{
  "lockfileVersion": 3,
  "packages": {
    "": { "dependencies": { "uuid": "^11.0.3", "dockerode": "^4.0.12" } },
    "node_modules/uuid": { "version": "11.0.3" },
    "node_modules/dockerode": { "version": "4.0.12", "dependencies": { "uuid": "^10.0.0" } },
    "node_modules/dockerode/node_modules/uuid": { "version": "10.0.0" }
  }
}`), 0644); err != nil {
		t.Fatal(err)
	}

	parser := NewNpmParser()

	got, err := parser.IsDirectVulnerable(ctx, lockPath, pkgPath, "uuid", "11.0.3")
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Error("expected uuid@11.0.3 to be detected as direct + vulnerable")
	}

	got, err = parser.IsDirectVulnerable(ctx, lockPath, pkgPath, "uuid", "10.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if got {
		t.Error("uuid@10.0.0 is the transitive copy, not direct — should be false")
	}

	got, err = parser.IsDirectVulnerable(ctx, lockPath, pkgPath, "lodash", "5.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if got {
		t.Error("lodash isn't even a dependency — should be false")
	}
}

// TestNpmParser_UpdatePackageJSON_DirectAndTransitive is the regression test
// for the user's exact reported example: uuid@^11.0.3 is the user's direct
// dep (vulnerable at v11.0.3), AND dockerode pulls a transitive uuid@10.0.0
// (also vulnerable). The patcher must rewrite the direct dep to the v11
// alias AND emit a parent-nested override under dockerode for the v10 alias.
func TestNpmParser_UpdatePackageJSON_DirectAndTransitive(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	pkgPath := tmpDir + "/package.json"

	if err := os.WriteFile(pkgPath, []byte(`{
  "name": "test-app",
  "version": "1.0.0",
  "dependencies": {
    "dockerode": "^4.0.12",
    "uuid": "^11.0.3"
  }
}`), 0644); err != nil {
		t.Fatal(err)
	}

	overrides := []ScopedOverride{
		{
			PackageName:   "uuid",
			Version:       "11.0.3",
			Value:         "npm:@rootio/uuid@11.0.3-root.io.1",
			PatchInfo:     rootio.PatchInfo{Name: "@rootio/uuid", Version: "11.0.3-root.io.1"},
			RewriteDirect: true,
		},
		{
			PackageName: "uuid",
			Version:     "10.0.0",
			Value:       "npm:@rootio/uuid@10.0.0-root.io.1",
			Parents:     []string{"dockerode"},
		},
	}
	if err := NewNpmParser().UpdatePackageJSON(ctx, overrides, pkgPath); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(pkgPath)
	content := string(got)

	// Old uuid should be removed, new @rootio/uuid added
	if strings.Contains(content, `"uuid": "npm:@rootio/uuid@11.0.3-root.io.1"`) {
		t.Errorf("old uuid package should be removed; got:\n%s", content)
	}
	if !strings.Contains(content, `"@rootio/uuid": "11.0.3-root.io.1"`) {
		t.Errorf("expected new @rootio/uuid package; got:\n%s", content)
	}
	// npm now uses version-scoped flat overrides for transitive uses
	if !strings.Contains(content, `"uuid@10.0.0": "npm:@rootio/uuid@10.0.0-root.io.1"`) {
		t.Errorf("expected version-scoped override for uuid@10.0.0; got:\n%s", content)
	}
	if !strings.Contains(content, `"uuid@11.0.3": "npm:@rootio/uuid@11.0.3-root.io.1"`) {
		t.Errorf("expected version-scoped override for uuid@11.0.3; got:\n%s", content)
	}
	if !strings.Contains(content, `"dockerode": "^4.0.12"`) {
		t.Errorf("dockerode direct dep must remain unchanged; got:\n%s", content)
	}
}

// TestNpmParser_FindParents_HoistedTransitive verifies the EOVERRIDE-fix
// case: the user's direct uuid range overlaps dockerode's transitive range,
// so npm hoists a single uuid copy at the top level (no nested path).
// FindParents must still report dockerode as a consumer so the caller can
// emit a parent-nested override (the only shape that doesn't trip npm's
// "Override for X conflicts with direct dependency" check).
func TestNpmParser_FindParents_HoistedTransitive(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	lockPath := tmpDir + "/package-lock.json"

	lock := `{
  "name": "test-app",
  "lockfileVersion": 3,
  "packages": {
    "": {
      "dependencies": { "dockerode": "^4.0.12", "uuid": "^10.0.0" }
    },
    "node_modules/dockerode": {
      "version": "4.0.12",
      "dependencies": { "uuid": "^10.0.0" }
    },
    "node_modules/uuid": {
      "version": "10.0.0"
    }
  }
}`
	if err := os.WriteFile(lockPath, []byte(lock), 0644); err != nil {
		t.Fatalf("Failed to write lock: %v", err)
	}

	got, err := NewNpmParser().FindParents(ctx, lockPath, "uuid", "10.0.0")
	if err != nil {
		t.Fatalf("FindParents failed: %v", err)
	}
	if len(got) != 1 || got[0] != "dockerode" {
		t.Errorf("expected [dockerode] (hoisted-transitive consumer), got %v", got)
	}
}

// TestNpmParser_FindParents_NestedTransitive verifies parent detection on
// the user's reported bug shape: hoisted uuid@14 (direct) plus a nested
// uuid@10 inside dockerode. FindParents must return ["dockerode"] for
// uuid@10.0.0 and an empty list for uuid@14.0.0 (top-level).
func TestNpmParser_FindParents_NestedTransitive(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	lockPath := tmpDir + "/package-lock.json"

	lock := `{
  "name": "test-app",
  "lockfileVersion": 3,
  "packages": {
    "": {
      "dependencies": { "dockerode": "^4.0.12", "uuid": "^14.0.0" }
    },
    "node_modules/dockerode": {
      "version": "4.0.12",
      "dependencies": { "uuid": "^10.0.0" }
    },
    "node_modules/dockerode/node_modules/uuid": {
      "version": "10.0.0"
    },
    "node_modules/uuid": {
      "version": "14.0.0"
    }
  }
}`
	if err := os.WriteFile(lockPath, []byte(lock), 0644); err != nil {
		t.Fatalf("Failed to write lock: %v", err)
	}

	parser := NewNpmParser()

	got, err := parser.FindParents(ctx, lockPath, "uuid", "10.0.0")
	if err != nil {
		t.Fatalf("FindParents failed: %v", err)
	}
	if len(got) != 1 || got[0] != "dockerode" {
		t.Errorf("expected [dockerode] for uuid@10.0.0, got %v", got)
	}

	gotTop, err := parser.FindParents(ctx, lockPath, "uuid", "14.0.0")
	if err != nil {
		t.Fatalf("FindParents (top-level) failed: %v", err)
	}
	if len(gotTop) != 0 {
		t.Errorf("expected no parents for top-level uuid@14.0.0, got %v", gotTop)
	}
}

// TestNpmParser_UpdatePackageJSON_ParentScoped is the regression test for the
// reported bug: a vulnerable transitive dependency must not degrade the user's
// direct dependency at a different version. The override must be parent-scoped
// under the consumer (e.g. dockerode), and the direct uuid version stays
// untouched.
func TestNpmParser_UpdatePackageJSON_ParentScoped(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	pkgPath := tmpDir + "/package.json"

	pkg := `{
  "name": "test-app",
  "version": "1.0.0",
  "dependencies": {
    "dockerode": "^4.0.12",
    "uuid": "^14.0.0"
  }
}`
	if err := os.WriteFile(pkgPath, []byte(pkg), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	overrides := []ScopedOverride{
		{
			PackageName: "uuid",
			Version:     "10.0.0",
			Value:       "npm:@rootio/uuid@10.0.0-root.io.1",
			Parents:     []string{"dockerode"},
		},
	}

	if err := NewNpmParser().UpdatePackageJSON(ctx, overrides, pkgPath); err != nil {
		t.Fatalf("UpdatePackageJSON failed: %v", err)
	}

	got, _ := os.ReadFile(pkgPath)
	content := string(got)

	// Direct uuid dep must remain at ^14.0.0 (not modified by override)
	if !strings.Contains(content, `"uuid": "^14.0.0"`) {
		t.Errorf("user's direct uuid@^14.0.0 must not be modified; got:\n%s", content)
	}
	if !strings.Contains(content, `"dockerode": "^4.0.12"`) {
		t.Errorf("user's direct dockerode dep must not be modified; got:\n%s", content)
	}
	// npm now uses version-scoped flat overrides which safely target specific versions
	// "uuid@10.0.0" only affects uuid@10.0.0, not uuid@14.0.0, so no conflict
	if !strings.Contains(content, `"uuid@10.0.0": "npm:@rootio/uuid@10.0.0-root.io.1"`) {
		t.Errorf("expected version-scoped override for uuid@10.0.0; got:\n%s", content)
	}
}

// TestNpmParser_UpdatePackageJSON_UpdatesShadowingNestedOverride is the
// regression test for the infinite-loop bug: a pre-existing nested/path-scoped
// override (e.g. "@apollo/query-planner": {"@apollo/federation-internals": "...root.io.1"})
// takes npm resolution precedence over any flat version-scoped key for the
// same package. If remediate only ever adds a new flat key
// ("@apollo/federation-internals@2.13.0-root.io.1": "...root.io.3") without
// touching the stale nested entry, the nested entry keeps winning forever and
// --dry-run reports the same finding on every run. UpdatePackageJSON must
// update the pre-existing nested entry's value in place.
func TestNpmParser_UpdatePackageJSON_UpdatesShadowingNestedOverride(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	pkgPath := tmpDir + "/package.json"

	pkg := `{
  "name": "test-app",
  "version": "1.0.0",
  "dependencies": {
    "@apollo/gateway": "npm:@rootio/apollo__gateway@2.13.0-root.io.1"
  },
  "overrides": {
    "@apollo/query-planner": {
      "@apollo/federation-internals": "npm:@rootio/apollo__federation-internals@2.13.0-root.io.1"
    },
    "cacache": { "tar": "npm:@rootio/tar@6.2.1-root.io.7" }
  }
}`
	if err := os.WriteFile(pkgPath, []byte(pkg), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	overrides := []ScopedOverride{
		{
			PackageName: "@apollo/federation-internals",
			Version:     "2.13.0-root.io.1",
			Value:       "npm:@rootio/apollo__federation-internals@2.13.0-root.io.3",
		},
		{
			PackageName: "tar",
			Version:     "6.2.1-root.io.7",
			Value:       "npm:@rootio/tar@6.2.1-root.io.9",
		},
	}

	if err := NewNpmParser().UpdatePackageJSON(ctx, overrides, pkgPath); err != nil {
		t.Fatalf("UpdatePackageJSON failed: %v", err)
	}

	got, _ := os.ReadFile(pkgPath)
	content := string(got)

	if strings.Contains(content, `"@apollo/federation-internals": "npm:@rootio/apollo__federation-internals@2.13.0-root.io.1"`) {
		t.Errorf("stale nested override for @apollo/federation-internals must be updated, not left shadowing the new flat key; got:\n%s", content)
	}
	if !strings.Contains(content, `"@apollo/federation-internals": "npm:@rootio/apollo__federation-internals@2.13.0-root.io.3"`) {
		t.Errorf("expected nested override under @apollo/query-planner to be bumped to root.io.3; got:\n%s", content)
	}
	if strings.Contains(content, `"tar": "npm:@rootio/tar@6.2.1-root.io.7"`) {
		t.Errorf("stale nested override for tar under cacache must be updated, not left shadowing the new flat key; got:\n%s", content)
	}
	if !strings.Contains(content, `"tar": "npm:@rootio/tar@6.2.1-root.io.9"`) {
		t.Errorf("expected nested override under cacache to be bumped to root.io.9; got:\n%s", content)
	}
}

// TestNpmParser_UpdatePackageJSON_LeavesUnrelatedVersionScopedNestedOverride
// guards against a regression where findNestedOverrideParents matched a
// nested override by package name alone, ignoring the version embedded in a
// version-scoped nested key (e.g. "uuid@9.0.1"). That caused patching one
// version of a package to overwrite an unrelated nested override pinned to a
// different version of the same package.
func TestNpmParser_UpdatePackageJSON_LeavesUnrelatedVersionScopedNestedOverride(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	pkgPath := tmpDir + "/package.json"

	pkg := `{
  "name": "test-app",
  "version": "1.0.0",
  "overrides": {
    "parentA": {
      "uuid@9.0.1": "npm:@rootio/uuid@9.0.1-root.io.1"
    }
  }
}`
	if err := os.WriteFile(pkgPath, []byte(pkg), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	overrides := []ScopedOverride{
		{
			PackageName: "uuid",
			Version:     "11.1.1",
			Value:       "npm:@rootio/uuid@11.1.0-root.io.1",
		},
	}

	if err := NewNpmParser().UpdatePackageJSON(ctx, overrides, pkgPath); err != nil {
		t.Fatalf("UpdatePackageJSON failed: %v", err)
	}

	got, _ := os.ReadFile(pkgPath)
	content := string(got)

	if !strings.Contains(content, `"uuid@9.0.1": "npm:@rootio/uuid@9.0.1-root.io.1"`) {
		t.Errorf("unrelated uuid@9.0.1 nested override must not be touched by a uuid@11.1.1 patch; got:\n%s", content)
	}
	if !strings.Contains(content, `"uuid@11.1.1": "npm:@rootio/uuid@11.1.0-root.io.1"`) {
		t.Errorf("expected flat override for uuid@11.1.1; got:\n%s", content)
	}
}
