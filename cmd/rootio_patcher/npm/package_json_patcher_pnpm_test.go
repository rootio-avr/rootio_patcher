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
			Value:       "npm:@rootio/uuid@10.0.0-root.io.1",
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
	if !strings.Contains(content, `"uuid@10.0.0": "npm:@rootio/uuid@10.0.0-root.io.1"`) {
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

	if !strings.Contains(content, `"@babel/core": {`) {
		t.Errorf("expected nested object under '@babel/core'; got:\n%s", content)
	}
	if !strings.Contains(content, `"json5": "npm:@rootio/json5@2.0.1"`) {
		t.Errorf("expected json5 override under @babel/core; got:\n%s", content)
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
			Value:       "npm:@rootio/uuid@10.0.0-root.io.1",
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
	if !strings.Contains(content, `"dockerode/uuid": "npm:@rootio/uuid@10.0.0-root.io.1"`) {
		t.Errorf("expected parent/child slash-path resolution 'dockerode/uuid'; got:\n%s", content)
	}
	if !strings.Contains(content, `"resolutions"`) {
		t.Error("expected resolutions field")
	}
}
