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
		"https://pkg.root.io",
		"npm",
		false, // not dry-run
		true,  // useAlias (default)
		nil,
		logger,
		NewParser(),
		mockAPIClient,
		&MockCommandRunner{},
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

	// @babel/helpers@7.26.0 is direct + vulnerable → the dependency VALUE is rewritten to
	// the npm: alias while the original "@babel/helpers" key is kept; no key rename, no override.
	deps := pkgJSON["dependencies"].(map[string]interface{})

	expectedAlias := "npm:@rootio/babel__helpers@7.26.0-root.io.2"
	if v, _ := deps["@babel/helpers"].(string); v != expectedAlias {
		t.Errorf("Expected @babel/helpers rewritten to %q, got %q", expectedAlias, deps["@babel/helpers"])
	}
	// Key must NOT be renamed to the scoped name.
	if _, exists := deps["@rootio/babel__helpers"]; exists {
		t.Error("direct dep key must not be renamed to @rootio/babel__helpers")
	}
	// A direct dep must NOT also get an overrides entry (EOVERRIDE).
	if overrides, hasOverrides := pkgJSON["overrides"].(map[string]interface{}); hasOverrides {
		for key := range overrides {
			if strings.HasPrefix(key, "@babel/helpers@") {
				t.Errorf("direct dep must not get an overrides entry, found %q", key)
			}
		}
	}

	t.Logf("✓ Correctly rewrote direct dep @babel/helpers → %s", expectedAlias)
}
