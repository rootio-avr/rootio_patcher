package golang

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	assert.Error(t, err)
}

func TestGoLangApp_Run_NoPackages(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	goModPath := writeGoMod(t, t.TempDir(), "module example.com/app\n\ngo 1.21\n")

	app := NewAppWithServices(
		"test-key", "https://api.root.io", goModPath, true, logger,
		&MockGoModParser{}, &MockAPIClient{}, &MockCommandRunner{},
	)

	require.NoError(t, app.Run(ctx))
}

func TestGoLangApp_Run_APIError(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	goModPath := writeGoMod(t, t.TempDir(), "module example.com/app\n\ngo 1.21\n")
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
	assert.ErrorIs(t, err, apiErr)
}

func TestGoLangApp_Run_NoPatches(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	goModPath := writeGoMod(t, t.TempDir(), "module example.com/app\n\ngo 1.21\n")

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

	require.NoError(t, app.Run(ctx))
}

func TestGoLangApp_Run_DryRun(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	originalContent := "module example.com/app\n\ngo 1.21\n\nrequire github.com/google/uuid v1.3.0\n"
	goModPath := writeGoMod(t, t.TempDir(), originalContent)
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

	require.NoError(t, app.Run(ctx))

	content, err := os.ReadFile(goModPath)
	require.NoError(t, err)
	assert.Equal(t, originalContent, string(content), "go.mod must not be modified in dry-run mode")
	assert.Empty(t, cmdRunner.Calls, "no commands should be run in dry-run mode")
}

func TestGoLangApp_Run_ApplyPatches(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	goModPath := writeGoMod(t, t.TempDir(), "module example.com/app\n\ngo 1.21\n\nrequire github.com/google/uuid v1.3.0\n")
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

	require.NoError(t, app.Run(ctx))

	content, err := os.ReadFile(goModPath)
	require.NoError(t, err)
	assert.Equal(t, patchedContent, string(content))

	require.Len(t, cmdRunner.Calls, 1, "expected exactly one command (go mod tidy)")
	assert.Equal(t, "go", cmdRunner.Calls[0].Name)
	assert.Equal(t, "mod tidy", strings.Join(cmdRunner.Calls[0].Args, " "))
}

func TestGoLangApp_Run_ApplyPatches_WithVendor(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	dir := t.TempDir()
	goModPath := writeGoMod(t, dir, "module example.com/app\n\ngo 1.21\n\nrequire github.com/google/uuid v1.3.0\n")

	vendorDir := filepath.Join(dir, "vendor")
	require.NoError(t, os.MkdirAll(vendorDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(vendorDir, "modules.txt"), []byte("# vendor modules"), 0644))

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

	require.NoError(t, app.Run(ctx))

	require.Len(t, cmdRunner.Calls, 2, "expected go mod tidy and go mod vendor")
	assert.Equal(t, "mod tidy", strings.Join(cmdRunner.Calls[0].Args, " "))
	assert.Equal(t, "mod vendor", strings.Join(cmdRunner.Calls[1].Args, " "))
}

func TestGoLangApp_Run_APICalledWithGolangEcosystem(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	goModPath := writeGoMod(t, t.TempDir(), "module example.com/app\n\ngo 1.21\n")

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
	assert.Equal(t, "golang", capturedEcosystem)
}
