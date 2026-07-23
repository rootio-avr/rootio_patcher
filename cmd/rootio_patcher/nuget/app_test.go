package nuget

import (
	"context"
	"errors"
	"fmt"
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
		"test-key", "https://api.root.io", "https://pkg.root.io",
		"/nonexistent/path",
		true,
		true,
		nil,
		testAppLogger(),
		&MockParser{},
		&MockAPIClient{},
		&MockCommandRunner{},
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
		"test-key", "https://api.root.io", "https://pkg.root.io",
		csproj,
		true,
		true,
		nil,
		testAppLogger(),
		&MockParser{},
		&MockAPIClient{},
		&MockCommandRunner{},
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
		"test-key", "https://api.root.io", "https://pkg.root.io",
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
		&MockCommandRunner{},
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
		"test-key", "https://api.root.io", "https://pkg.root.io",
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
		&MockCommandRunner{},
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
		"test-key", "https://api.root.io", "https://pkg.root.io",
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
		&MockCommandRunner{},
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

	mockCmd := &MockCommandRunner{}

	app := NewAppWithServices(
		"test-key", "https://api.root.io", "https://pkg.root.io",
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
		mockCmd,
	)

	// Make dotnet appear absent to skip resolver (we're testing file patching, not dotnet invocation)
	app.lookPath = func(name string) (string, error) { return "", os.ErrNotExist }

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

	mockCmd := &MockCommandRunner{}

	app := NewAppWithServices(
		"test-key", "https://api.root.io", "https://pkg.root.io",
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
		mockCmd,
	)

	// Make dotnet appear absent to skip resolver
	app.lookPath = func(name string) (string, error) { return "", os.ErrNotExist }

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

	mockCmd := &MockCommandRunner{}

	app := NewAppWithServices(
		"test-key", "https://api.root.io", "https://pkg.root.io",
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
		mockCmd,
	)

	// Make dotnet appear absent to skip resolver
	app.lookPath = func(name string) (string, error) { return "", os.ErrNotExist }

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
		"https://pkg.root.io",
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
		&MockCommandRunner{},
	)

	_ = app.Run(ctx)
	if capturedEcosystem != "nuget" {
		t.Errorf("expected ecosystem 'nuget', got %q", capturedEcosystem)
	}
}

func TestNuGetApp_Run_InvokesRestoreAfterPatch(t *testing.T) {
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

	mockAPIClient := &MockAPIClient{
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
	}

	mockParser := &MockNuGetParser{
		NuGetParser: NewParser(testAppLogger()),
		ParseFunc: func(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
			return []common.PackageInfo{
				{Name: "Newtonsoft.Json", Version: "12.0.3"},
			}, nil
		},
	}

	mockCmd := &MockCommandRunner{}

	app := NewAppWithServices(
		"test-key",
		"https://api.root.io",
		"https://pkg.root.io",
		csproj,
		false, // NOT dry-run
		true,
		nil,
		testAppLogger(),
		mockParser,
		mockAPIClient,
		mockCmd,
	)

	// Make dotnet appear available
	app.lookPath = func(name string) (string, error) { return "/usr/bin/" + name, nil }

	err := app.Run(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Assert dotnet restore was invoked
	require := func(cond bool, msg string) {
		if !cond {
			t.Fatal(msg)
		}
	}
	assert := func(cond bool, msg string) {
		if !cond {
			t.Error(msg)
		}
	}

	require(len(mockCmd.Calls) == 1, fmt.Sprintf("Expected 1 command call, got %d", len(mockCmd.Calls)))
	assert(mockCmd.Calls[0].Name == "dotnet", fmt.Sprintf("Expected command name 'dotnet', got '%s'", mockCmd.Calls[0].Name))
	found := false
	sawConfig := false
	for i, arg := range mockCmd.Calls[0].Args {
		if arg == "restore" {
			found = true
		}
		// The resolver must be pointed at the Root.io feed via `--configfile <NuGet.Config>`
		// so it can fetch -root.io.N packages (which exist only in the Root.io NuGet feed).
		if arg == "--configfile" && i+1 < len(mockCmd.Calls[0].Args) {
			sawConfig = true
		}
	}
	assert(found, fmt.Sprintf("Expected 'restore' in args, got: %v", mockCmd.Calls[0].Args))
	assert(sawConfig, fmt.Sprintf("Expected '--configfile <NuGet.Config>' (Root.io feed auth) in args, got: %v", mockCmd.Calls[0].Args))
}

func TestWriteNuGetConfig(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// With pkgURL + apiKey → a NuGet.Config with the Root.io feed + basic-auth credentials.
	app := &App{apiKey: "sk_nugetkey", pkgURL: "https://pkg.root.io", logger: logger}
	path, cleanup, err := app.writeNuGetConfig(t.TempDir())
	if err != nil {
		t.Fatalf("writeNuGetConfig error: %v", err)
	}
	if path == "" {
		t.Fatal("expected a NuGet.Config path")
	}
	defer cleanup()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read NuGet.Config: %v", err)
	}
	content := string(data)
	for _, want := range []string{
		`<add key="rootio" value="https://pkg.root.io/nuget/v3/index.json" />`,
		`<add key="Username" value="root" />`,
		`<add key="ClearTextPassword" value="sk_nugetkey" />`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("NuGet.Config missing %q; got:\n%s", want, content)
		}
	}

	// cleanup() must remove the file.
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected NuGet.Config removed after cleanup, stat err=%v", err)
	}

	// Missing pkgURL or apiKey → no config (restore without it).
	if p, _, _ := (&App{apiKey: "k", logger: logger}).writeNuGetConfig(t.TempDir()); p != "" {
		t.Errorf("expected no NuGet.Config when pkgURL empty, got %q", p)
	}
	if p, _, _ := (&App{pkgURL: "https://pkg.root.io", logger: logger}).writeNuGetConfig(t.TempDir()); p != "" {
		t.Errorf("expected no NuGet.Config when apiKey empty, got %q", p)
	}
}

