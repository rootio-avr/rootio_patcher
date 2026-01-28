package pip

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/cmd/rootio_patcher/config"
	"rootio_patcher/pkg/rootio"
)

func TestPipApp_Run_NoPackages(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	mockPipService := &MockPipService{
		ListPackagesFunc: func(ctx context.Context) ([]common.InstalledPackage, error) {
			return []common.InstalledPackage{}, nil
		},
	}

	mockAPIClient := &MockAPIClient{}

	cfg := &config.Config{}
	app := NewAppWithServices(cfg, "python", true, true, logger, mockPipService, mockAPIClient, nil)

	err := app.Run(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
}

func TestPipApp_Run_CollectorError(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	expectedError := errors.New("collector error")
	mockPipService := &MockPipService{
		ListPackagesFunc: func(ctx context.Context) ([]common.InstalledPackage, error) {
			return nil, expectedError
		},
	}

	mockAPIClient := &MockAPIClient{}

	cfg := &config.Config{}
	app := NewAppWithServices(cfg, "python", true, true, logger, mockPipService, mockAPIClient, nil)

	err := app.Run(ctx)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if !errors.Is(err, expectedError) {
		t.Fatalf("Expected error to wrap collector error, got: %v", err)
	}
}

func TestPipApp_Run_APIError(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	mockPipService := &MockPipService{
		ListPackagesFunc: func(ctx context.Context) ([]common.InstalledPackage, error) {
			return []common.InstalledPackage{
				{Name: "django", Version: "4.0.0"},
			}, nil
		},
	}

	expectedError := errors.New("API error")
	mockAPIClient := &MockAPIClient{
		AnalyzePackagesFunc: func(ctx context.Context, packages []rootio.Package) (*rootio.AnalyzePackagesResponse, error) {
			return nil, expectedError
		},
	}


	cfg := &config.Config{}
	app := NewAppWithServices(cfg, "python", true, true, logger, mockPipService, mockAPIClient, nil)

	err := app.Run(ctx)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if !errors.Is(err, expectedError) {
		t.Fatalf("Expected error to wrap API error, got: %v", err)
	}
}

func TestPipApp_Run_NoPatches(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	mockPipService := &MockPipService{
		ListPackagesFunc: func(ctx context.Context) ([]common.InstalledPackage, error) {
			return []common.InstalledPackage{
				{Name: "django", Version: "4.0.0"},
			}, nil
		},
	}

	mockAPIClient := &MockAPIClient{
		AnalyzePackagesFunc: func(ctx context.Context, packages []rootio.Package) (*rootio.AnalyzePackagesResponse, error) {
			return &rootio.AnalyzePackagesResponse{
				Patches: []rootio.PackagePatch{},
				Skipped: []rootio.SkippedPackage{},
			}, nil
		},
	}


	cfg := &config.Config{}
	app := NewAppWithServices(cfg, "python", true, true, logger, mockPipService, mockAPIClient, nil)

	err := app.Run(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
}

func TestPipApp_Run_DryRunWithPatches(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	mockPipService := &MockPipService{
		ListPackagesFunc: func(ctx context.Context) ([]common.InstalledPackage, error) {
			return []common.InstalledPackage{
				{Name: "django", Version: "4.0.0"},
			}, nil
		},
		ApplyPatchFunc: func(ctx context.Context, patch rootio.PackagePatch) error {
			t.Fatal("Should not apply patches in dry-run mode")
			return nil
		},
	}

	mockAPIClient := &MockAPIClient{
		AnalyzePackagesFunc: func(ctx context.Context, packages []rootio.Package) (*rootio.AnalyzePackagesResponse, error) {
			return &rootio.AnalyzePackagesResponse{
				Patches: []rootio.PackagePatch{
					{
						PackageName: "django",
						Version:     "4.0.0",
						Patch:       rootio.PatchInfo{Name: "django", Version: "4.0.1"},
						PatchAlias:  rootio.PatchInfo{Name: "rootio-django", Version: "4.0.1"},
						CVEIDs:      []string{"CVE-2023-1234"},
					},
				},
			}, nil
		},
	}

	mockReporter := common.NewReporter("https://pkg.root.io", logger)
	cfg := &config.Config{}
	app := NewAppWithServices(cfg, "python", true, true, logger, mockPipService, mockAPIClient, mockReporter)

	err := app.Run(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
}

func TestPipApp_Run_ApplyPatches(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	patchApplied := false
	mockPipService := &MockPipService{
		ListPackagesFunc: func(ctx context.Context) ([]common.InstalledPackage, error) {
			return []common.InstalledPackage{
				{Name: "django", Version: "4.0.0"},
			}, nil
		},
		ApplyPatchFunc: func(ctx context.Context, patch rootio.PackagePatch) error {
			patchApplied = true
			if patch.PackageName != "django" {
				t.Errorf("Expected package name 'django', got '%s'", patch.PackageName)
			}
			return nil
		},
	}

	mockAPIClient := &MockAPIClient{
		AnalyzePackagesFunc: func(ctx context.Context, packages []rootio.Package) (*rootio.AnalyzePackagesResponse, error) {
			return &rootio.AnalyzePackagesResponse{
				Patches: []rootio.PackagePatch{
					{
						PackageName: "django",
						Version:     "4.0.0",
						Patch:       rootio.PatchInfo{Name: "django", Version: "4.0.1"},
						PatchAlias:  rootio.PatchInfo{Name: "rootio-django", Version: "4.0.1"},
						CVEIDs:      []string{"CVE-2023-1234"},
					},
				},
			}, nil
		},
	}

	mockReporter := common.NewReporter("https://pkg.root.io", logger)
	cfg := &config.Config{}
	app := NewAppWithServices(cfg, "python", false, true, logger, mockPipService, mockAPIClient, mockReporter)

	err := app.Run(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if !patchApplied {
		t.Fatal("Expected patch to be applied")
	}
}

func TestPipApp_Run_ApplyPatchError(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	expectedError := errors.New("pip install failed")
	mockPipService := &MockPipService{
		ListPackagesFunc: func(ctx context.Context) ([]common.InstalledPackage, error) {
			return []common.InstalledPackage{
				{Name: "django", Version: "4.0.0"},
			}, nil
		},
		ApplyPatchFunc: func(ctx context.Context, patch rootio.PackagePatch) error {
			return expectedError
		},
	}

	mockAPIClient := &MockAPIClient{
		AnalyzePackagesFunc: func(ctx context.Context, packages []rootio.Package) (*rootio.AnalyzePackagesResponse, error) {
			return &rootio.AnalyzePackagesResponse{
				Patches: []rootio.PackagePatch{
					{
						PackageName: "django",
						Version:     "4.0.0",
						Patch:       rootio.PatchInfo{Name: "django", Version: "4.0.1"},
						PatchAlias:  rootio.PatchInfo{Name: "rootio-django", Version: "4.0.1"},
					},
				},
			}, nil
		},
	}

	mockReporter := common.NewReporter("https://pkg.root.io", logger)
	cfg := &config.Config{}
	app := NewAppWithServices(cfg, "python", false, true, logger, mockPipService, mockAPIClient, mockReporter)

	err := app.Run(ctx)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
}

func TestPipApp_Run_PipPackageUsesSpecialHandler(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	pipHandlerCalled := false
	mockPipService := &MockPipService{
		ListPackagesFunc: func(ctx context.Context) ([]common.InstalledPackage, error) {
			return []common.InstalledPackage{
				{Name: "pip", Version: "21.0.0"},
			}, nil
		},
		ApplyPatchFunc: func(ctx context.Context, patch rootio.PackagePatch) error {
			t.Fatal("Should use ApplyPatchForPip for pip package")
			return nil
		},
		ApplyPatchForPipFunc: func(ctx context.Context, patch rootio.PackagePatch) error {
			pipHandlerCalled = true
			return nil
		},
	}

	mockAPIClient := &MockAPIClient{
		AnalyzePackagesFunc: func(ctx context.Context, packages []rootio.Package) (*rootio.AnalyzePackagesResponse, error) {
			return &rootio.AnalyzePackagesResponse{
				Patches: []rootio.PackagePatch{
					{
						PackageName: "pip",
						Version:     "21.0.0",
						Patch:       rootio.PatchInfo{Name: "pip", Version: "21.3.0"},
						PatchAlias:  rootio.PatchInfo{Name: "pip", Version: "21.3.0"},
					},
				},
			}, nil
		},
	}

	mockReporter := common.NewReporter("https://pkg.root.io", logger)
	cfg := &config.Config{}
	app := NewAppWithServices(cfg, "python", false, false, logger, mockPipService, mockAPIClient, mockReporter)

	err := app.Run(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if !pipHandlerCalled {
		t.Fatal("Expected ApplyPatchForPip to be called for pip package")
	}
}

func TestPipApp_Run_MultiplePatches(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	patchCount := 0
	mockPipService := &MockPipService{
		ListPackagesFunc: func(ctx context.Context) ([]common.InstalledPackage, error) {
			return []common.InstalledPackage{
				{Name: "django", Version: "4.0.0"},
				{Name: "flask", Version: "2.0.0"},
			}, nil
		},
		ApplyPatchFunc: func(ctx context.Context, patch rootio.PackagePatch) error {
			patchCount++
			return nil
		},
	}

	mockAPIClient := &MockAPIClient{
		AnalyzePackagesFunc: func(ctx context.Context, packages []rootio.Package) (*rootio.AnalyzePackagesResponse, error) {
			return &rootio.AnalyzePackagesResponse{
				Patches: []rootio.PackagePatch{
					{
						PackageName: "django",
						Version:     "4.0.0",
						Patch:       rootio.PatchInfo{Name: "django", Version: "4.0.1"},
						PatchAlias:  rootio.PatchInfo{Name: "rootio-django", Version: "4.0.1"},
					},
					{
						PackageName: "flask",
						Version:     "2.0.0",
						Patch:       rootio.PatchInfo{Name: "flask", Version: "2.0.1"},
						PatchAlias:  rootio.PatchInfo{Name: "rootio-flask", Version: "2.0.1"},
					},
				},
			}, nil
		},
	}

	mockReporter := common.NewReporter("https://pkg.root.io", logger)
	cfg := &config.Config{}
	app := NewAppWithServices(cfg, "python", false, true, logger, mockPipService, mockAPIClient, mockReporter)

	err := app.Run(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if patchCount != 2 {
		t.Fatalf("Expected 2 patches to be applied, got %d", patchCount)
	}
}
