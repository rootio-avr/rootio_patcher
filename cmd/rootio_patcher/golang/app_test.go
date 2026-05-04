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

	"rootio_patcher/pkg/rootio"
)

func TestGoLangApp_Run_FileNotFound(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	app := NewApp(
		"test-key", "https://api.root.io", "https://pkg.root.io", "/nonexistent/go.mod", true, logger,
		NewGoModParser(logger), &MockAPIClient{}, &MockCommandRunner{},
	)

	assert.Error(t, app.Run(ctx))
}

func TestGoLangApp_Run_NoPackages(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// go.mod with only pseudo-version requires — parser should return nothing
	goModPath := writeGoMod(t, t.TempDir(), `module example.com/app

go 1.21

require golang.org/x/sys v0.0.0-20230101000000-abcdef123456 // indirect
`)

	app := NewApp(
		"test-key", "https://api.root.io", "https://pkg.root.io", goModPath, true, logger,
		NewGoModParser(logger), &MockAPIClient{}, &MockCommandRunner{},
	)

	require.NoError(t, app.Run(ctx))
}

func TestGoLangApp_Run_APIError(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	goModPath := writeGoMod(t, t.TempDir(), `module example.com/app

go 1.21

require github.com/google/uuid v1.3.0
`)
	apiErr := errors.New("API unavailable")

	app := NewApp(
		"test-key", "https://api.root.io", "https://pkg.root.io", goModPath, true, logger,
		NewGoModParser(logger),
		&MockAPIClient{
			AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
				return nil, apiErr
			},
		},
		&MockCommandRunner{},
	)

	assert.ErrorIs(t, app.Run(ctx), apiErr)
}

func TestGoLangApp_Run_NoPatches(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	goModPath := writeGoMod(t, t.TempDir(), `module example.com/app

go 1.21

require github.com/google/uuid v1.3.0
`)

	app := NewApp(
		"test-key", "https://api.root.io", "https://pkg.root.io", goModPath, true, logger,
		NewGoModParser(logger),
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

	app := NewApp(
		"test-key", "https://api.root.io", "https://pkg.root.io", goModPath, true /* dry-run */, logger,
		NewGoModParser(logger),
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

	goModPath := writeGoMod(t, t.TempDir(), `module example.com/app

go 1.21

require github.com/google/uuid v1.3.0
`)
	cmdRunner := &MockCommandRunner{}

	app := NewApp(
		"test-key", "https://api.root.io", "https://pkg.root.io", goModPath, false /* not dry-run */, logger,
		NewGoModParser(logger),
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
	assert.True(t, containsLine(string(content), "replace github.com/google/uuid v1.3.0 => pkg.root.io/golang/github.com/google/uuid v1.3.0-rootio.1"))
	// require directive must not be modified
	assert.True(t, strings.Contains(string(content), "require github.com/google/uuid v1.3.0"))

	require.Len(t, cmdRunner.Calls, 1, "expected exactly one command (go mod tidy)")
	assert.Equal(t, "go", cmdRunner.Calls[0].Name)
	assert.Equal(t, "mod tidy", strings.Join(cmdRunner.Calls[0].Args, " "))
}

func TestGoLangApp_Run_ApplyPatches_WithVendor(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	dir := t.TempDir()
	goModPath := writeGoMod(t, dir, `module example.com/app

go 1.21

require github.com/google/uuid v1.3.0
`)

	vendorDir := filepath.Join(dir, "vendor")
	require.NoError(t, os.MkdirAll(vendorDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(vendorDir, "modules.txt"), []byte("# vendor modules"), 0644))

	cmdRunner := &MockCommandRunner{}

	app := NewApp(
		"test-key", "https://api.root.io", "https://pkg.root.io", goModPath, false, logger,
		NewGoModParser(logger),
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

func TestGoLangApp_Run_ApplyPatches_SetsGoProxyEnv(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	goModPath := writeGoMod(t, t.TempDir(), `module example.com/app

go 1.21

require github.com/google/uuid v1.3.0
`)
	cmdRunner := &MockCommandRunner{}

	app := NewApp(
		"my-api-key", "https://api.root.io", "https://pkg.root.io", goModPath, false, logger,
		NewGoModParser(logger),
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

	require.Len(t, cmdRunner.Calls, 1)
	env := cmdRunner.Calls[0].Env
	assert.Contains(t, env, "GOPROXY=https://:my-api-key@pkg.root.io/gobinary,https://proxy.golang.org,direct")
	assert.Contains(t, env, "GONOSUMDB=pkg.root.io")
}

func TestGoLangApp_Run_APICalledWithGolangEcosystem(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	goModPath := writeGoMod(t, t.TempDir(), `module example.com/app

go 1.21

require github.com/google/uuid v1.3.0
`)

	var capturedEcosystem string
	app := NewApp(
		"test-key", "https://api.root.io", "https://pkg.root.io", goModPath, true, logger,
		NewGoModParser(logger),
		&MockAPIClient{
			AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error) {
				capturedEcosystem = ecosystem
				return &rootio.AnalyzePackagesResponse{}, nil
			},
		},
		&MockCommandRunner{},
	)

	require.NoError(t, app.Run(ctx))
	assert.Equal(t, "golang", capturedEcosystem)
}
