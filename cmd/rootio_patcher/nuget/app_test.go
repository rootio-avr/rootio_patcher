package nuget

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

func testAppLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, nil))
}

func TestNuGetApp_Run_PathNotFound(t *testing.T) {
	ctx := context.Background()
	app := NewAppWithServices(
		"test-key", "https://api.root.io",
		"/nonexistent/path",
		true,
		true,
		nil,
		testAppLogger(),
		&MockParser{},
		&MockAPIClient{},
	)
	if err := app.Run(ctx); err == nil {
		t.Fatal("expected error for nonexistent path, got nil")
	}
}

func TestNuGetApp_Run_NoPackages(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	csproj := filepath.Join(tmpDir, "MyApp.csproj")
	if err := os.WriteFile(csproj, []byte(`<Project></Project>`), 0644); err != nil {
		t.Fatal(err)
	}

	app := NewAppWithServices(
		"test-key", "https://api.root.io",
		csproj,
		true,
		true,
		nil,
		testAppLogger(),
		&MockParser{},
		&MockAPIClient{},
	)
	if err := app.Run(ctx); err != nil {
		t.Fatalf("expected no error for empty project, got: %v", err)
	}
}

func TestNuGetApp_Run_APIError(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	csproj := filepath.Join(tmpDir, "MyApp.csproj")
	if err := os.WriteFile(csproj, []byte(`<Project></Project>`), 0644); err != nil {
		t.Fatal(err)
	}

	apiErr := errors.New("api unavailable")
	app := NewAppWithServices(
		"test-key", "https://api.root.io",
		csproj,
		true,
		true,
		nil,
		testAppLogger(),
		&MockParser{
			ParseFunc: func(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
				return []common.PackageInfo{
					{Name: "Newtonsoft.Json", Version: "12.0.3"},
				}, nil
			},
		},
		&MockAPIClient{
			AnalyzePackagesFunc: func(ctx context.Context, packages []rootio.Package, ignore []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error) {
				return nil, apiErr
			},
		},
	)

	err := app.Run(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, apiErr) {
		t.Fatalf("expected api error, got: %v", err)
	}
}

