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
		logger,
		NewGoModParser(logger), // real parser
		&MockAPIClient{
			AnalyzePackagesFunc: func(_ context.Context, packages []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error) {
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
		logger,
		NewGoModParser(logger),
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
		logger,
		NewGoModParser(logger),
		&MockAPIClient{
			AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
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

	require.NoError(t, app.Run(ctx))

	content, err := os.ReadFile(goModPath)
	require.NoError(t, err)
	assert.True(t, containsLine(string(content), "replace github.com/google/uuid v1.3.0 => pkg.root.io/golang/github.com/google/uuid v1.3.0-rootio.1"))

	require.Len(t, cmdRunner.Calls, 2, "expected go mod tidy and go mod vendor")
	assert.Equal(t, "mod tidy", strings.Join(cmdRunner.Calls[0].Args, " "))
	assert.Equal(t, "mod vendor", strings.Join(cmdRunner.Calls[1].Args, " "))
	assert.Equal(t, dir, cmdRunner.Calls[1].Dir)
}
