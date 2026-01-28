package npm

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/pkg/rootio"
)

// TestNpmApp_UpdatePackageJSON_Npm tests npm overrides format
func TestNpmApp_UpdatePackageJSON_Npm(t *testing.T) {
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
    "lodash": "4.17.20"
  }
}`
	if err := os.WriteFile(packageJSON, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	// Change to temp directory
	originalDir, _ := os.Getwd()
	defer os.Chdir(originalDir)
	os.Chdir(tmpDir)

	// Create app with mock services
	mockAPIClient := &MockAPIClient{
		AnalyzePackagesFunc: func(ctx context.Context, packages []rootio.Package) (*rootio.AnalyzePackagesResponse, error) {
			return &rootio.AnalyzePackagesResponse{
				Patches: []rootio.PackagePatch{
					{
						PackageName: "lodash",
						Version:     "4.17.20",
						Patch:       rootio.PatchInfo{Name: "lodash", Version: "4.17.21"},
						PatchAlias:  rootio.PatchInfo{Name: "@rootio/lodash", Version: "4.17.21"},
					},
				},
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
      "dependencies": { "lodash": "4.17.20" }
    },
    "node_modules/lodash": {
      "version": "4.17.20"
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

	// Verify overrides field exists
	overrides, ok := pkgJSON["overrides"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected 'overrides' field in package.json")
	}

	// Verify lodash override (should be aliased)
	lodashOverride, ok := overrides["lodash"].(string)
	if !ok {
		t.Fatal("Expected lodash in overrides")
	}
	expectedOverride := "npm:@rootio/lodash@4.17.21"
	if lodashOverride != expectedOverride {
		t.Errorf("Expected lodash override '%s', got '%s'", expectedOverride, lodashOverride)
	}

	t.Log("Successfully updated package.json with npm overrides (aliased packages)")
}

// TestNpmApp_UpdatePackageJSON_Yarn tests yarn resolutions format
func TestNpmApp_UpdatePackageJSON_Yarn(t *testing.T) {
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
    "express": "4.18.0"
  }
}`
	if err := os.WriteFile(packageJSON, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	// Change to temp directory
	originalDir, _ := os.Getwd()
	defer os.Chdir(originalDir)
	os.Chdir(tmpDir)

	// Create app with mock services
	mockAPIClient := &MockAPIClient{
		AnalyzePackagesFunc: func(ctx context.Context, packages []rootio.Package) (*rootio.AnalyzePackagesResponse, error) {
			return &rootio.AnalyzePackagesResponse{
				Patches: []rootio.PackagePatch{
					{
						PackageName: "express",
						Version:     "4.18.0",
						Patch:       rootio.PatchInfo{Name: "express", Version: "4.18.2"},
						PatchAlias:  rootio.PatchInfo{Name: "@rootio/express", Version: "4.18.2"},
					},
				},
			}, nil
		},
	}

	// Create a fake yarn.lock (just needs to exist)
	lockFile := filepath.Join(tmpDir, "yarn.lock")
	if err := os.WriteFile(lockFile, []byte("# yarn lockfile v1\n"), 0644); err != nil {
		t.Fatalf("Failed to create lock file: %v", err)
	}

	app := NewAppWithServices(
		"test-key",
		"https://api.root.io",
		"yarn",
		false, // not dry-run
		logger,
		&MockParser{
			ParseFunc: func(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
				return []common.PackageInfo{
					{Name: "express", Version: "4.18.0"},
				}, nil
			},
		},
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

	// Verify resolutions field exists
	resolutions, ok := pkgJSON["resolutions"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected 'resolutions' field in package.json for yarn")
	}

	// Verify express resolution (should be aliased)
	expressOverride, ok := resolutions["express"].(string)
	if !ok {
		t.Fatal("Expected express in resolutions")
	}
	expectedOverride := "npm:@rootio/express@4.18.2"
	if expressOverride != expectedOverride {
		t.Errorf("Expected express resolution '%s', got '%s'", expectedOverride, expressOverride)
	}

	t.Log("Successfully updated package.json with yarn resolutions (aliased packages)")
}

// TestNpmApp_UpdatePackageJSON_Pnpm tests pnpm overrides format (nested under "pnpm")
func TestNpmApp_UpdatePackageJSON_Pnpm(t *testing.T) {
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
    "jest": "29.0.0"
  }
}`
	if err := os.WriteFile(packageJSON, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	// Change to temp directory
	originalDir, _ := os.Getwd()
	defer os.Chdir(originalDir)
	os.Chdir(tmpDir)

	// Create app with mock services
	mockAPIClient := &MockAPIClient{
		AnalyzePackagesFunc: func(ctx context.Context, packages []rootio.Package) (*rootio.AnalyzePackagesResponse, error) {
			return &rootio.AnalyzePackagesResponse{
				Patches: []rootio.PackagePatch{
					{
						PackageName: "jest",
						Version:     "29.0.0",
						Patch:       rootio.PatchInfo{Name: "jest", Version: "29.5.0"},
						PatchAlias:  rootio.PatchInfo{Name: "@rootio/jest", Version: "29.5.0"},
					},
				},
			}, nil
		},
	}

	// Create a fake pnpm-lock.yaml (just needs to exist)
	lockFile := filepath.Join(tmpDir, "pnpm-lock.yaml")
	if err := os.WriteFile(lockFile, []byte("lockfileVersion: '6.0'\n"), 0644); err != nil {
		t.Fatalf("Failed to create lock file: %v", err)
	}

	app := NewAppWithServices(
		"test-key",
		"https://api.root.io",
		"pnpm",
		false, // not dry-run
		logger,
		&MockParser{
			ParseFunc: func(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
				return []common.PackageInfo{
					{Name: "jest", Version: "29.0.0"},
				}, nil
			},
		},
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

	// Verify pnpm field exists
	pnpmConfig, ok := pkgJSON["pnpm"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected 'pnpm' field in package.json for pnpm")
	}

	// Verify overrides nested under pnpm
	overrides, ok := pnpmConfig["overrides"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected 'overrides' field under 'pnpm' in package.json")
	}

	// Verify jest override (should be aliased)
	jestOverride, ok := overrides["jest"].(string)
	if !ok {
		t.Fatal("Expected jest in pnpm.overrides")
	}
	expectedOverride := "npm:@rootio/jest@29.5.0"
	if jestOverride != expectedOverride {
		t.Errorf("Expected jest override '%s', got '%s'", expectedOverride, jestOverride)
	}

	t.Log("Successfully updated package.json with pnpm overrides (nested under 'pnpm', aliased packages)")
}
