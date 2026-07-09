package golang

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rootio_patcher/pkg/rootio"
)

// TestGoLangApp_E2E runs the full remediation flow against a real go.mod file,
// with only the API client mocked.
func TestGoLangApp_E2E(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	dir := t.TempDir()
	goModPath := writeGoMod(t, dir, `module example.com/myapp

go 1.21

require (
	github.com/google/uuid v1.3.0
	github.com/pkg/errors v0.9.1 // indirect
)
`)
	cmdRunner := &MockCommandRunner{}

	app := NewApp(
		"test-key",
		"https://api.root.io",
		"https://pkg.root.io",
		goModPath,
		false,
		true,
		nil,
		logger,
		NewGoModParser(logger), // real parser
		&MockAPIClient{
			AnalyzePackagesFunc: func(_ context.Context, packages []rootio.Package, ignore []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error) {
				assert.Equal(t, "golang", ecosystem)
				assert.Len(t, packages, 2, "both packages should be sent to the API")
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

	require.NoError(t, app.Run(ctx))

	content, err := os.ReadFile(goModPath)
	require.NoError(t, err)
	got := string(content)

	// a replace directive must be added
	assert.True(t, containsLine(got, "replace github.com/google/uuid v1.3.0 => pkg.root.io/golang/github.com/google/uuid v1.3.0-rootio.1"))
	// require directive must not be modified
	assert.True(t, strings.Contains(got, "github.com/google/uuid v1.3.0"))

	// go mod tidy must be called, go mod vendor must not
	require.Len(t, cmdRunner.Calls, 1, "expected only go mod tidy")
	assert.Equal(t, "go", cmdRunner.Calls[0].Name)
	assert.Equal(t, "mod tidy", strings.Join(cmdRunner.Calls[0].Args, " "))
	assert.Equal(t, dir, cmdRunner.Calls[0].Dir)
	assert.Contains(t, cmdRunner.Calls[0].Env, "GOPROXY=https://:test-key@pkg.root.io/gobinary,https://proxy.golang.org,direct")
	assert.Contains(t, cmdRunner.Calls[0].Env, "GONOSUMDB=pkg.root.io")
}

// TestGoLangApp_E2E_PreservesNonRootioReplaceDirective verifies that a pre-existing replace
// directive unrelated to Root.io patches is preserved after remediation.
func TestGoLangApp_E2E_PreservesNonRootioReplaceDirective(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	dir := t.TempDir()
	goModPath := writeGoMod(t, dir, `module example.com/myapp

go 1.21

require (
	github.com/google/uuid v1.3.0
	github.com/pkg/errors v0.9.1
)

replace github.com/pkg/errors v0.9.1 => github.com/pkg/errors-fork v0.9.1-custom
`)
	cmdRunner := &MockCommandRunner{}

	app := NewApp(
		"test-key",
		"https://api.root.io",
		"https://pkg.root.io",
		goModPath,
		false,
		true,
		nil,
		logger,
		NewGoModParser(logger),
		&MockAPIClient{
			AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, _ []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
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

	require.NoError(t, app.Run(ctx))

	content, err := os.ReadFile(goModPath)
	require.NoError(t, err)
	got := string(content)

	// rootio replace directive must be added
	assert.True(t, containsLine(got, "replace github.com/google/uuid v1.3.0 => pkg.root.io/golang/github.com/google/uuid v1.3.0-rootio.1"))
	// pre-existing non-rootio replace must be preserved
	assert.True(t, containsLine(got, "replace github.com/pkg/errors v0.9.1 => github.com/pkg/errors-fork v0.9.1-custom"))
}

// TestGoLangApp_E2E_UpgradesRootioReplaceRevision verifies that when a go.mod already contains
// a rootio replace directive from a prior run (e.g. revision 1) and the API responds with a
// higher revision (e.g. revision 2), only the higher revision remains in go.mod.
func TestGoLangApp_E2E_UpgradesRootioReplaceRevision(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	dir := t.TempDir()
	goModPath := writeGoMod(t, dir, `module example.com/myapp

go 1.21

require github.com/google/uuid v1.3.0

replace github.com/google/uuid v1.3.0 => pkg.root.io/golang/github.com/google/uuid v1.3.0-rootio.1
`)
	cmdRunner := &MockCommandRunner{}

	app := NewApp(
		"test-key",
		"https://api.root.io",
		"https://pkg.root.io",
		goModPath,
		false,
		true,
		nil,
		logger,
		NewGoModParser(logger),
		&MockAPIClient{
			AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, _ []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
				return &rootio.AnalyzePackagesResponse{
					Patches: []rootio.PackagePatch{
						{
							PackageName: "github.com/google/uuid",
							Version:     "v1.3.0",
							Patch:       rootio.PatchInfo{Name: "github.com/google/uuid", Version: "v1.3.0-rootio.2"},
							PatchAlias:  rootio.PatchInfo{Name: "pkg.root.io/golang/github.com/google/uuid", Version: "v1.3.0-rootio.2"},
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
	got := string(content)

	// revision 2 must be present
	assert.True(t, containsLine(got, "replace github.com/google/uuid v1.3.0 => pkg.root.io/golang/github.com/google/uuid v1.3.0-rootio.2"))
	// revision 1 must no longer appear
	assert.False(t, strings.Contains(got, "v1.3.0-rootio.1"))
}

func TestGoLangApp_E2E_NonAliased(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	dir := t.TempDir()
	goModPath := writeGoMod(t, dir, `module example.com/myapp

go 1.21

require (
	github.com/google/uuid v1.3.0
	github.com/pkg/errors v0.9.1 // indirect
)
`)
	goSumPath := filepath.Join(dir, "go.sum")
	goSumContent := "github.com/google/uuid v1.3.0 h1:t6JiXb...=\n" +
		"github.com/google/uuid v1.3.0/go.mod h1:TIyP...=\n" +
		"github.com/pkg/errors v0.9.1 h1:abc=\n" +
		"github.com/pkg/errors v0.9.1/go.mod h1:def=\n"
	require.NoError(t, os.WriteFile(goSumPath, []byte(goSumContent), 0644))

	cmdRunner := &MockCommandRunner{}

	app := NewApp(
		"test-key",
		"https://api.root.io",
		"https://pkg.root.io",
		goModPath,
		false,
		false, // useAlias=false
		nil,
		logger,
		NewGoModParser(logger),
		&MockAPIClient{
			AnalyzePackagesFunc: func(_ context.Context, packages []rootio.Package, _ []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error) {
				assert.Equal(t, "golang", ecosystem)
				assert.Len(t, packages, 2, "both packages should be sent to the API")
				return &rootio.AnalyzePackagesResponse{
					Patches: []rootio.PackagePatch{
						{
							PackageName: "github.com/google/uuid",
							Version:     "v1.3.0",
							Patch:       rootio.PatchInfo{Name: "github.com/google/uuid", Version: "v1.3.0-root.io.1"},
							PatchAlias:  rootio.PatchInfo{Name: "pkg.root.io/golang/github.com/google/uuid", Version: "v1.3.0-rootio.1"},
							CVEIDs:      []string{"CVE-2023-12345"},
						},
					},
					// pkg/errors is vulnerable but has no patch yet — API skips it.
					Skipped: []rootio.SkippedPackage{
						{PackageName: "github.com/pkg/errors", Reason: "no patch available"},
					},
				}, nil
			},
		},
		cmdRunner,
	)

	require.NoError(t, app.Run(ctx))

	// go.mod: require lines stay untouched; a same-path replace pinned at the original
	// version is added only for the patched module.
	goModContent, err := os.ReadFile(goModPath)
	require.NoError(t, err)
	got := string(goModContent)
	assert.True(t, containsLine(got, "replace github.com/google/uuid v1.3.0 => github.com/google/uuid v1.3.0-root.io.1"),
		"expected a same-path replace directive pinned to the original version")
	assert.False(t, strings.Contains(got, "github.com/pkg/errors =>"), "skipped module must not get a replace directive")
	assert.True(t, strings.Contains(got, "github.com/google/uuid v1.3.0"), "original require must remain")

	// go.sum must be left untouched by our own code.
	sumContent, err := os.ReadFile(goSumPath)
	require.NoError(t, err)
	assert.Equal(t, goSumContent, string(sumContent), "go.sum must not be modified directly by the tool")

	// Only `go mod tidy` should run — no `go get` for either module.
	require.Len(t, cmdRunner.Calls, 1, "expected only go mod tidy")
	assert.Equal(t, "go", cmdRunner.Calls[0].Name)
	assert.Equal(t, "mod tidy", strings.Join(cmdRunner.Calls[0].Args, " "))
	assert.Equal(t, dir, cmdRunner.Calls[0].Dir)
	assert.Contains(t, cmdRunner.Calls[0].Env, "GOPROXY=https://:test-key@pkg.root.io/gobinary,https://proxy.golang.org,direct")
	// GONOSUMDB must cover only the patched module; skipped module keeps normal checksum verification
	assert.Contains(t, cmdRunner.Calls[0].Env, "GONOSUMDB=github.com/google/uuid")
	assert.NotContains(t, cmdRunner.Calls[0].Env, "github.com/pkg/errors", "skipped module must not be in GONOSUMDB")
}

// TestGoLangApp_E2E_NonAliased_MultiplePatches verifies that when multiple modules are patched,
// same-path replace directives are added for both patched modules (each pinned to its own
// original version), go.sum stays untouched by the tool, and GONOSUMDB covers both module
// paths ahead of the single `go mod tidy` call.
func TestGoLangApp_E2E_NonAliased_MultiplePatches(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	dir := t.TempDir()
	goModPath := writeGoMod(t, dir, `module example.com/myapp

go 1.21

require (
	github.com/google/uuid v1.3.0
	github.com/pkg/errors v0.9.1
)
`)
	goSumPath := filepath.Join(dir, "go.sum")
	goSumContent := "github.com/google/uuid v1.3.0 h1:aaa=\n" +
		"github.com/google/uuid v1.3.0/go.mod h1:bbb=\n" +
		"github.com/pkg/errors v0.9.1 h1:ccc=\n" +
		"github.com/pkg/errors v0.9.1/go.mod h1:ddd=\n"
	require.NoError(t, os.WriteFile(goSumPath, []byte(goSumContent), 0644))

	cmdRunner := &MockCommandRunner{}

	app := NewApp(
		"test-key",
		"https://api.root.io",
		"https://pkg.root.io",
		goModPath,
		false,
		false, // useAlias=false
		nil,
		logger,
		NewGoModParser(logger),
		&MockAPIClient{
			AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, _ []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
				return &rootio.AnalyzePackagesResponse{
					Patches: []rootio.PackagePatch{
						{
							PackageName: "github.com/google/uuid",
							Version:     "v1.3.0",
							Patch:       rootio.PatchInfo{Name: "github.com/google/uuid", Version: "v1.3.0-root.io.1"},
							PatchAlias:  rootio.PatchInfo{Name: "pkg.root.io/golang/github.com/google/uuid", Version: "v1.3.0-rootio.1"},
						},
						{
							PackageName: "github.com/pkg/errors",
							Version:     "v0.9.1",
							Patch:       rootio.PatchInfo{Name: "github.com/pkg/errors", Version: "v0.9.1-root.io.1"},
							PatchAlias:  rootio.PatchInfo{Name: "pkg.root.io/golang/github.com/pkg/errors", Version: "v0.9.1-rootio.1"},
						},
					},
				}, nil
			},
		},
		cmdRunner,
	)

	require.NoError(t, app.Run(ctx))

	// go.mod: both modules get a same-path replace pinned at their original version.
	goModContent, err := os.ReadFile(goModPath)
	require.NoError(t, err)
	got := string(goModContent)
	assert.True(t, containsLine(got, "replace github.com/google/uuid v1.3.0 => github.com/google/uuid v1.3.0-root.io.1"))
	assert.True(t, containsLine(got, "replace github.com/pkg/errors v0.9.1 => github.com/pkg/errors v0.9.1-root.io.1"))

	// go.sum must be left untouched by our own code.
	sumContent, err := os.ReadFile(goSumPath)
	require.NoError(t, err)
	assert.Equal(t, goSumContent, string(sumContent), "go.sum must not be modified directly by the tool")

	// Only `go mod tidy` should run — no `go get` calls.
	require.Len(t, cmdRunner.Calls, 1, "expected only go mod tidy")
	assert.Equal(t, "mod tidy", strings.Join(cmdRunner.Calls[0].Args, " "))

	// GONOSUMDB must contain both patched module paths
	assert.Contains(t, cmdRunner.Calls[0].Env, "GONOSUMDB=github.com/google/uuid,github.com/pkg/errors")
}

// TestGoLangApp_E2E_NonAliased_WithVendor verifies that go mod vendor is also called in
// non-aliased mode when a vendor/modules.txt file exists, after the same-path replace
// directive is written and go mod tidy runs — no `go get` step in between.
func TestGoLangApp_E2E_NonAliased_WithVendor(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	dir := t.TempDir()
	goModPath := writeGoMod(t, dir, `module example.com/myapp

go 1.21

require github.com/google/uuid v1.3.0
`)
	goSumPath := filepath.Join(dir, "go.sum")
	goSumContent := "github.com/google/uuid v1.3.0 h1:aaa=\n" +
		"github.com/google/uuid v1.3.0/go.mod h1:bbb=\n"
	require.NoError(t, os.WriteFile(goSumPath, []byte(goSumContent), 0644))

	vendorDir := filepath.Join(dir, "vendor")
	require.NoError(t, os.MkdirAll(vendorDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(vendorDir, "modules.txt"), []byte("# vendor modules\n"), 0644))

	cmdRunner := &MockCommandRunner{}

	app := NewApp(
		"test-key",
		"https://api.root.io",
		"https://pkg.root.io",
		goModPath,
		false,
		false, // useAlias=false
		nil,
		logger,
		NewGoModParser(logger),
		&MockAPIClient{
			AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, _ []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
				return &rootio.AnalyzePackagesResponse{
					Patches: []rootio.PackagePatch{
						{
							PackageName: "github.com/google/uuid",
							Version:     "v1.3.0",
							Patch:       rootio.PatchInfo{Name: "github.com/google/uuid", Version: "v1.3.0-root.io.1"},
							PatchAlias:  rootio.PatchInfo{Name: "pkg.root.io/golang/github.com/google/uuid", Version: "v1.3.0-rootio.1"},
						},
					},
				}, nil
			},
		},
		cmdRunner,
	)

	require.NoError(t, app.Run(ctx))

	// go.mod: same-path replace pinned at the original version.
	goModContent, err := os.ReadFile(goModPath)
	require.NoError(t, err)
	assert.True(t, containsLine(string(goModContent), "replace github.com/google/uuid v1.3.0 => github.com/google/uuid v1.3.0-root.io.1"))

	// go.sum must be left untouched by our own code.
	sumContent, err := os.ReadFile(goSumPath)
	require.NoError(t, err)
	assert.Equal(t, goSumContent, string(sumContent), "go.sum must not be modified directly by the tool")

	// Commands: go mod tidy, go mod vendor — no go get.
	require.Len(t, cmdRunner.Calls, 2, "expected go mod tidy, go mod vendor")
	assert.Equal(t, "mod tidy", strings.Join(cmdRunner.Calls[0].Args, " "))
	assert.Equal(t, "mod vendor", strings.Join(cmdRunner.Calls[1].Args, " "))
	assert.Equal(t, dir, cmdRunner.Calls[1].Dir)
}

// TestGoLangApp_E2E_WithVendor verifies that go mod vendor is also called
// when a vendor/modules.txt file exists.
func TestGoLangApp_E2E_WithVendor(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	dir := t.TempDir()
	goModPath := writeGoMod(t, dir, `module example.com/myapp

go 1.21

require github.com/google/uuid v1.3.0
`)

	vendorDir := filepath.Join(dir, "vendor")
	require.NoError(t, os.MkdirAll(vendorDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(vendorDir, "modules.txt"), []byte("# vendor modules\n"), 0644))

	cmdRunner := &MockCommandRunner{}

	app := NewApp(
		"test-key",
		"https://api.root.io",
		"https://pkg.root.io",
		goModPath,
		false,
		true,
		nil,
		logger,
		NewGoModParser(logger), // real parser
		&MockAPIClient{
			AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, _ []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
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

	require.NoError(t, app.Run(ctx))

	content, err := os.ReadFile(goModPath)
	require.NoError(t, err)
	assert.True(t, containsLine(string(content), "replace github.com/google/uuid v1.3.0 => pkg.root.io/golang/github.com/google/uuid v1.3.0-rootio.1"))

	require.Len(t, cmdRunner.Calls, 2, "expected go mod tidy and go mod vendor")
	assert.Equal(t, "mod tidy", strings.Join(cmdRunner.Calls[0].Args, " "))
	assert.Equal(t, "mod vendor", strings.Join(cmdRunner.Calls[1].Args, " "))
	assert.Equal(t, dir, cmdRunner.Calls[1].Dir)
}
