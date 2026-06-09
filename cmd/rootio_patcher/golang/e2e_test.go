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

// TestGoLangApp_E2E_NonAliased verifies the non-aliased flow: go.sum entries for patched
// modules are removed, go get is called for each, then go mod tidy — no replace directives.
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
	require.NoError(t, os.WriteFile(goSumPath, []byte(
		"github.com/google/uuid v1.3.0 h1:t6JiXb...=\n"+
			"github.com/google/uuid v1.3.0/go.mod h1:TIyP...=\n"+
			"github.com/pkg/errors v0.9.1 h1:abc=\n"+
			"github.com/pkg/errors v0.9.1/go.mod h1:def=\n",
	), 0644))

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
							Patch:       rootio.PatchInfo{Name: "github.com/google/uuid", Version: "v1.3.0"},
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

	// go.mod must not be modified — no replace directives
	goModContent, err := os.ReadFile(goModPath)
	require.NoError(t, err)
	assert.False(t, strings.Contains(string(goModContent), "replace"), "no replace directives must be added in non-aliased mode")
	assert.True(t, strings.Contains(string(goModContent), "github.com/google/uuid v1.3.0"), "original require must remain")

	// go.sum: uuid entries removed; skipped pkg/errors entries preserved
	sumContent, err := os.ReadFile(goSumPath)
	require.NoError(t, err)
	assert.False(t, strings.Contains(string(sumContent), "github.com/google/uuid"), "go.sum entries for patched module must be removed")
	assert.True(t, strings.Contains(string(sumContent), "github.com/pkg/errors"), "go.sum entries for skipped module must be preserved")

	// Commands: go get uuid@v1.3.0, then go mod tidy — no go get for the skipped module
	require.Len(t, cmdRunner.Calls, 2, "expected go get then go mod tidy")
	assert.Equal(t, "go", cmdRunner.Calls[0].Name)
	assert.Equal(t, []string{"get", "github.com/google/uuid@v1.3.0"}, cmdRunner.Calls[0].Args)
	assert.Equal(t, dir, cmdRunner.Calls[0].Dir)
	assert.Contains(t, cmdRunner.Calls[0].Env, "GOPROXY=https://:test-key@pkg.root.io/gobinary,https://proxy.golang.org,direct")
	// GONOSUMDB must cover only the patched module; skipped module keeps normal checksum verification
	assert.Contains(t, cmdRunner.Calls[0].Env, "GONOSUMDB=github.com/google/uuid")
	assert.NotContains(t, cmdRunner.Calls[0].Env, "github.com/pkg/errors", "skipped module must not be in GONOSUMDB")

	assert.Equal(t, "go", cmdRunner.Calls[1].Name)
	assert.Equal(t, "mod tidy", strings.Join(cmdRunner.Calls[1].Args, " "))
	assert.Equal(t, dir, cmdRunner.Calls[1].Dir)
}

// TestGoLangApp_E2E_NonAliased_MultiplePatches verifies that when multiple modules are patched,
// all their go.sum entries are removed, go get is called for each, and GONOSUMDB covers all of them.
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
	require.NoError(t, os.WriteFile(goSumPath, []byte(
		"github.com/google/uuid v1.3.0 h1:aaa=\n"+
			"github.com/google/uuid v1.3.0/go.mod h1:bbb=\n"+
			"github.com/pkg/errors v0.9.1 h1:ccc=\n"+
			"github.com/pkg/errors v0.9.1/go.mod h1:ddd=\n",
	), 0644))

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
							Patch:       rootio.PatchInfo{Name: "github.com/google/uuid", Version: "v1.3.0"},
							PatchAlias:  rootio.PatchInfo{Name: "pkg.root.io/golang/github.com/google/uuid", Version: "v1.3.0-rootio.1"},
						},
						{
							PackageName: "github.com/pkg/errors",
							Version:     "v0.9.1",
							Patch:       rootio.PatchInfo{Name: "github.com/pkg/errors", Version: "v0.9.1"},
							PatchAlias:  rootio.PatchInfo{Name: "pkg.root.io/golang/github.com/pkg/errors", Version: "v0.9.1-rootio.1"},
						},
					},
				}, nil
			},
		},
		cmdRunner,
	)

	require.NoError(t, app.Run(ctx))

	// go.sum: both patched modules' entries removed
	sumContent, err := os.ReadFile(goSumPath)
	require.NoError(t, err)
	assert.False(t, strings.Contains(string(sumContent), "github.com/google/uuid"), "uuid entries must be removed")
	assert.False(t, strings.Contains(string(sumContent), "github.com/pkg/errors"), "pkg/errors entries must be removed")

	// Commands: go get for each module, then go mod tidy
	require.Len(t, cmdRunner.Calls, 3, "expected go get x2 then go mod tidy")
	assert.Equal(t, []string{"get", "github.com/google/uuid@v1.3.0"}, cmdRunner.Calls[0].Args)
	assert.Equal(t, []string{"get", "github.com/pkg/errors@v0.9.1"}, cmdRunner.Calls[1].Args)
	assert.Equal(t, "mod tidy", strings.Join(cmdRunner.Calls[2].Args, " "))

	// GONOSUMDB must contain both patched module paths
	assert.Contains(t, cmdRunner.Calls[0].Env, "GONOSUMDB=github.com/google/uuid,github.com/pkg/errors")
}

// TestGoLangApp_E2E_NonAliased_WithVendor verifies that go mod vendor is also called in
// non-aliased mode when a vendor/modules.txt file exists.
func TestGoLangApp_E2E_NonAliased_WithVendor(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	dir := t.TempDir()
	goModPath := writeGoMod(t, dir, `module example.com/myapp

go 1.21

require github.com/google/uuid v1.3.0
`)
	goSumPath := filepath.Join(dir, "go.sum")
	require.NoError(t, os.WriteFile(goSumPath, []byte(
		"github.com/google/uuid v1.3.0 h1:aaa=\n"+
			"github.com/google/uuid v1.3.0/go.mod h1:bbb=\n",
	), 0644))

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

	// go.sum: uuid entries removed
	sumContent, err := os.ReadFile(goSumPath)
	require.NoError(t, err)
	assert.False(t, strings.Contains(string(sumContent), "github.com/google/uuid"), "go.sum entries for patched module must be removed")

	// Commands: go get, go mod tidy, go mod vendor
	require.Len(t, cmdRunner.Calls, 3, "expected go get, go mod tidy, go mod vendor")
	assert.Equal(t, []string{"get", "github.com/google/uuid@v1.3.0"}, cmdRunner.Calls[0].Args)
	assert.Equal(t, "mod tidy", strings.Join(cmdRunner.Calls[1].Args, " "))
	assert.Equal(t, "mod vendor", strings.Join(cmdRunner.Calls[2].Args, " "))
	assert.Equal(t, dir, cmdRunner.Calls[2].Dir)
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
