package npm

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestPackageJSONPatcher_PatchPackageJSON_Npm(t *testing.T) {
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
    "lodash": "^4.17.20",
    "express": "^4.18.0"
  },
  "devDependencies": {
    "jest": "^29.0.0"
  }
}`
	if err := os.WriteFile("package.json", []byte(packageJSON), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	// Create patcher
	patcher := NewPackageJSONPatcher()

	// Apply patches
	overrides := map[string]string{
		"lodash":  "npm:@rootio/lodash@4.17.21",
		"express": "npm:@rootio/express@4.18.2",
	}

	if err := patcher.Patch(ctx, PatchOptions{
		Updates:                  overrides,
		UpdateDirectDependencies: true,
		OverridesPath:            "overrides",
	}); err != nil {
		t.Fatalf("Patch failed: %v", err)
	}

	// Read updated file
	content, err := os.ReadFile("package.json")
	if err != nil {
		t.Fatalf("Failed to read updated package.json: %v", err)
	}

	contentStr := string(content)

	// Verify overrides field exists
	if !strings.Contains(contentStr, `"overrides"`) {
		t.Error("Expected overrides field in package.json")
	}

	// Verify lodash override
	if !strings.Contains(contentStr, "npm:@rootio/lodash@4.17.21") {
		t.Error("Expected lodash override in package.json")
	}

	// Verify express override
	if !strings.Contains(contentStr, "npm:@rootio/express@4.18.2") {
		t.Error("Expected express override in package.json")
	}

	// Verify direct dependencies were also updated
	if !strings.Contains(contentStr, `"dependencies"`) {
		t.Error("Expected dependencies field to still exist")
	}

	t.Logf("Updated package.json:\n%s", contentStr)
}

func TestPackageJSONPatcher_PatchPackageJSON_Yarn(t *testing.T) {
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

	// Apply patches
	overrides := map[string]string{
		"lodash": "npm:@rootio/lodash@4.17.21",
	}

	if err := patcher.Patch(ctx, PatchOptions{
		Updates:                  overrides,
		UpdateDirectDependencies: true,
		OverridesPath:            "resolutions",
	}); err != nil {
		t.Fatalf("Patch failed: %v", err)
	}

	// Read updated file
	content, err := os.ReadFile("package.json")
	if err != nil {
		t.Fatalf("Failed to read updated package.json: %v", err)
	}

	contentStr := string(content)

	// Verify resolutions field exists
	if !strings.Contains(contentStr, `"resolutions"`) {
		t.Error("Expected resolutions field in package.json")
	}

	// Verify lodash resolution
	if !strings.Contains(contentStr, "npm:@rootio/lodash@4.17.21") {
		t.Error("Expected lodash resolution in package.json")
	}

	t.Logf("Updated package.json:\n%s", contentStr)
}

func TestPackageJSONPatcher_PatchPackageJSON_Pnpm(t *testing.T) {
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
    "jest": "^29.0.0"
  }
}`
	if err := os.WriteFile("package.json", []byte(packageJSON), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	// Create patcher
	patcher := NewPackageJSONPatcher()

	// Apply patches
	overrides := map[string]string{
		"jest": "npm:@rootio/jest@29.5.0",
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

	// Verify pnpm.overrides field exists
	if !strings.Contains(contentStr, `"pnpm"`) {
		t.Error("Expected pnpm field in package.json")
	}
	if !strings.Contains(contentStr, `"overrides"`) {
		t.Error("Expected overrides field nested under pnpm in package.json")
	}

	// Verify jest override
	if !strings.Contains(contentStr, "npm:@rootio/jest@29.5.0") {
		t.Error("Expected jest override in package.json")
	}

	t.Logf("Updated package.json:\n%s", contentStr)
}

func TestPackageJSONPatcher_PreserveIndentation(t *testing.T) {
	tests := []struct {
		name           string
		inputJSON      string
		expectedIndent string
	}{
		{
			name: "2 spaces",
			inputJSON: `{
  "name": "test",
  "version": "1.0.0"
}`,
			expectedIndent: "  ",
		},
		{
			name: "4 spaces",
			inputJSON: `{
    "name": "test",
    "version": "1.0.0"
}`,
			expectedIndent: "    ",
		},
		{
			name: "tab",
			inputJSON: `{
	"name": "test",
	"version": "1.0.0"
}`,
			expectedIndent: "\t",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create temp directory
			tmpDir := t.TempDir()
			oldWd, _ := os.Getwd()
			os.Chdir(tmpDir)
			defer os.Chdir(oldWd)

			// Create package.json with specific indentation
			if err := os.WriteFile("package.json", []byte(tt.inputJSON), 0644); err != nil {
				t.Fatalf("Failed to create package.json: %v", err)
			}

			// Create patcher
			patcher := NewPackageJSONPatcher()

			// Apply patches
			overrides := map[string]string{
				"lodash": "npm:@rootio/lodash@4.17.21",
			}

			if err := patcher.Patch(ctx, PatchOptions{
				Updates:                  overrides,
				UpdateDirectDependencies: true,
				OverridesPath:            "overrides",
			}); err != nil {
				t.Fatalf("Patch failed: %v", err)
			}

			// Read updated file
			content, err := os.ReadFile("package.json")
			if err != nil {
				t.Fatalf("Failed to read updated package.json: %v", err)
			}

			// Detect indentation in updated file
			detectedIndent := detectIndentation(content)
			if detectedIndent != tt.expectedIndent {
				t.Errorf("Expected indentation %q, got %q", tt.expectedIndent, detectedIndent)
			}
		})
	}
}

