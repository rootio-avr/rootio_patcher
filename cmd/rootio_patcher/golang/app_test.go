package golang

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

func TestGoLangApp_Run_FileNotFound(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	app := NewAppWithServices(
		"test-key", "https://api.root.io", "/nonexistent/go.mod", true, logger,
		&MockGoModParser{}, &MockAPIClient{}, &MockCommandRunner{},
	)

	err := app.Run(ctx)
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

func TestGoLangApp_Run_NoPackages(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	dir := t.TempDir()
	goModPath := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(goModPath, []byte("module example.com/app\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatalf("failed to create go.mod: %v", err)
	}

	app := NewAppWithServices(
		"test-key", "https://api.root.io", goModPath, true, logger,
		&MockGoModParser{}, &MockAPIClient{}, &MockCommandRunner{},
	)

	if err := app.Run(ctx); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestGoLangApp_Run_APIError(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	dir := t.TempDir()
	goModPath := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(goModPath, []byte("module example.com/app\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatalf("failed to create go.mod: %v", err)
	}

	apiErr := errors.New("API unavailable")
	app := NewAppWithServices(
		"test-key", "https://api.root.io", goModPath, true, logger,
		&MockGoModParser{
			ParseFunc: func(_ context.Context, _ string) ([]common.PackageInfo, error) {
				return []common.PackageInfo{{Name: "github.com/google/uuid", Version: "v1.3.0"}}, nil
			},
		},
		&MockAPIClient{
			AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
				return nil, apiErr
			},
		},
		&MockCommandRunner{},
	)

	err := app.Run(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, apiErr) {
		t.Errorf("expected error to wrap apiErr, got: %v", err)
	}
}

func TestGoLangApp_Run_NoPatches(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	dir := t.TempDir()
	goModPath := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(goModPath, []byte("module example.com/app\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatalf("failed to create go.mod: %v", err)
	}

	app := NewAppWithServices(
		"test-key", "https://api.root.io", goModPath, true, logger,
		&MockGoModParser{
			ParseFunc: func(_ context.Context, _ string) ([]common.PackageInfo, error) {
				return []common.PackageInfo{{Name: "github.com/google/uuid", Version: "v1.3.0"}}, nil
			},
		},
		&MockAPIClient{
			AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
				return &rootio.AnalyzePackagesResponse{Patches: []rootio.PackagePatch{}}, nil
			},
		},
		&MockCommandRunner{},
	)

	if err := app.Run(ctx); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestGoLangApp_Run_DryRun(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	dir := t.TempDir()
	goModPath := filepath.Join(dir, "go.mod")
	originalContent := "module example.com/app\n\ngo 1.21\n\nrequire github.com/google/uuid v1.3.0\n"
	if err := os.WriteFile(goModPath, []byte(originalContent), 0644); err != nil {
		t.Fatalf("failed to create go.mod: %v", err)
	}

	cmdRunner := &MockCommandRunner{}
	app := NewAppWithServices(
		"test-key", "https://api.root.io", goModPath, true /* dry-run */, logger,
		&MockGoModParser{
			ParseFunc: func(_ context.Context, _ string) ([]common.PackageInfo, error) {
				return []common.PackageInfo{{Name: "github.com/google/uuid", Version: "v1.3.0"}}, nil
			},
		},
		&MockAPIClient{
			AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
				return &rootio.AnalyzePackagesResponse{
					Patches: []rootio.PackagePatch{
						{
							PackageName: "github.com/google/uuid",
							Version:     "v1.3.0",
							PatchAlias:  rootio.PatchInfo{Name: "pkg.root.io/golang/github.com/google/uuid", Version: "v1.3.0-rootio.1"},
							CVEIDs:      []string{"CVE-2023-12345"},
						},
					},
				}, nil
			},
		},
		cmdRunner,
	)

	if err := app.Run(ctx); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// File must not be modified
	content, _ := os.ReadFile(goModPath)
	if string(content) != originalContent {
		t.Errorf("go.mod should not be modified in dry-run mode")
	}

	// No commands should be run
	if len(cmdRunner.Calls) != 0 {
		t.Errorf("expected no commands in dry-run mode, got %d", len(cmdRunner.Calls))
	}
}

func TestGoLangApp_Run_ApplyPatches(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	dir := t.TempDir()
	goModPath := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(goModPath, []byte("module example.com/app\n\ngo 1.21\n\nrequire github.com/google/uuid v1.3.0\n"), 0644); err != nil {
		t.Fatalf("failed to create go.mod: %v", err)
	}

	patchedContent := "module example.com/app\n\ngo 1.21\n\nrequire github.com/google/uuid v1.3.0\n\nreplace github.com/google/uuid v1.3.0 => pkg.root.io/golang/github.com/google/uuid v1.3.0-rootio.1\n"
	cmdRunner := &MockCommandRunner{}

	app := NewAppWithServices(
		"test-key", "https://api.root.io", goModPath, false /* not dry-run */, logger,
		&MockGoModParser{
			ParseFunc: func(_ context.Context, _ string) ([]common.PackageInfo, error) {
				return []common.PackageInfo{{Name: "github.com/google/uuid", Version: "v1.3.0"}}, nil
			},
			PatchFunc: func(_ context.Context, _ string, _ []GoModUpdate) (string, error) {
				return patchedContent, nil
			},
		},
		&MockAPIClient{
			AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
				return &rootio.AnalyzePackagesResponse{
					Patches: []rootio.PackagePatch{
						{
							PackageName: "github.com/google/uuid",
							Version:     "v1.3.0",
							PatchAlias:  rootio.PatchInfo{Name: "pkg.root.io/golang/github.com/google/uuid", Version: "v1.3.0-rootio.1"},
						},
					},
				}, nil
			},
		},
		cmdRunner,
	)

	if err := app.Run(ctx); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// File must be updated
	content, _ := os.ReadFile(goModPath)
	if string(content) != patchedContent {
		t.Errorf("go.mod should be patched, got:\n%s", string(content))
	}

	// go mod tidy must be called exactly once
	if len(cmdRunner.Calls) != 1 {
		t.Fatalf("expected 1 command call, got %d", len(cmdRunner.Calls))
	}
	call := cmdRunner.Calls[0]
	if call.Name != "go" || strings.Join(call.Args, " ") != "mod tidy" {
		t.Errorf("expected 'go mod tidy', got '%s %s'", call.Name, strings.Join(call.Args, " "))
	}
}

func TestGoLangApp_Run_ApplyPatches_WithVendor(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	dir := t.TempDir()
	goModPath := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(goModPath, []byte("module example.com/app\n\ngo 1.21\n\nrequire github.com/google/uuid v1.3.0\n"), 0644); err != nil {
		t.Fatalf("failed to create go.mod: %v", err)
	}

	// Create vendor/modules.txt to simulate a vendored project
	vendorDir := filepath.Join(dir, "vendor")
	if err := os.MkdirAll(vendorDir, 0755); err != nil {
		t.Fatalf("failed to create vendor dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vendorDir, "modules.txt"), []byte("# vendor modules"), 0644); err != nil {
		t.Fatalf("failed to create vendor/modules.txt: %v", err)
	}

	cmdRunner := &MockCommandRunner{}

	app := NewAppWithServices(
		"test-key", "https://api.root.io", goModPath, false, logger,
		&MockGoModParser{
			ParseFunc: func(_ context.Context, _ string) ([]common.PackageInfo, error) {
				return []common.PackageInfo{{Name: "github.com/google/uuid", Version: "v1.3.0"}}, nil
			},
			PatchFunc: func(_ context.Context, _ string, _ []GoModUpdate) (string, error) {
				return "patched content", nil
			},
		},
		&MockAPIClient{
			AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
				return &rootio.AnalyzePackagesResponse{
					Patches: []rootio.PackagePatch{
						{
							PackageName: "github.com/google/uuid",
							Version:     "v1.3.0",
							PatchAlias:  rootio.PatchInfo{Name: "pkg.root.io/golang/github.com/google/uuid", Version: "v1.3.0-rootio.1"},
						},
					},
				}, nil
			},
		},
		cmdRunner,
	)

	if err := app.Run(ctx); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Expect both go mod tidy and go mod vendor
	if len(cmdRunner.Calls) != 2 {
		t.Fatalf("expected 2 command calls (tidy + vendor), got %d", len(cmdRunner.Calls))
	}
	if strings.Join(cmdRunner.Calls[0].Args, " ") != "mod tidy" {
		t.Errorf("first call should be 'go mod tidy', got %v", cmdRunner.Calls[0].Args)
	}
	if strings.Join(cmdRunner.Calls[1].Args, " ") != "mod vendor" {
		t.Errorf("second call should be 'go mod vendor', got %v", cmdRunner.Calls[1].Args)
	}
}

func TestGoLangApp_Run_APICalledWithGolangEcosystem(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	dir := t.TempDir()
	goModPath := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(goModPath, []byte("module example.com/app\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatalf("failed to create go.mod: %v", err)
	}

	var capturedEcosystem string
	app := NewAppWithServices(
		"test-key", "https://api.root.io", goModPath, true, logger,
		&MockGoModParser{
			ParseFunc: func(_ context.Context, _ string) ([]common.PackageInfo, error) {
				return []common.PackageInfo{{Name: "github.com/google/uuid", Version: "v1.3.0"}}, nil
			},
		},
		&MockAPIClient{
			AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error) {
				capturedEcosystem = ecosystem
				return &rootio.AnalyzePackagesResponse{}, nil
			},
		},
		&MockCommandRunner{},
	)

	_ = app.Run(ctx)

	if capturedEcosystem != "golang" {
		t.Errorf("expected ecosystem 'golang', got %q", capturedEcosystem)
	}
}
