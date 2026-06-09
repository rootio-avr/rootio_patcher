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
		"test-key", "https://api.root.io", "https://pkg.root.io", "/nonexistent/go.mod", true, true, nil, logger,
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
		"test-key", "https://api.root.io", "https://pkg.root.io", goModPath, true, true, nil, logger,
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
		"test-key", "https://api.root.io", "https://pkg.root.io", goModPath, true, true, nil, logger,
		NewGoModParser(logger),
		&MockAPIClient{
			AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, _ []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
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
		"test-key", "https://api.root.io", "https://pkg.root.io", goModPath, true, true, nil, logger,
		NewGoModParser(logger),
		&MockAPIClient{
			AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, _ []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
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
		"test-key", "https://api.root.io", "https://pkg.root.io", goModPath, true /* dry-run */, true, nil, logger,
		NewGoModParser(logger),
		&MockAPIClient{
			AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, _ []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
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
		"test-key", "https://api.root.io", "https://pkg.root.io", goModPath, false /* not dry-run */, true, nil, logger,
		NewGoModParser(logger),
		&MockAPIClient{
			AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, _ []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
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
		"test-key", "https://api.root.io", "https://pkg.root.io", goModPath, false, true, nil, logger,
		NewGoModParser(logger),
		&MockAPIClient{
			AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, _ []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
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
		"my-api-key", "https://api.root.io", "https://pkg.root.io", goModPath, false, true, nil, logger,
		NewGoModParser(logger),
		&MockAPIClient{
			AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, _ []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
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
		"test-key", "https://api.root.io", "https://pkg.root.io", goModPath, true, true, nil, logger,
		NewGoModParser(logger),
		&MockAPIClient{
			AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, _ []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error) {
				capturedEcosystem = ecosystem
				return &rootio.AnalyzePackagesResponse{}, nil
			},
		},
		&MockCommandRunner{},
	)

	require.NoError(t, app.Run(ctx))
	assert.Equal(t, "golang", capturedEcosystem)
}

func TestGoLangApp_Run_ForwardsIgnoreListToAPI(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	goModPath := writeGoMod(t, t.TempDir(), `module example.com/app

go 1.21

require github.com/google/uuid v1.3.0
`)

	var capturedIgnore []rootio.Package
	app := NewApp(
		"test-key", "https://api.root.io", "https://pkg.root.io", goModPath, true, true,
		[]string{"github.com/google/uuid@v1.3.0"}, logger,
		NewGoModParser(logger),
		&MockAPIClient{
			AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, ignore []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
				capturedIgnore = ignore
				return &rootio.AnalyzePackagesResponse{}, nil
			},
		},
		&MockCommandRunner{},
	)

	require.NoError(t, app.Run(ctx))
	require.Len(t, capturedIgnore, 1)
	assert.Equal(t, "github.com/google/uuid", capturedIgnore[0].Name)
	assert.Equal(t, "v1.3.0", capturedIgnore[0].Version)
}

func TestGoLangApp_Run_NonAliased_NoReplaceDirectives(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	dir := t.TempDir()
	originalContent := "module example.com/app\n\ngo 1.21\n\nrequire github.com/google/uuid v1.3.0\n"
	goModPath := writeGoMod(t, dir, originalContent)

	// Pre-populate go.sum with entries for the patched module and an unrelated one.
	goSumPath := filepath.Join(dir, "go.sum")
	goSumContent := "github.com/google/uuid v1.3.0 h1:t6JiXb...=\n" +
		"github.com/google/uuid v1.3.0/go.mod h1:TIyP...=\n" +
		"github.com/pkg/errors v0.9.1 h1:abc=\n"
	require.NoError(t, os.WriteFile(goSumPath, []byte(goSumContent), 0644))

	cmdRunner := &MockCommandRunner{}

	app := NewApp(
		"test-key", "https://api.root.io", "https://pkg.root.io", goModPath, false, false /* useAlias=false */, nil, logger,
		NewGoModParser(logger),
		&MockAPIClient{
			AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, _ []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
				return &rootio.AnalyzePackagesResponse{
					Patches: []rootio.PackagePatch{
						{
							PackageName: "github.com/google/uuid",
							Version:     "v1.3.0",
							Patch:       rootio.PatchInfo{Name: "github.com/google/uuid", Version: "v1.3.0"},
							PatchAlias:  rootio.PatchInfo{Name: "pkg.root.io/golang/github.com/google/uuid", Version: "v1.3.0-rootio.1"},
						},
					},
				}, nil
			},
		},
		cmdRunner,
	)

	require.NoError(t, app.Run(ctx))

	// go.mod must not be modified — no replace directives added
	content, err := os.ReadFile(goModPath)
	require.NoError(t, err)
	assert.Equal(t, originalContent, string(content), "go.mod must not be modified in non-aliased mode")

	// go.sum entries for the patched module must be removed; unrelated entries preserved
	sumContent, err := os.ReadFile(goSumPath)
	require.NoError(t, err)
	assert.NotContains(t, string(sumContent), "github.com/google/uuid", "go.sum entries for patched module must be removed")
	assert.Contains(t, string(sumContent), "github.com/pkg/errors", "unrelated go.sum entries must be preserved")

	// Commands must be: go get uuid@v1.3.0, then go mod tidy
	require.Len(t, cmdRunner.Calls, 2, "expected go get + go mod tidy")
	assert.Equal(t, []string{"get", "github.com/google/uuid@v1.3.0"}, cmdRunner.Calls[0].Args)
	assert.Equal(t, []string{"mod", "tidy"}, cmdRunner.Calls[1].Args)

	// GONOSUMDB must be scoped to the patched module path only (not wildcard)
	assert.Contains(t, cmdRunner.Calls[0].Env, "GONOSUMDB=github.com/google/uuid")
	assert.Contains(t, cmdRunner.Calls[0].Env, "GOPROXY=https://:test-key@pkg.root.io/gobinary,https://proxy.golang.org,direct")
}

func TestGoLangApp_Run_NonAliased_DryRun_GoModUnchanged(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	originalContent := "module example.com/app\n\ngo 1.21\n\nrequire github.com/google/uuid v1.3.0\n"
	goModPath := writeGoMod(t, t.TempDir(), originalContent)
	cmdRunner := &MockCommandRunner{}

	app := NewApp(
		"test-key", "https://api.root.io", "https://pkg.root.io", goModPath, true /* dry-run */, false /* useAlias=false */, nil, logger,
		NewGoModParser(logger),
		&MockAPIClient{
			AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, _ []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
				return &rootio.AnalyzePackagesResponse{
					Patches: []rootio.PackagePatch{
						{
							PackageName: "github.com/google/uuid",
							Version:     "v1.3.0",
							Patch:       rootio.PatchInfo{Name: "github.com/google/uuid", Version: "v1.3.0"},
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
	assert.Equal(t, originalContent, string(content), "go.mod must not be modified in dry-run mode")
	assert.Empty(t, cmdRunner.Calls, "no commands should be run in dry-run mode")
}