func TestNuGetApp_Run_NoPatches(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	csproj := filepath.Join(tmpDir, "MyApp.csproj")
	if err := os.WriteFile(csproj, []byte(`<Project></Project>`), 0644); err != nil {
		t.Fatal(err)
	}

	app := NewAppWithServices(
		"test-key", "https://api.root.io",
		csproj,
		true,
		true,
		nil,
		testAppLogger(),
		&MockParser{
			ParseFunc: func(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
				return []common.PackageInfo{
					{Name: "Newtonsoft.Json", Version: "13.0.1"},
				}, nil
			},
		},
		&MockAPIClient{
			AnalyzePackagesFunc: func(ctx context.Context, packages []rootio.Package, ignore []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error) {
				return &rootio.AnalyzePackagesResponse{Patches: nil}, nil
			},
		},
	)

	if err := app.Run(ctx); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestNuGetApp_Run_DryRun(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	csproj := filepath.Join(tmpDir, "MyApp.csproj")
	original := `<Project Sdk="Microsoft.NET.Sdk">
  <ItemGroup>
    <PackageReference Include="Newtonsoft.Json" Version="12.0.3" />
  </ItemGroup>
</Project>`
	if err := os.WriteFile(csproj, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	app := NewAppWithServices(
		"test-key", "https://api.root.io",
		csproj,
		true, // dry-run
		true,
		nil,
		testAppLogger(),
		&MockParser{
			ParseFunc: func(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
				return []common.PackageInfo{
					{Name: "Newtonsoft.Json", Version: "12.0.3"},
				}, nil
			},
		},
		&MockAPIClient{
			AnalyzePackagesFunc: func(ctx context.Context, packages []rootio.Package, ignore []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error) {
				return &rootio.AnalyzePackagesResponse{
					Patches: []rootio.PackagePatch{
						{PackageName: "Newtonsoft.Json", Version: "12.0.3", Patch: rootio.PatchInfo{Name: "Newtonsoft.Json", Version: "13.0.1"}},
					},
				}, nil
			},
		},
	)

	err := app.Run(ctx)
	if err == nil {
		t.Fatal("expected ErrPatchesAvailable in dry-run, got nil")
	}

	// File must not be modified
	content, _ := os.ReadFile(csproj)
	if !strings.Contains(string(content), `Version="12.0.3"`) {
		t.Error("file should not be modified in dry-run mode")
	}
}

func TestNuGetApp_Run_ApplyPatches(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	csproj := filepath.Join(tmpDir, "MyApp.csproj")
	original := `<Project Sdk="Microsoft.NET.Sdk">
  <ItemGroup>
    <PackageReference Include="Newtonsoft.Json" Version="12.0.3" />
  </ItemGroup>
</Project>`
	if err := os.WriteFile(csproj, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	app := NewAppWithServices(
		"test-key", "https://api.root.io",
		csproj,
		false, // NOT dry-run
		true,
		nil,
		testAppLogger(),
		&MockNuGetParser{
			NuGetParser: NewParser(testAppLogger()),
			ParseFunc: func(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
				return []common.PackageInfo{
					{Name: "Newtonsoft.Json", Version: "12.0.3"},
				}, nil
			},
		},
		&MockAPIClient{
			AnalyzePackagesFunc: func(ctx context.Context, packages []rootio.Package, ignore []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error) {
				return &rootio.AnalyzePackagesResponse{
					Patches: []rootio.PackagePatch{
						{
							PackageName: "Newtonsoft.Json",
							Version:     "12.0.3",
							Patch:       rootio.PatchInfo{Name: "Newtonsoft.Json", Version: "13.0.1"},
							PatchAlias:  rootio.PatchInfo{Name: "Newtonsoft.Json", Version: "13.0.1"},
						},
					},
				}, nil
			},
		},
	)

	if err := app.Run(ctx); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	content, _ := os.ReadFile(csproj)
	if !strings.Contains(string(content), `Version="13.0.1"`) {
		t.Error("expected file to contain updated version 13.0.1")
	}
	if strings.Contains(string(content), `Version="12.0.3"`) {
		t.Error("expected old version 12.0.3 to be replaced")
	}
}

func TestNuGetApp_Run_UsesPatchAlias(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	csproj := filepath.Join(tmpDir, "MyApp.csproj")
	original := `<Project Sdk="Microsoft.NET.Sdk">
  <ItemGroup>
    <PackageReference Include="Newtonsoft.Json" Version="12.0.3" />
  </ItemGroup>
</Project>`
	if err := os.WriteFile(csproj, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	app := NewAppWithServices(
		"test-key", "https://api.root.io",
		csproj,
		false,
		true,
		nil,
		testAppLogger(),
		&MockNuGetParser{
			NuGetParser: NewParser(testAppLogger()),
			ParseFunc: func(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
				return []common.PackageInfo{
					{Name: "Newtonsoft.Json", Version: "12.0.3", Location: csproj},
				}, nil
			},
		},
		&MockAPIClient{
			AnalyzePackagesFunc: func(ctx context.Context, packages []rootio.Package, ignore []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error) {
				return &rootio.AnalyzePackagesResponse{
					Patches: []rootio.PackagePatch{
						{
							PackageName: "Newtonsoft.Json",
							Version:     "12.0.3",
							Patch:       rootio.PatchInfo{Name: "Newtonsoft.Json", Version: "13.0.0"},              // non-alias
							PatchAlias:  rootio.PatchInfo{Name: "Rootio.Newtonsoft.Json", Version: "13.0.1-alias"}, // alias - should be used
						},
					},
				}, nil
			},
		},
	)

	if err := app.Run(ctx); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	content, _ := os.ReadFile(csproj)
	if !strings.Contains(string(content), `Include="Rootio.Newtonsoft.Json"`) {
		t.Error("expected PatchAlias name Rootio.Newtonsoft.Json to be used")
	}
	if !strings.Contains(string(content), `Version="13.0.1-alias"`) {
		t.Error("expected PatchAlias version 13.0.1-alias to be used, not Patch version")
	}
}

func TestNuGetApp_Run_UseAliasFalse(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	csproj := filepath.Join(tmpDir, "MyApp.csproj")
	original := `<Project Sdk="Microsoft.NET.Sdk">
  <ItemGroup>
    <PackageReference Include="Newtonsoft.Json" Version="12.0.3" />
  </ItemGroup>
</Project>`
	if err := os.WriteFile(csproj, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	app := NewAppWithServices(
		"test-key", "https://api.root.io",
		csproj,
		false,
		false, // useAlias=false: keep original package name
		nil,
		testAppLogger(),
		&MockNuGetParser{
			NuGetParser: NewParser(testAppLogger()),
			ParseFunc: func(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
				return []common.PackageInfo{
					{Name: "Newtonsoft.Json", Version: "12.0.3", Location: csproj},
				}, nil
			},
		},
		&MockAPIClient{
			AnalyzePackagesFunc: func(ctx context.Context, packages []rootio.Package, ignore []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error) {
				return &rootio.AnalyzePackagesResponse{
					Patches: []rootio.PackagePatch{
						{
							PackageName: "Newtonsoft.Json",
							Version:     "12.0.3",
							Patch:       rootio.PatchInfo{Name: "Newtonsoft.Json", Version: "13.0.0"},              // non-alias - should be used
							PatchAlias:  rootio.PatchInfo{Name: "Rootio.Newtonsoft.Json", Version: "13.0.1-alias"}, // alias
						},
					},
				}, nil
			},
		},
	)

	if err := app.Run(ctx); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	content, _ := os.ReadFile(csproj)
	if !strings.Contains(string(content), `Include="Newtonsoft.Json"`) {
		t.Error("expected original package name Newtonsoft.Json to be kept when useAlias=false")
	}
	if !strings.Contains(string(content), `Version="13.0.0"`) {
		t.Error("expected Patch version 13.0.0 to be used, not PatchAlias version")
	}
	if strings.Contains(string(content), "Rootio.Newtonsoft.Json") {
		t.Error("should not use aliased package name when useAlias=false")
	}
}

func TestNuGetApp_Run_EcosystemPassedToAPI(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	csproj := filepath.Join(tmpDir, "MyApp.csproj")
	if err := os.WriteFile(csproj, []byte(`<Project></Project>`), 0644); err != nil {
		t.Fatal(err)
	}

	var capturedEcosystem string
	app := NewAppWithServices(
		"test-key", "https://api.root.io",
		csproj,
		true,
		true,
		nil,
		testAppLogger(),
		&MockParser{
			ParseFunc: func(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
				return []common.PackageInfo{
					{Name: "Newtonsoft.Json", Version: "12.0.3"},
				}, nil
			},
		},
		&MockAPIClient{
			AnalyzePackagesFunc: func(ctx context.Context, packages []rootio.Package, ignore []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error) {
				capturedEcosystem = ecosystem
				return &rootio.AnalyzePackagesResponse{}, nil
			},
		},
	)

	_ = app.Run(ctx)
	if capturedEcosystem != "nuget" {
		t.Errorf("expected ecosystem 'nuget', got %q", capturedEcosystem)
	}
}
