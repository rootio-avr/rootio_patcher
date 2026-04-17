package golang

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rootio_patcher/pkg/rootio"
)

// TestGoLangApp_E2E runs the full remediation flow against a real go.mod file,
// with only the API client mocked.
func TestGoLangApp_E2E(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	dir := t.TempDir()
	goModContent := `module example.com/myapp

go 1.21

require (
	github.com/google/uuid v1.3.0
	github.com/pkg/errors v0.9.1 // indirect
)
`
	goModPath := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(goModPath, []byte(goModContent), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	cmdRunner := &MockCommandRunner{}

	app := NewAppWithServices(
		"test-key",
		"https://api.root.io",
		goModPath,
		false, // not dry-run
		logger,
		NewGoModParser(logger), // real parser
		&MockAPIClient{
			AnalyzePackagesFunc: func(_ context.Context, packages []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error) {
				if ecosystem != "golang" {
					t.Errorf("expected ecosystem 'golang', got %q", ecosystem)
				}
				// Verify both packages were sent
				if len(packages) != 2 {
					t.Errorf("expected 2 packages sent to API, got %d", len(packages))
				}
				return &rootio.AnalyzePackagesResponse{
					Patches: []rootio.PackagePatch{
						{
							PackageName: "github.com/google/uuid",
							Version:     "v1.3.0",
							Patch:       rootio.PatchInfo{Name: "github.com/google/uuid", Version: "v1.3.0-rootio.1"},
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
		t.Fatalf("app.Run failed: %v", err)
	}

	// Assert go.mod was patched with the correct replace directive
	content, err := os.ReadFile(goModPath)
	if err != nil {
		t.Fatalf("failed to read go.mod after patching: %v", err)
	}
	gotContent := string(content)

	expectedReplace := "replace github.com/google/uuid v1.3.0 => pkg.root.io/golang/github.com/google/uuid v1.3.0-rootio.1"
	if !containsLine(gotContent, expectedReplace) {
		t.Errorf("expected go.mod to contain:\n  %q\ngot:\n%s", expectedReplace, gotContent)
	}

	// require directives must not be modified
	if !strings.Contains(gotContent, "github.com/google/uuid v1.3.0") {
		t.Errorf("original require entry should still be present, got:\n%s", gotContent)
	}

	// Assert go mod tidy was called
	if len(cmdRunner.Calls) != 1 {
		t.Fatalf("expected 1 command call (go mod tidy), got %d: %v", len(cmdRunner.Calls), cmdRunner.Calls)
	}
	tidyCall := cmdRunner.Calls[0]
	if tidyCall.Name != "go" || strings.Join(tidyCall.Args, " ") != "mod tidy" {
		t.Errorf("expected 'go mod tidy', got '%s %s'", tidyCall.Name, strings.Join(tidyCall.Args, " "))
	}
	if tidyCall.Dir != dir {
		t.Errorf("expected go mod tidy to run in %q, got %q", dir, tidyCall.Dir)
	}

	// Assert go mod vendor was NOT called (no vendor dir)
	for _, call := range cmdRunner.Calls {
		if strings.Join(call.Args, " ") == "mod vendor" {
			t.Errorf("go mod vendor should not be called when no vendor directory exists")
		}
	}
}

// TestGoLangApp_E2E_WithVendor verifies that go mod vendor is also called
// when a vendor/modules.txt file exists.
func TestGoLangApp_E2E_WithVendor(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	dir := t.TempDir()
	goModContent := `module example.com/myapp

go 1.21

require github.com/google/uuid v1.3.0
`
	goModPath := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(goModPath, []byte(goModContent), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	// Simulate a vendored project
	vendorDir := filepath.Join(dir, "vendor")
	if err := os.MkdirAll(vendorDir, 0755); err != nil {
		t.Fatalf("failed to create vendor dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vendorDir, "modules.txt"), []byte("# vendor modules\n"), 0644); err != nil {
		t.Fatalf("failed to write vendor/modules.txt: %v", err)
	}

	cmdRunner := &MockCommandRunner{}

	app := NewAppWithServices(
		"test-key",
		"https://api.root.io",
		goModPath,
		false,
		logger,
		NewGoModParser(logger), // real parser
		&MockAPIClient{
			AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
				return &rootio.AnalyzePackagesResponse{
					Patches: []rootio.PackagePatch{
						{
							PackageName: "github.com/google/uuid",
							Version:     "v1.3.0",
							Patch:       rootio.PatchInfo{Name: "github.com/google/uuid", Version: "v1.3.0-rootio.1"},
							PatchAlias:  rootio.PatchInfo{Name: "pkg.root.io/golang/github.com/google/uuid", Version: "v1.3.0-rootio.1"},
						},
					},
				}, nil
			},
		},
		cmdRunner,
	)

	if err := app.Run(ctx); err != nil {
		t.Fatalf("app.Run failed: %v", err)
	}

	// Assert go.mod was patched
	content, err := os.ReadFile(goModPath)
	if err != nil {
		t.Fatalf("failed to read go.mod after patching: %v", err)
	}
	expectedReplace := "replace github.com/google/uuid v1.3.0 => pkg.root.io/golang/github.com/google/uuid v1.3.0-rootio.1"
	if !containsLine(string(content), expectedReplace) {
		t.Errorf("expected replace directive in go.mod, got:\n%s", string(content))
	}

	// Assert both go mod tidy and go mod vendor were called
	if len(cmdRunner.Calls) != 2 {
		t.Fatalf("expected 2 command calls (tidy + vendor), got %d: %v", len(cmdRunner.Calls), cmdRunner.Calls)
	}

	tidyCall := cmdRunner.Calls[0]
	if strings.Join(tidyCall.Args, " ") != "mod tidy" {
		t.Errorf("first call should be 'go mod tidy', got %v", tidyCall.Args)
	}

	vendorCall := cmdRunner.Calls[1]
	if strings.Join(vendorCall.Args, " ") != "mod vendor" {
		t.Errorf("second call should be 'go mod vendor', got %v", vendorCall.Args)
	}
	if vendorCall.Dir != dir {
		t.Errorf("expected go mod vendor to run in %q, got %q", dir, vendorCall.Dir)
	}
}