func TestPackageJSONPatcher_ScopedPackages(t *testing.T) {
	ctx := context.Background()

	// Create temp directory
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Create package.json with scoped packages
	packageJSON := `{
  "name": "test-app",
  "version": "1.0.0",
  "dependencies": {
    "@babel/core": "^7.20.0",
    "@types/node": "^18.0.0"
  }
}`
	if err := os.WriteFile("package.json", []byte(packageJSON), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	// Create patcher
	patcher := NewPackageJSONPatcher()

	// Apply patches with scoped packages
	overrides := map[string]string{
		"@babel/core": "npm:@rootio/babel__core@7.21.0",
		"@types/node": "npm:@rootio/types__node@18.11.0",
	}

	if err := patcher.Patch(ctx, PatchOptions{
		Updates:                  overrides,
		UpdateDirectDependencies: true,
		OverridesPath:            "overrides",
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
	if !strings.Contains(contentStr, "@babel/core") {
		t.Error("Expected @babel/core in package.json")
	}
	if !strings.Contains(contentStr, "npm:@rootio/babel__core@7.21.0") {
		t.Error("Expected @babel/core override in package.json")
	}
	if !strings.Contains(contentStr, "@types/node") {
		t.Error("Expected @types/node in package.json")
	}

	t.Logf("Updated package.json:\n%s", contentStr)
}

func TestPackageJSONPatcher_AppendToExistingOverrides(t *testing.T) {
	ctx := context.Background()

	// Create temp directory
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Create package.json with existing overrides
	packageJSON := `{
  "name": "test-app",
  "version": "1.0.0",
  "dependencies": {
    "lodash": "^4.17.20",
    "express": "^4.18.0"
  },
  "overrides": {
    "axios": "^1.2.0"
  }
}`
	if err := os.WriteFile("package.json", []byte(packageJSON), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	// Create patcher
	patcher := NewPackageJSONPatcher()

	// Apply patches
	overrides := map[string]string{
		"lodash":  "npm:@rootio/lodash@4.17.21",
		"express": "npm:@rootio/express@4.18.2",
	}

	if err := patcher.Patch(ctx, PatchOptions{
		Updates:                  overrides,
		UpdateDirectDependencies: true,
		OverridesPath:            "overrides",
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
		t.Error("Expected lodash override in package.json")
	}
	if !strings.Contains(contentStr, "npm:@rootio/express@4.18.2") {
		t.Error("Expected express override in package.json")
	}

	t.Logf("Updated package.json:\n%s", contentStr)
}
