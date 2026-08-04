package npm

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rootio_patcher/pkg/rootio"
)

// TestNpmApp_RealAPIResponse tests with the exact API response format from the user
func TestNpmApp_RealAPIResponse(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create temp directory
	tmpDir := t.TempDir()

	// Create package.json
	packageJSON := filepath.Join(tmpDir, "package.json")
	initialContent := `{
  "name": "test-project",
  "version": "1.0.0",
  "dependencies": {
    "@babel/helpers": "7.26.0"
  }
}`
	if err := os.WriteFile(packageJSON, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	// Change to temp directory
	originalDir, _ := os.Getwd()
	defer os.Chdir(originalDir)
	os.Chdir(tmpDir)

	// Create app with mock that returns the EXACT response from the user's example
	mockAPIClient := &MockAPIClient{
		AnalyzePackagesFunc: func(ctx context.Context, packages []rootio.Package, ignore []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error) {
			return &rootio.AnalyzePackagesResponse{
				Patches: []rootio.PackagePatch{
					{
						PackageName: "@babel/helpers",
						Version:     "7.26.0",
						Patch: rootio.PatchInfo{
							Name:    "@babel/helpers",
							Version: "7.26.0-root.io.2",
						},
						PatchAlias: rootio.PatchInfo{
							Name:    "@rootio/babel__helpers", // Note: double underscore replaces slash
							Version: "7.26.0-root.io.2",
						},
						CVEIDs: []string{"CVE-2025-27789"},
					},
				},
				Skipped: []rootio.SkippedPackage{},
			}, nil
		},
	}

	// Create a fake lock file
	lockFile := filepath.Join(tmpDir, "package-lock.json")
	lockContent := `{
  "name": "test-project",
  "lockfileVersion": 3,
  "packages": {
    "": {
      "dependencies": { "@babel/helpers": "7.26.0" }
    },
    "node_modules/@babel/helpers": {
      "version": "7.26.0"
    }
  }
}`
	if err := os.WriteFile(lockFile, []byte(lockContent), 0644); err != nil {
		t.Fatalf("Failed to create lock file: %v", err)
	}

	app := NewAppWithServices(
		"test-key",
		"https://api.root.io",
		"npm",
		false, // not dry-run
		nil,
		logger,
		NewParser(),
		mockAPIClient,
	)

	// Run the app
	if err := app.Run(ctx); err != nil {
		t.Fatalf("App run failed: %v", err)
	}

	// Read updated package.json
	updatedContent, err := os.ReadFile(packageJSON)
	if err != nil {
		t.Fatalf("Failed to read updated package.json: %v", err)
	}

	// Parse and verify
	var pkgJSON map[string]interface{}
	if err := json.Unmarshal(updatedContent, &pkgJSON); err != nil {
		t.Fatalf("Failed to parse updated package.json: %v", err)
	}

	// @babel/helpers@7.26.0 is direct + vulnerable → expect:
	// 1. Dependency entry bumped in place (name unchanged, no aliasing)
	// 2. Plain version-scoped override for transitive uses
	deps := pkgJSON["dependencies"].(map[string]interface{})

	expectedVersion := "7.26.0-root.io.2"
	if deps["@babel/helpers"] == nil {
		t.Fatal("Expected @babel/helpers to remain in dependencies")
	} else if deps["@babel/helpers"].(string) != expectedVersion {
		t.Errorf("Expected @babel/helpers version %q, got %q", expectedVersion, deps["@babel/helpers"])
	}

	// Should have a plain version-scoped override for transitive uses
	overrides, hasOverrides := pkgJSON["overrides"].(map[string]interface{})
	if !hasOverrides {
		t.Error("expected overrides field with version-scoped override")
	}
	if hasOverrides {
		found := false
		for key, val := range overrides {
			if strings.HasPrefix(key, "@babel/helpers@") && val == expectedVersion {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected plain version-scoped override for @babel/helpers in overrides, got %+v", overrides)
		}
	}

	t.Logf("✓ Correctly bumped direct dep in place: @babel/helpers@%s", expectedVersion)
}