func TestNuGetApp_Run_DryRunDoesNotInvokeRestore(t *testing.T) {
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

	mockAPIClient := &MockAPIClient{
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
	}

	mockParser := &MockParser{
		ParseFunc: func(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
			return []common.PackageInfo{
				{Name: "Newtonsoft.Json", Version: "12.0.3"},
			}, nil
		},
	}

	mockCmd := &MockCommandRunner{}

	app := NewAppWithServices(
		"test-key",
		"https://api.root.io",
		"https://pkg.root.io",
		csproj,
		true, // DRY-RUN
		true,
		nil,
		testAppLogger(),
		mockParser,
		mockAPIClient,
		mockCmd,
	)

	// Make dotnet appear available
	app.lookPath = func(name string) (string, error) { return "/usr/bin/" + name, nil }

	err := app.Run(ctx)
	// Dry-run returns early with ErrPatchesAvailable, not an actual error
	if err != common.ErrPatchesAvailable {
		t.Fatalf("Expected ErrPatchesAvailable, got: %v", err)
	}

	// Assert dotnet restore was NOT invoked
	if len(mockCmd.Calls) != 0 {
		t.Errorf("Expected no command calls in dry-run mode, got %d", len(mockCmd.Calls))
	}
}

func TestNuGetApp_Run_SkipsRestoreWhenDotnetAbsent(t *testing.T) {
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

	mockAPIClient := &MockAPIClient{
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
	}

	mockParser := &MockNuGetParser{
		NuGetParser: NewParser(testAppLogger()),
		ParseFunc: func(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
			return []common.PackageInfo{
				{Name: "Newtonsoft.Json", Version: "12.0.3"},
			}, nil
		},
	}

	mockCmd := &MockCommandRunner{}

	app := NewAppWithServices(
		"test-key",
		"https://api.root.io",
		"https://pkg.root.io",
		csproj,
		false, // NOT dry-run
		true,
		nil,
		testAppLogger(),
		mockParser,
		mockAPIClient,
		mockCmd,
	)

	// Make dotnet appear ABSENT
	app.lookPath = func(name string) (string, error) {
		return "", os.ErrNotExist
	}

	err := app.Run(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Assert dotnet restore was NOT invoked
	if len(mockCmd.Calls) != 0 {
		t.Errorf("Expected no command calls when dotnet is absent, got %d", len(mockCmd.Calls))
	}

	// Verify manifest was still patched
	updatedContent, err := os.ReadFile(csproj)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if !strings.Contains(string(updatedContent), "13.0.1") {
		t.Error("File should contain updated version 13.0.1 even when dotnet is absent")
	}
}
