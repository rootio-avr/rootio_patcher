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
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func writeGoMod(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "go.mod")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

// --- Parse tests ---

func TestGoModParser_Parse_BasicRequires(t *testing.T) {
	ctx := context.Background()
	p := NewGoModParser(newTestLogger())
	path := writeGoMod(t, t.TempDir(), `module example.com/app

go 1.21

require (
	github.com/google/uuid v1.3.0
	github.com/pkg/errors v0.9.1 // indirect
)

require github.com/stretchr/testify v1.8.4
`)

	pkgs, err := p.Parse(ctx, path)
	require.NoError(t, err)
	require.Len(t, pkgs, 3)

	byName := make(map[string]bool)
	for _, pkg := range pkgs {
		byName[pkg.Name] = pkg.Direct
	}
	assert.Contains(t, byName, "github.com/google/uuid")
	assert.Contains(t, byName, "github.com/pkg/errors")
	assert.Contains(t, byName, "github.com/stretchr/testify")

	assert.True(t, byName["github.com/google/uuid"], "uuid should be direct")
	assert.False(t, byName["github.com/pkg/errors"], "pkg/errors should be indirect")
}

func TestGoModParser_Parse_SkipsPseudoVersions(t *testing.T) {
	ctx := context.Background()
	p := NewGoModParser(newTestLogger())
	path := writeGoMod(t, t.TempDir(), `module example.com/app

go 1.21

require (
	github.com/google/uuid v1.3.0
	golang.org/x/sys v0.0.0-20230101000000-abcdef123456 // indirect
)
`)

	pkgs, err := p.Parse(ctx, path)
	require.NoError(t, err)
	require.Len(t, pkgs, 1, "pseudo-version should be skipped")
	assert.Equal(t, "github.com/google/uuid", pkgs[0].Name)
}

func TestGoModParser_Parse_SkipsNonSemver(t *testing.T) {
	ctx := context.Background()
	p := NewGoModParser(newTestLogger())
	path := writeGoMod(t, t.TempDir(), `module example.com/app

go 1.21

require github.com/some/module latest
`)

	pkgs, err := p.Parse(ctx, path)
	require.NoError(t, err)
	assert.Empty(t, pkgs, "non-semver version should be skipped")
}

func TestGoModParser_Parse_HandlesUppercaseModules(t *testing.T) {
	ctx := context.Background()
	p := NewGoModParser(newTestLogger())
	path := writeGoMod(t, t.TempDir(), `module example.com/app

go 1.21

require (
	github.com/Azure/azure-sdk-for-go v68.0.0+incompatible
	github.com/BurntSushi/toml v1.3.2
)
`)

	pkgs, err := p.Parse(ctx, path)
	require.NoError(t, err)
	require.Len(t, pkgs, 2)

	byName := make(map[string]string)
	for _, pkg := range pkgs {
		byName[pkg.Name] = pkg.Version
	}
	assert.Equal(t, "v68.0.0+incompatible", byName["github.com/Azure/azure-sdk-for-go"])
	assert.Equal(t, "v1.3.2", byName["github.com/BurntSushi/toml"])
}

func TestGoModParser_Parse_IgnoresNonRequireBlocks(t *testing.T) {
	ctx := context.Background()
	p := NewGoModParser(newTestLogger())
	path := writeGoMod(t, t.TempDir(), `module example.com/app

go 1.21

require github.com/google/uuid v1.3.0

exclude github.com/bad/module v1.0.0

replace (
	github.com/foo/bar v1.0.0 => github.com/foo/bar-patched v1.0.1
)
`)

	pkgs, err := p.Parse(ctx, path)
	require.NoError(t, err)
	require.Len(t, pkgs, 1, "exclude and replace blocks should not be parsed as packages")
	assert.Equal(t, "github.com/google/uuid", pkgs[0].Name)
}

// --- Patch tests ---

