package npm

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rootio_patcher/pkg/rootio"
)

func TestNpmApp_Run_FileNotFound(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	app := NewAppWithServices(
		"test-key",
		"https://api.root.io",
		"/nonexistent/package-lock.json",
		true,
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
		true,
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
		AnalyzePackagesFunc: func(ctx context.Context, packages []rootio.Package) (*rootio.AnalyzePackagesResponse, error) {
			return nil, expectedError
		},
	}

	app := NewAppWithServices(
		"test-key",
		"https://api.root.io",
		lockFile,
		true,
		logger,
		&MockParser{},
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
		AnalyzePackagesFunc: func(ctx context.Context, packages []rootio.Package) (*rootio.AnalyzePackagesResponse, error) {
			return &rootio.AnalyzePackagesResponse{
				Patches: []rootio.PackagePatch{},
			}, nil
		},
	}

	app := NewAppWithServices(
		"test-key",
		"https://api.root.io",
		lockFile,
		true,
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

	mockAPIClient := &MockAPIClient{
		AnalyzePackagesFunc: func(ctx context.Context, packages []rootio.Package) (*rootio.AnalyzePackagesResponse, error) {
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

	app := NewAppWithServices(
		"test-key",
		"https://api.root.io",
		lockFile,
		true, // dry-run
		logger,
		&MockParser{},
		mockAPIClient,
	)

	err := app.Run(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify file was NOT modified
	updatedContent, err := os.ReadFile(lockFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if !strings.Contains(string(updatedContent), "4.17.20") {
		t.Error("File should not be modified in dry-run mode")
	}
}

func TestNpmApp_Run_ApplyPatches(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "package-lock.json")
	content := `{
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
	if err := os.WriteFile(lockFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	mockAPIClient := &MockAPIClient{
		AnalyzePackagesFunc: func(ctx context.Context, packages []rootio.Package) (*rootio.AnalyzePackagesResponse, error) {
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

	app := NewAppWithServices(
		"test-key",
		"https://api.root.io",
		lockFile,
		false, // NOT dry-run
		logger,
		&MockParser{},
		mockAPIClient,
	)

	err := app.Run(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify file was modified
	updatedContent, err := os.ReadFile(lockFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if !strings.Contains(string(updatedContent), "4.17.21") {
		t.Error("File should contain updated version 4.17.21")
	}
	if strings.Contains(string(updatedContent), `"version": "4.17.20"`) {
		t.Error("File should not contain old version 4.17.20")
	}
}
