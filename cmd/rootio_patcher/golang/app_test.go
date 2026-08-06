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
		"test-key", "https://api.root.io", "https://pkg.root.io", "/nonexistent/go.mod", true, nil, logger,
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
		"test-key", "https://api.root.io", "https://pkg.root.io", goModPath, true, nil, logger,
		NewGoModParser(logger), &MockAPIClient{}, &MockCommandRunner{},
	)

	require.NoError(t, app.Run(ctx))
}

func TestGoLangApp_Run_WritesReport(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	dir := t.TempDir()
	goModPath := writeGoMod(t, dir, `module example.com/app

go 1.21

require golang.org/x/net v0.17.0
`)
	patch := rootio.PackagePatch{
		PackageName: "golang.org/x/net",
		Version:     "v0.17.0",
		Patch:       rootio.PatchInfo{Name: "golang.org/x/net", Version: "v0.17.0-aikido.1"},
		CVEIDs:      []string{"CVE-2023-44487", "CVE-2023-39325"},
	}
	client := &MockAPIClient{
		AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, _ []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
			return &rootio.AnalyzePackagesResponse{Patches: []rootio.PackagePatch{patch}}, nil
		},
	}

	reportPath := filepath.Join(dir, "report.json")
	app := NewApp(
		"test-key", "https://api.root.io", "https://pkg.root.io", goModPath, true, nil, logger,
		NewGoModParser(logger), client, &MockCommandRunner{},
	).WithReport(reportPath)

	require.NoError(t, app.Run(ctx))

	data, err := os.ReadFile(reportPath)
	require.NoError(t, err)
	assert.JSONEq(t, `[{
		"name": "golang.org/x/net",
		"old_version": "v0.17.0",
		"new_version": "v0.17.0-aikido.1",
		"cve_ids": ["CVE-2023-44487", "CVE-2023-39325"]
	}]`, string(data))
}

func TestGoLangApp_Run_WritesEmptyReportWhenNothingToFix(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	dir := t.TempDir()
	goModPath := writeGoMod(t, dir, `module example.com/app

go 1.21

require github.com/google/uuid v1.3.0
`)
	client := &MockAPIClient{
		AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, _ []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
			return &rootio.AnalyzePackagesResponse{Patches: []rootio.PackagePatch{}}, nil
		},
	}

	reportPath := filepath.Join(dir, "report.json")
	app := NewApp(
		"test-key", "https://api.root.io", "https://pkg.root.io", goModPath, true, nil, logger,
		NewGoModParser(logger), client, &MockCommandRunner{},
	).WithReport(reportPath)

	require.NoError(t, app.Run(ctx))

	data, err := os.ReadFile(reportPath)
	require.NoError(t, err)
	assert.JSONEq(t, "[]", string(data), "an empty report distinguishes nothing-to-fix from never-ran")
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
		"test-key", "https://api.root.io", "https://pkg.root.io", goModPath, true, nil, logger,
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
		"test-key", "https://api.root.io", "https://pkg.root.io", goModPath, true, nil, logger,
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

func TestGoLangApp_Run_APICalledWithGolangEcosystem(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	goModPath := writeGoMod(t, t.TempDir(), `module example.com/app

go 1.21

require github.com/google/uuid v1.3.0
`)

	var capturedEcosystem string
	app := NewApp(
		"test-key", "https://api.root.io", "https://pkg.root.io", goModPath, true, nil, logger,
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
		"test-key", "https://api.root.io", "https://pkg.root.io", goModPath, true,
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

func TestGoLangApp_Run_AddsSamePathReplaceDirective(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	dir := t.TempDir()
	originalContent := "module example.com/app\n\ngo 1.21\n\nrequire github.com/google/uuid v1.3.0\n"
	goModPath := writeGoMod(t, dir, originalContent)

	// Pre-populate go.sum; the tool must not touch it — go.sum maintenance is left to the
	// real `go mod tidy` invocation (mocked here).
	goSumPath := filepath.Join(dir, "go.sum")
	goSumContent := "github.com/google/uuid v1.3.0 h1:t6JiXb...=\n" +
		"github.com/google/uuid v1.3.0/go.mod h1:TIyP...=\n" +
		"github.com/pkg/errors v0.9.1 h1:abc=\n"
	require.NoError(t, os.WriteFile(goSumPath, []byte(goSumContent), 0644))

	cmdRunner := &MockCommandRunner{}

	app := NewApp(
		"test-key", "https://api.root.io", "https://pkg.root.io", goModPath, false, nil, logger,
		NewGoModParser(logger),
		&MockAPIClient{
			AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, _ []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
				return &rootio.AnalyzePackagesResponse{
					Patches: []rootio.PackagePatch{
						{
							PackageName: "github.com/google/uuid",
							Version:     "v1.3.0",
							Patch:       rootio.PatchInfo{Name: "github.com/google/uuid", Version: "v1.3.0-root.io.1"},
						},
					},
				}, nil
			},
		},
		cmdRunner,
	)

	require.NoError(t, app.Run(ctx))

	// go.mod: require line stays untouched; a same-path replace pinned at the original
	// version is added, redirecting only the version to the patched artifact.
	content, err := os.ReadFile(goModPath)
	require.NoError(t, err)
	got := string(content)
	assert.True(t, strings.Contains(got, "github.com/google/uuid v1.3.0"), "original require must remain")
	assert.True(t, containsLine(got, "replace github.com/google/uuid v1.3.0 => github.com/google/uuid v1.3.0-root.io.1"),
		"expected a same-path replace directive pinned to the original version")

	// go.sum must be left untouched by our own code — pruning/updating it is the real
	// `go mod tidy` invocation's job, not something we do ourselves anymore.
	sumContent, err := os.ReadFile(goSumPath)
	require.NoError(t, err)
	assert.Equal(t, goSumContent, string(sumContent), "go.sum must not be modified directly by the tool")

	// Only `go mod tidy` should run — no `go get`, since the replace directive alone
	// causes `go mod tidy` to fetch and verify the patched artifact.
	require.Len(t, cmdRunner.Calls, 1, "expected only go mod tidy")
	assert.Equal(t, []string{"mod", "tidy"}, cmdRunner.Calls[0].Args)

	// GONOSUMDB must be scoped to the patched module path only (not wildcard) since the
	// patched version's checksum isn't in the public sumdb.
	assert.Contains(t, cmdRunner.Calls[0].Env, "GONOSUMDB=github.com/google/uuid")
	assert.Contains(t, cmdRunner.Calls[0].Env, "GOPROXY=https://:test-key@pkg.root.io/gobinary,https://proxy.golang.org,direct")
}

func TestGoLangApp_Run_DryRun_GoModUnchanged(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	originalContent := "module example.com/app\n\ngo 1.21\n\nrequire github.com/google/uuid v1.3.0\n"
	goModPath := writeGoMod(t, t.TempDir(), originalContent)
	cmdRunner := &MockCommandRunner{}

	app := NewApp(
		"test-key", "https://api.root.io", "https://pkg.root.io", goModPath, true /* dry-run */, nil, logger,
		NewGoModParser(logger),
		&MockAPIClient{
			AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, _ []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
				return &rootio.AnalyzePackagesResponse{
					Patches: []rootio.PackagePatch{
						{
							PackageName: "github.com/google/uuid",
							Version:     "v1.3.0",
							Patch:       rootio.PatchInfo{Name: "github.com/google/uuid", Version: "v1.3.0"},
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