func TestGoModParser_Patch_AddsReplaceDirectives(t *testing.T) {
	ctx := context.Background()
	p := NewGoModParser(newTestLogger())
	path := writeGoMod(t, t.TempDir(), `module example.com/app

go 1.21

require github.com/google/uuid v1.3.0
`)

	result, err := p.Patch(ctx, path, []GoModUpdate{
		{
			Module:         "github.com/google/uuid",
			CurrentVersion: "v1.3.0",
			AliasName:      "pkg.root.io/golang/github.com/google/uuid",
			AliasVersion:   "v1.3.0-rootio.1",
		},
	})
	require.NoError(t, err)
	assert.True(t, containsLine(result, "replace github.com/google/uuid v1.3.0 => pkg.root.io/golang/github.com/google/uuid v1.3.0-rootio.1"))
}

func TestGoModParser_Patch_OverwritesExistingStandaloneReplace(t *testing.T) {
	ctx := context.Background()
	p := NewGoModParser(newTestLogger())
	path := writeGoMod(t, t.TempDir(), `module example.com/app

go 1.21

require github.com/google/uuid v1.3.0

replace github.com/google/uuid v1.3.0 => old/replacement v1.3.0-old
`)

	result, err := p.Patch(ctx, path, []GoModUpdate{
		{
			Module:         "github.com/google/uuid",
			CurrentVersion: "v1.3.0",
			AliasName:      "pkg.root.io/golang/github.com/google/uuid",
			AliasVersion:   "v1.3.0-rootio.1",
		},
	})
	require.NoError(t, err)
	assert.True(t, containsLine(result, "replace github.com/google/uuid v1.3.0 => pkg.root.io/golang/github.com/google/uuid v1.3.0-rootio.1"))
	assert.False(t, containsLine(result, "replace github.com/google/uuid v1.3.0 => old/replacement v1.3.0-old"))
}

func TestGoModParser_Patch_OverwritesExistingBlockReplace(t *testing.T) {
	ctx := context.Background()
	p := NewGoModParser(newTestLogger())
	path := writeGoMod(t, t.TempDir(), `module example.com/app

go 1.21

require github.com/google/uuid v1.3.0

replace (
	github.com/google/uuid v1.3.0 => old/replacement v1.3.0-old
)
`)

	result, err := p.Patch(ctx, path, []GoModUpdate{
		{
			Module:         "github.com/google/uuid",
			CurrentVersion: "v1.3.0",
			AliasName:      "pkg.root.io/golang/github.com/google/uuid",
			AliasVersion:   "v1.3.0-rootio.1",
		},
	})
	require.NoError(t, err)
	assert.True(t, containsLine(result, "\tgithub.com/google/uuid v1.3.0 => pkg.root.io/golang/github.com/google/uuid v1.3.0-rootio.1"))
	assert.False(t, strings.Contains(result, "old/replacement"))
}

func TestGoModParser_Patch_PreservesUnrelatedReplaces(t *testing.T) {
	ctx := context.Background()
	p := NewGoModParser(newTestLogger())
	path := writeGoMod(t, t.TempDir(), `module example.com/app

go 1.21

require (
	github.com/google/uuid v1.3.0
	github.com/pkg/errors v0.9.1
)

replace github.com/pkg/errors v0.9.1 => github.com/pkg/errors-fork v0.9.1
`)

	result, err := p.Patch(ctx, path, []GoModUpdate{
		{
			Module:         "github.com/google/uuid",
			CurrentVersion: "v1.3.0",
			AliasName:      "pkg.root.io/golang/github.com/google/uuid",
			AliasVersion:   "v1.3.0-rootio.1",
		},
	})
	require.NoError(t, err)
	assert.True(t, containsLine(result, "replace github.com/google/uuid v1.3.0 => pkg.root.io/golang/github.com/google/uuid v1.3.0-rootio.1"))
	assert.True(t, containsLine(result, "replace github.com/pkg/errors v0.9.1 => github.com/pkg/errors-fork v0.9.1"))
}

// containsLine reports whether s contains a line equal to want.
func containsLine(s, want string) bool {
	for _, line := range strings.Split(s, "\n") {
		if line == want {
			return true
		}
	}
	return false
}
