package npm

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestPnpmParser_UpdatePackageJSON_VersionScoped verifies pnpm uses the
// version-scoped flat-key override form ("name@version") and never modifies
// direct dependencies.
func TestPnpmParser_UpdatePackageJSON_VersionScoped(t *testing.T) {
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
			Value:       "npm:@rootio/uuid@10.0.0-aikido.1",
		},
	}
	if err := NewPnpmParser().UpdatePackageJSON(ctx, overrides, pkgPath); err != nil {
		t.Fatalf("UpdatePackageJSON failed: %v", err)
	}

	got, _ := os.ReadFile(pkgPath)
	content := string(got)

	if !strings.Contains(content, `"uuid": "^14.0.0"`) {
		t.Errorf("user's direct uuid@^14.0.0 must be unchanged; got:\n%s", content)
	}
	if !strings.Contains(content, `"uuid@10.0.0": "npm:@rootio/uuid@10.0.0-aikido.1"`) {
		t.Errorf("expected version-scoped pnpm override key 'uuid@10.0.0'; got:\n%s", content)
	}
	if !strings.Contains(content, `"pnpm"`) {
		t.Error("expected pnpm field")
	}
}

// TestPnpmParser_UpdatePackageJSON_AppendsToExisting verifies new overrides
// are merged into an existing pnpm.overrides block.
func TestPnpmParser_UpdatePackageJSON_AppendsToExisting(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	pkgPath := tmpDir + "/package.json"

	pkg := `{
  "name": "test-app",
  "version": "1.0.0",
  "dependencies": {
    "lodash": "^4.17.20"
  },
  "pnpm": {
    "overrides": {
      "axios@1.2.0": "npm:@rootio/axios@1.2.1"
    }
  }
}`
	if err := os.WriteFile(pkgPath, []byte(pkg), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	overrides := []ScopedOverride{
		{
			PackageName: "lodash",
			Version:     "4.17.20",
			Value:       "npm:@rootio/lodash@4.17.21",
		},
	}
	if err := NewPnpmParser().UpdatePackageJSON(ctx, overrides, pkgPath); err != nil {
		t.Fatalf("UpdatePackageJSON failed: %v", err)
	}

	got, _ := os.ReadFile(pkgPath)
	content := string(got)

	if !strings.Contains(content, `"axios@1.2.0": "npm:@rootio/axios@1.2.1"`) {
		t.Error("existing pnpm override should be preserved")
	}
	if !strings.Contains(content, `"lodash@4.17.20": "npm:@rootio/lodash@4.17.21"`) {
		t.Error("expected new lodash@4.17.20 version-scoped override")
	}
}

// TestPnpmParser_UpdatePackageJSON_ScopedPackage verifies that escaping
// produces a literal "@scope/name@version" key in the JSON (not a backslash-
// escaped variant).
func TestPnpmParser_UpdatePackageJSON_ScopedPackage(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	pkgPath := tmpDir + "/package.json"

	pkg := `{
  "name": "test-app",
  "version": "1.0.0",
  "dependencies": {
    "@babel/runtime": "^7.25.0"
  }
}`
	if err := os.WriteFile(pkgPath, []byte(pkg), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	overrides := []ScopedOverride{
		{
			PackageName: "@babel/runtime",
			Version:     "7.25.0",
			Value:       "npm:@rootio/babel__runtime@7.25.0",
		},
	}
	if err := NewPnpmParser().UpdatePackageJSON(ctx, overrides, pkgPath); err != nil {
		t.Fatalf("UpdatePackageJSON failed: %v", err)
	}

	got, _ := os.ReadFile(pkgPath)
	content := string(got)

	if !strings.Contains(content, `"@babel/runtime@7.25.0": "npm:@rootio/babel__runtime@7.25.0"`) {
		t.Errorf("expected literal key '@babel/runtime@7.25.0' in pnpm.overrides; got:\n%s", content)
	}
	if strings.Contains(content, `\@`) || strings.Contains(content, `\.`) {
		t.Errorf("backslash-escape sequence leaked into JSON output; got:\n%s", content)
	}
}

// TestNpmParser_UpdatePackageJSON_ScopedParent verifies that an npm parent-
// nested override with a scoped parent name (e.g. "@babel/core") lands as a
// proper nested object under "@babel/core", not as a deep-nested path.
func TestNpmParser_UpdatePackageJSON_ScopedParent(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	pkgPath := tmpDir + "/package.json"

	pkg := `{
  "name": "test-app",
  "version": "1.0.0",
  "dependencies": {
    "@babel/core": "^7.0.0"
  }
}`
	if err := os.WriteFile(pkgPath, []byte(pkg), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	overrides := []ScopedOverride{
		{
			PackageName: "json5",
			Version:     "2.0.0",
			Value:       "npm:@rootio/json5@2.0.1",
			Parents:     []string{"@babel/core"},
		},
	}
	if err := NewNpmParser().UpdatePackageJSON(ctx, overrides, pkgPath); err != nil {
		t.Fatalf("UpdatePackageJSON failed: %v", err)
	}

	got, _ := os.ReadFile(pkgPath)
	content := string(got)

	// npm now uses version-scoped flat overrides (like pnpm) instead of parent-nested
	if !strings.Contains(content, `"json5@2.0.0": "npm:@rootio/json5@2.0.1"`) {
		t.Errorf("expected version-scoped flat override 'json5@2.0.0'; got:\n%s", content)
	}
	if strings.Contains(content, `\@`) || strings.Contains(content, `\.`) {
		t.Errorf("backslash-escape sequence leaked into JSON output; got:\n%s", content)
	}
}

// TestYarnParser_UpdatePackageJSON_ParentScoped verifies yarn uses the
// parent/child slash-path resolution form and never modifies direct
// dependencies.
func TestYarnParser_UpdatePackageJSON_ParentScoped(t *testing.T) {
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
			Value:       "npm:@rootio/uuid@10.0.0-aikido.1",
			Parents:     []string{"dockerode"},
		},
	}
	if err := NewYarnParser().UpdatePackageJSON(ctx, overrides, pkgPath); err != nil {
		t.Fatalf("UpdatePackageJSON failed: %v", err)
	}

	got, _ := os.ReadFile(pkgPath)
	content := string(got)

	if !strings.Contains(content, `"uuid": "^14.0.0"`) {
		t.Errorf("user's direct uuid@^14.0.0 must be unchanged; got:\n%s", content)
	}
	if !strings.Contains(content, `"dockerode/uuid": "npm:@rootio/uuid@10.0.0-aikido.1"`) {
		t.Errorf("expected parent/child slash-path resolution 'dockerode/uuid'; got:\n%s", content)
	}
	if !strings.Contains(content, `"resolutions"`) {
		t.Error("expected resolutions field")
	}
}

// TestPnpmParser_UpdatePackageJSON_SecondRoundBumpsInPlace is the pnpm twin of
// the npm Pattern A guard test. Round 1 left "lodash@4.17.20" pointing at a
// patched build; round 2 re-flags the package at that OUTPUT version, so the
// existing key must be bumped in place. Adding a
// "lodash@4.17.21-aikido.1" key instead would be dead — no dependent declares
// that string as a range — so the original key would keep pinning the old
// build and the CVE would go unfixed while the run reported success.
func TestPnpmParser_UpdatePackageJSON_SecondRoundBumpsInPlace(t *testing.T) {
	for _, tt := range []struct {
		name       string
		pkg        string
		priorKey   string
		priorValue string
		prior      string
		next       string
	}{
		{"plain patched version", "lodash", "lodash@4.17.20", "4.17.21-aikido.1", "4.17.21-aikido.1", "4.17.21-aikido.2"},
		{"legacy npm: alias descriptor", "lodash", "lodash@4.17.20", "npm:@rootio/lodash@4.17.21-aikido.1", "4.17.21-aikido.1", "4.17.21-aikido.2"},
		{"scoped package", "@babel/runtime", "@babel/runtime@7.25.0", "7.25.1-aikido.1", "7.25.1-aikido.1", "7.25.1-aikido.2"},
		{"bare (unversioned) key", "lodash", "lodash", "4.17.21-aikido.1", "4.17.21-aikido.1", "4.17.21-aikido.2"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pkgPath := t.TempDir() + "/package.json"
			if err := os.WriteFile(pkgPath, []byte(`{
  "name": "test-app",
  "dependencies": {"`+tt.pkg+`": "*"},
  "pnpm": {"overrides": {"`+tt.priorKey+`": "`+tt.priorValue+`"}}
}`), 0644); err != nil {
				t.Fatal(err)
			}

			overrides := []ScopedOverride{{
				PackageName: tt.pkg,
				Version:     tt.prior,
				Value:       tt.next,
			}}
			if err := NewPnpmParser().UpdatePackageJSON(ctx, overrides, pkgPath); err != nil {
				t.Fatal(err)
			}

			got, _ := os.ReadFile(pkgPath)
			content := string(got)
			if !strings.Contains(content, `"`+tt.priorKey+`": "`+tt.next+`"`) {
				t.Errorf("expected controlling key %q bumped in place to %s; got:\n%s", tt.priorKey, tt.next, content)
			}
			if strings.Contains(content, `"`+tt.pkg+`@`+tt.prior+`"`) {
				t.Errorf("must not add a dead override key scoped to the prior output version; got:\n%s", content)
			}
		})
	}
}
