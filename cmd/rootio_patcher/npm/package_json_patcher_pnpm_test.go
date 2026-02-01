package npm

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestPackageJSONPatcher_Pnpm_CreateNestedOverrides(t *testing.T) {
	ctx := context.Background()

	// Create temp directory
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Create package.json WITHOUT pnpm section
	packageJSON := `{
  "name": "test-app",
  "version": "1.0.0",
  "dependencies": {
    "lodash": "^4.17.20"
  }
}`
	if err := os.WriteFile("package.json", []byte(packageJSON), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	// Create patcher
	patcher := NewPackageJSONPatcher()

	// Apply patches - should create pnpm.overrides structure
	overrides := map[string]string{
		"lodash": "npm:@rootio/lodash@4.17.21",
		"express": "npm:@rootio/express@4.18.2",
	}

	if err := patcher.Patch(ctx, PatchOptions{
		Updates:                  overrides,
		UpdateDirectDependencies: true,
		OverridesPath:            "pnpm.overrides",
	}); err != nil {
		t.Fatalf("Patch failed: %v", err)
	}

	// Read updated file
	content, err := os.ReadFile("package.json")
	if err != nil {
		t.Fatalf("Failed to read updated package.json: %v", err)
	}

	contentStr := string(content)

	// Verify nested pnpm.overrides structure was created
	if !strings.Contains(contentStr, `"pnpm"`) {
		t.Error("Expected pnpm field in package.json")
	}
	if !strings.Contains(contentStr, `"overrides"`) {
		t.Error("Expected overrides field nested under pnpm")
	}

	// Verify both overrides exist
	if !strings.Contains(contentStr, "npm:@rootio/lodash@4.17.21") {
		t.Error("Expected lodash override")
	}
	if !strings.Contains(contentStr, "npm:@rootio/express@4.18.2") {
		t.Error("Expected express override")
	}

	// Verify direct dependency was updated
	if !strings.Contains(contentStr, `"lodash": "npm:@rootio/lodash@4.17.21"`) {
		t.Error("Expected lodash direct dependency to be updated")
	}

	t.Logf("Updated package.json:\n%s", contentStr)
}

func TestPackageJSONPatcher_Pnpm_AppendToExistingOverrides(t *testing.T) {
	ctx := context.Background()

	// Create temp directory
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Create package.json with existing pnpm.overrides
	packageJSON := `{
  "name": "test-app",
  "version": "1.0.0",
  "dependencies": {
    "lodash": "^4.17.20",
    "express": "^4.18.0"
  },
  "pnpm": {
    "overrides": {
      "axios": "^1.2.0"
    }
  }
}`
	if err := os.WriteFile("package.json", []byte(packageJSON), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	// Create patcher
	patcher := NewPackageJSONPatcher()

	// Apply new patches - should append to existing pnpm.overrides
	overrides := map[string]string{
		"lodash":  "npm:@rootio/lodash@4.17.21",
		"express": "npm:@rootio/express@4.18.2",
	}

	if err := patcher.Patch(ctx, PatchOptions{
		Updates:                  overrides,
		UpdateDirectDependencies: true,
		OverridesPath:            "pnpm.overrides",
	}); err != nil {
		t.Fatalf("Patch failed: %v", err)
	}

	// Read updated file
	content, err := os.ReadFile("package.json")
	if err != nil {
		t.Fatalf("Failed to read updated package.json: %v", err)
	}

	contentStr := string(content)

	// Verify existing override is preserved
	if !strings.Contains(contentStr, "axios") {
		t.Error("Expected existing axios override to be preserved")
	}

	// Verify new overrides were added
	if !strings.Contains(contentStr, "npm:@rootio/lodash@4.17.21") {
		t.Error("Expected lodash override to be added")
	}
	if !strings.Contains(contentStr, "npm:@rootio/express@4.18.2") {
		t.Error("Expected express override to be added")
	}

	t.Logf("Updated package.json:\n%s", contentStr)
}

func TestPackageJSONPatcher_Pnpm_ScopedPackages(t *testing.T) {
	ctx := context.Background()

	// Create temp directory
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Create package.json with scoped dependencies
	packageJSON := `{
  "name": "test-app",
  "version": "1.0.0",
  "dependencies": {
    "@babel/runtime": "^7.25.0",
    "@types/node": "^18.0.0"
  },
  "devDependencies": {
    "@babel/core": "^7.20.0"
  }
}`
	if err := os.WriteFile("package.json", []byte(packageJSON), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	// Create patcher
	patcher := NewPackageJSONPatcher()

	// Apply patches with scoped packages
	overrides := map[string]string{
		"@babel/runtime": "npm:@rootio/babel__runtime@7.25.0-root.io.1",
		"@types/node":    "npm:@rootio/types__node@18.11.0",
		"@babel/core":    "npm:@rootio/babel__core@7.21.0",
	}

	if err := patcher.Patch(ctx, PatchOptions{
		Updates:                  overrides,
		UpdateDirectDependencies: true,
		OverridesPath:            "pnpm.overrides",
	}); err != nil {
		t.Fatalf("Patch failed: %v", err)
	}

	// Read updated file
	content, err := os.ReadFile("package.json")
	if err != nil {
		t.Fatalf("Failed to read updated package.json: %v", err)
	}

	contentStr := string(content)

	// Verify scoped package overrides
	if !strings.Contains(contentStr, "@babel/runtime") {
		t.Error("Expected @babel/runtime in package.json")
	}
	if !strings.Contains(contentStr, "npm:@rootio/babel__runtime@7.25.0-root.io.1") {
		t.Error("Expected @babel/runtime override")
	}
	if !strings.Contains(contentStr, "@types/node") {
		t.Error("Expected @types/node in package.json")
	}
	if !strings.Contains(contentStr, "@babel/core") {
		t.Error("Expected @babel/core in devDependencies")
	}

	// Verify pnpm.overrides exists
	if !strings.Contains(contentStr, `"pnpm"`) {
		t.Error("Expected pnpm field in package.json")
	}
	if !strings.Contains(contentStr, `"overrides"`) {
		t.Error("Expected overrides field nested under pnpm")
	}

	t.Logf("Updated package.json:\n%s", contentStr)
}

func TestPackageJSONPatcher_Pnpm_OnlyOverridesNoDirectUpdate(t *testing.T) {
	ctx := context.Background()

	// Create temp directory
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Create package.json
	packageJSON := `{
  "name": "test-app",
  "version": "1.0.0",
  "dependencies": {
    "lodash": "^4.17.20"
  }
}`
	if err := os.WriteFile("package.json", []byte(packageJSON), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	// Create patcher
	patcher := NewPackageJSONPatcher()

	// Apply patches WITHOUT updating direct dependencies
	overrides := map[string]string{
		"lodash": "npm:@rootio/lodash@4.17.21",
	}

	if err := patcher.Patch(ctx, PatchOptions{
		Updates:                  overrides,
		UpdateDirectDependencies: false, // Don't touch direct dependencies
		OverridesPath:            "pnpm.overrides",
	}); err != nil {
		t.Fatalf("Patch failed: %v", err)
	}

	// Read updated file
	content, err := os.ReadFile("package.json")
	if err != nil {
		t.Fatalf("Failed to read updated package.json: %v", err)
	}

	contentStr := string(content)

	// Verify direct dependency was NOT changed
	if !strings.Contains(contentStr, `"lodash": "^4.17.20"`) {
		t.Error("Expected original lodash dependency to remain unchanged")
	}

	// Verify override was added
	if !strings.Contains(contentStr, "npm:@rootio/lodash@4.17.21") {
		t.Error("Expected lodash override in pnpm.overrides")
	}

	t.Logf("Updated package.json:\n%s", contentStr)
}

func TestPackageJSONPatcher_Pnpm_MultiplePackagesWithVersions(t *testing.T) {
	ctx := context.Background()

	// Create temp directory
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Create package.json with multiple packages
	packageJSON := `{
  "name": "test-app",
  "version": "1.0.0",
  "dependencies": {
    "lodash": "^4.17.20",
    "express": "^4.18.0",
    "axios": "^1.0.0"
  },
  "devDependencies": {
    "jest": "^29.0.0",
    "mocha": "^8.0.0"
  }
}`
	if err := os.WriteFile("package.json", []byte(packageJSON), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	// Create patcher
	patcher := NewPackageJSONPatcher()

	// Apply patches to multiple packages
	overrides := map[string]string{
		"lodash":  "npm:@rootio/lodash@4.17.21",
		"express": "npm:@rootio/express@4.18.2",
		"jest":    "npm:@rootio/jest@29.5.0",
		"axios":   "npm:@rootio/axios@1.2.0",
	}

	if err := patcher.Patch(ctx, PatchOptions{
		Updates:                  overrides,
		UpdateDirectDependencies: true,
		OverridesPath:            "pnpm.overrides",
	}); err != nil {
		t.Fatalf("Patch failed: %v", err)
	}

	// Read updated file
	content, err := os.ReadFile("package.json")
	if err != nil {
		t.Fatalf("Failed to read updated package.json: %v", err)
	}

	contentStr := string(content)

	// Verify all overrides were added
	expectedOverrides := []string{
		"npm:@rootio/lodash@4.17.21",
		"npm:@rootio/express@4.18.2",
		"npm:@rootio/jest@29.5.0",
		"npm:@rootio/axios@1.2.0",
	}

	for _, override := range expectedOverrides {
		if !strings.Contains(contentStr, override) {
			t.Errorf("Expected to find override: %s", override)
		}
	}

	// Verify mocha (not in overrides) remains unchanged
	if !strings.Contains(contentStr, `"mocha": "^8.0.0"`) {
		t.Error("Expected mocha to remain unchanged")
	}

	t.Logf("Updated package.json:\n%s", contentStr)
}

func TestPackageJSONPatcher_Pnpm_EmptyPackageJSON(t *testing.T) {
	ctx := context.Background()

	// Create temp directory
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Create minimal package.json
	packageJSON := `{
  "name": "test-app",
  "version": "1.0.0"
}`
	if err := os.WriteFile("package.json", []byte(packageJSON), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	// Create patcher
	patcher := NewPackageJSONPatcher()

	// Apply patches to package not in dependencies
	overrides := map[string]string{
		"lodash": "npm:@rootio/lodash@4.17.21",
	}

	if err := patcher.Patch(ctx, PatchOptions{
		Updates:                  overrides,
		UpdateDirectDependencies: true,
		OverridesPath:            "pnpm.overrides",
	}); err != nil {
		t.Fatalf("Patch failed: %v", err)
	}

	// Read updated file
	content, err := os.ReadFile("package.json")
	if err != nil {
		t.Fatalf("Failed to read updated package.json: %v", err)
	}

	contentStr := string(content)

	// Verify pnpm.overrides was created
	if !strings.Contains(contentStr, `"pnpm"`) {
		t.Error("Expected pnpm field in package.json")
	}
	if !strings.Contains(contentStr, `"overrides"`) {
		t.Error("Expected overrides field nested under pnpm")
	}
	if !strings.Contains(contentStr, "npm:@rootio/lodash@4.17.21") {
		t.Error("Expected lodash override")
	}

	t.Logf("Updated package.json:\n%s", contentStr)
}

func TestPackageJSONPatcher_Pnpm_PreserveExistingPnpmConfig(t *testing.T) {
	ctx := context.Background()

	// Create temp directory
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Create package.json with existing pnpm config
	packageJSON := `{
  "name": "test-app",
  "version": "1.0.0",
  "dependencies": {
    "lodash": "^4.17.20"
  },
  "pnpm": {
    "peerDependencyRules": {
      "ignoreMissing": ["react"]
    },
    "overrides": {
      "foo": "1.0.0"
    }
  }
}`
	if err := os.WriteFile("package.json", []byte(packageJSON), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	// Create patcher
	patcher := NewPackageJSONPatcher()

	// Apply new overrides
	overrides := map[string]string{
		"lodash": "npm:@rootio/lodash@4.17.21",
	}

	if err := patcher.Patch(ctx, PatchOptions{
		Updates:                  overrides,
		UpdateDirectDependencies: true,
		OverridesPath:            "pnpm.overrides",
	}); err != nil {
		t.Fatalf("Patch failed: %v", err)
	}

	// Read updated file
	content, err := os.ReadFile("package.json")
	if err != nil {
		t.Fatalf("Failed to read updated package.json: %v", err)
	}

	contentStr := string(content)

	// Verify existing pnpm config is preserved
	if !strings.Contains(contentStr, `"peerDependencyRules"`) {
		t.Error("Expected existing peerDependencyRules to be preserved")
	}
	if !strings.Contains(contentStr, `"ignoreMissing"`) {
		t.Error("Expected ignoreMissing config to be preserved")
	}

	// Verify existing override is preserved
	if !strings.Contains(contentStr, `"foo"`) {
		t.Error("Expected existing foo override to be preserved")
	}

	// Verify new override was added
	if !strings.Contains(contentStr, "npm:@rootio/lodash@4.17.21") {
		t.Error("Expected new lodash override to be added")
	}

	t.Logf("Updated package.json:\n%s", contentStr)
}
