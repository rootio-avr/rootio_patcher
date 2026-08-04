package npm

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/pkg/rootio"
)

func TestNpmApp_Run_FileNotFound(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	app := NewAppWithServices(
		"test-key",
		"https://api.root.io",
		"/nonexistent/package-lock.json",
		true, // dryRun
		nil,
		logger,
		&MockParser{},
		&MockAPIClient{},
	)

	err := app.Run(ctx)
	if err == nil {
		t.Fatal("Expected error for nonexistent file, got nil")
	}
}

func TestNpmApp_Run_NoPackages(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create empty lock file
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "package-lock.json")
	content := `{"name": "test", "version": "1.0.0", "lockfileVersion": 3, "packages": {"": {}}}`
	if err := os.WriteFile(lockFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	app := NewAppWithServices(
		"test-key",
		"https://api.root.io",
		lockFile,
		true, // dryRun
		nil,
		logger,
		&MockParser{},
		&MockAPIClient{},
	)

	err := app.Run(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
}

func TestNpmApp_Run_APIError(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "package-lock.json")
	content := `{
  "name": "test",
  "lockfileVersion": 3,
  "packages": {
    "": {},
    "node_modules/lodash": {"version": "4.17.20"}
  }
}`
	if err := os.WriteFile(lockFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	expectedError := errors.New("API error")
	mockAPIClient := &MockAPIClient{
		AnalyzePackagesFunc: func(ctx context.Context, packages []rootio.Package, ignore []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error) {
			return nil, expectedError
		},
	}

	mockParser := &MockParser{
		ParseFunc: func(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
			return []common.PackageInfo{
				{Name: "lodash", Version: "4.17.20"},
			}, nil
		},
	}

	app := NewAppWithServices(
		"test-key",
		"https://api.root.io",
		lockFile,
		true, // dryRun
		nil,
		logger,
		mockParser,
		mockAPIClient,
	)

	err := app.Run(ctx)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if !errors.Is(err, expectedError) {
		t.Fatalf("Expected error to wrap API error, got: %v", err)
	}
}

func TestNpmApp_Run_NoPatches(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "package-lock.json")
	content := `{
  "name": "test",
  "lockfileVersion": 3,
  "packages": {
    "": {},
    "node_modules/lodash": {"version": "4.17.21"}
  }
}`
	if err := os.WriteFile(lockFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	mockAPIClient := &MockAPIClient{
		AnalyzePackagesFunc: func(ctx context.Context, packages []rootio.Package, ignore []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error) {
			return &rootio.AnalyzePackagesResponse{
				Patches: []rootio.PackagePatch{},
			}, nil
		},
	}

	app := NewAppWithServices(
		"test-key",
		"https://api.root.io",
		lockFile,
		true, // dryRun
		nil,
		logger,
		&MockParser{},
		mockAPIClient,
	)

	err := app.Run(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
}

func TestNpmApp_Run_DryRun(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "package-lock.json")
	content := `{
  "name": "test",
  "lockfileVersion": 3,
  "packages": {
    "": {"dependencies": {"lodash": "^4.17.20"}},
    "node_modules/lodash": {"version": "4.17.20"}
  }
}`
	if err := os.WriteFile(lockFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	// Create package.json (should not be modified in dry-run)
	packageJSON := filepath.Join(tmpDir, "package.json")
	packageContent := `{
  "name": "test",
  "version": "1.0.0",
  "dependencies": {
    "lodash": "^4.17.20"
  }
}`
	if err := os.WriteFile(packageJSON, []byte(packageContent), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	mockAPIClient := &MockAPIClient{
		AnalyzePackagesFunc: func(ctx context.Context, packages []rootio.Package, ignore []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error) {
			return &rootio.AnalyzePackagesResponse{
				Patches: []rootio.PackagePatch{
					{
						PackageName: "lodash",
						Version:     "4.17.20",
						Patch:       rootio.PatchInfo{Name: "lodash", Version: "4.17.21"},
						CVEIDs:      []string{"CVE-2021-23337"},
					},
				},
			}, nil
		},
	}

	mockParser := &MockParser{
		ParseFunc: func(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
			return []common.PackageInfo{
				{Name: "lodash", Version: "4.17.20"},
			}, nil
		},
	}

	app := NewAppWithServices(
		"test-key",
		"https://api.root.io",
		lockFile,
		true, // dry-run
		nil,
		logger,
		mockParser,
		mockAPIClient,
	)

	err := app.Run(ctx)
	if !errors.Is(err, common.ErrPatchesAvailable) {
		t.Fatalf("Expected ErrPatchesAvailable, got: %v", err)
	}

	// Verify package.json was NOT modified in dry-run mode
	updatedContent, err := os.ReadFile(packageJSON)
	if err != nil {
		t.Fatalf("Failed to read package.json: %v", err)
	}
	if strings.Contains(string(updatedContent), "overrides") {
		t.Error("package.json should not be modified in dry-run mode")
	}
	if strings.Contains(string(updatedContent), "4.17.21") {
		t.Error("package.json should not contain patches in dry-run mode")
	}
}

func TestNpmApp_Run_ApplyPatches(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "package-lock.json")
	lockContent := `{
  "name": "test",
  "lockfileVersion": 3,
  "packages": {
    "": {"dependencies": {"lodash": "^4.17.20"}},
    "node_modules/lodash": {
      "version": "4.17.20",
      "resolved": "https://registry.npmjs.org/lodash/-/lodash-4.17.20.tgz"
    }
  }
}`
	if err := os.WriteFile(lockFile, []byte(lockContent), 0644); err != nil {
		t.Fatalf("Failed to create lock file: %v", err)
	}

	// Create package.json
	packageJSON := filepath.Join(tmpDir, "package.json")
	packageContent := `{
  "name": "test",
  "version": "1.0.0",
  "dependencies": {
    "lodash": "^4.17.20"
  }
}`
	if err := os.WriteFile(packageJSON, []byte(packageContent), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	// Change to tmpDir so UpdatePackageJSON can find package.json
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	mockAPIClient := &MockAPIClient{
		AnalyzePackagesFunc: func(ctx context.Context, packages []rootio.Package, ignore []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error) {
			return &rootio.AnalyzePackagesResponse{
				Patches: []rootio.PackagePatch{
					{
						PackageName: "lodash",
						Version:     "4.17.20",
						Patch:       rootio.PatchInfo{Name: "lodash", Version: "4.17.21"},
						CVEIDs:      []string{"CVE-2021-23337"},
					},
				},
			}, nil
		},
	}

	// Use MockNpmParser that uses real UpdatePackageJSON but mocks Parse
	mockParser := &MockNpmParser{
		NpmParser: NewNpmParser(),
		ParseFunc: func(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
			return []common.PackageInfo{
				{Name: "lodash", Version: "4.17.20"},
			}, nil
		},
	}

	app := NewAppWithServices(
		"test-key",
		"https://api.root.io",
		lockFile,
		false, // NOT dry-run
		nil,
		logger,
		mockParser,
		mockAPIClient,
	)

	err := app.Run(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify package.json was patched. lodash is direct + vulnerable, so the
	// dependencies entry is bumped in place to the patched version (name is
	// unchanged since there's no aliasing), and a plain version-scoped
	// override is also added for any transitive consumers (see
	// TestNpmApp_UpdatePackageJSON_Npm for the same assertion via the full
	// integration path).
	updatedContent, err := os.ReadFile(packageJSON)
	if err != nil {
		t.Fatalf("Failed to read package.json: %v", err)
	}
	content := string(updatedContent)
	if !strings.Contains(content, `"lodash": "4.17.21"`) {
		t.Error("package.json should bump lodash direct dep in place to 4.17.21")
	}
	if !strings.Contains(content, `"lodash@4.17.20": "4.17.21"`) {
		t.Error("package.json should contain plain scoped override for lodash@4.17.20")
	}
}

func TestApp_isYarnClassic(t *testing.T) {
	dir := t.TempDir()
	classic := filepath.Join(dir, "classic.lock")
	berry := filepath.Join(dir, "berry.lock")
	missing := filepath.Join(dir, "missing.lock")

	if err := os.WriteFile(classic, []byte("# THIS IS AUTOGENERATED\n# yarn lockfile v1\n\nlodash@4.17.20:\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(berry, []byte("__metadata:\n  version: 8\n  cacheKey: 10c0\n"), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name           string
		packageManager string
		lockFilePath   string
		want           bool
	}{
		{"classic yarn1 lockfile + pm=yarn", "yarn", classic, true},
		{"berry lockfile + pm=yarn", "yarn", berry, false},
		{"classic header but pm=npm", "npm", classic, false},
		{"classic header but pm=pnpm", "pnpm", classic, false},
		{"missing lockfile", "yarn", missing, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &App{packageManager: tt.packageManager, lockFilePath: tt.lockFilePath}
			if got := a.isYarnClassic(); got != tt.want {
				t.Errorf("isYarnClassic() = %v, want %v", got, tt.want)
			}
		})
	}
}
