package golang

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func writeGoMod(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}
	return path
}

// --- Parse tests ---

func TestGoModParser_Parse_BasicRequires(t *testing.T) {
	ctx := context.Background()
	p := NewGoModParser(newTestLogger())
	dir := t.TempDir()
	path := writeGoMod(t, dir, `module example.com/app

go 1.21

require (
	github.com/google/uuid v1.3.0
	github.com/pkg/errors v0.9.1 // indirect
)

require github.com/stretchr/testify v1.8.4
`)

	pkgs, err := p.Parse(ctx, path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pkgs) != 3 {
		t.Fatalf("expected 3 packages, got %d: %v", len(pkgs), pkgs)
	}

	byName := make(map[string]any)
	for _, pkg := range pkgs {
		byName[pkg.Name] = struct{}{}
	}
	for _, name := range []string{"github.com/google/uuid", "github.com/pkg/errors", "github.com/stretchr/testify"} {
		if _, ok := byName[name]; !ok {
			t.Errorf("expected package %q in result", name)
		}
	}

	// Check direct/indirect flag
	for _, pkg := range pkgs {
		if pkg.Name == "github.com/pkg/errors" && pkg.Direct {
			t.Errorf("expected github.com/pkg/errors to be indirect")
		}
		if pkg.Name == "github.com/google/uuid" && !pkg.Direct {
			t.Errorf("expected github.com/google/uuid to be direct")
		}
	}
}

func TestGoModParser_Parse_SkipsPseudoVersions(t *testing.T) {
	ctx := context.Background()
	p := NewGoModParser(newTestLogger())
	dir := t.TempDir()
	path := writeGoMod(t, dir, `module example.com/app

go 1.21

require (
	github.com/google/uuid v1.3.0
	golang.org/x/sys v0.0.0-20230101000000-abcdef123456 // indirect
)
`)

	pkgs, err := p.Parse(ctx, path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package (pseudo-version skipped), got %d: %v", len(pkgs), pkgs)
	}
	if pkgs[0].Name != "github.com/google/uuid" {
		t.Errorf("expected github.com/google/uuid, got %q", pkgs[0].Name)
	}
}

func TestGoModParser_Parse_SkipsNonSemver(t *testing.T) {
	ctx := context.Background()
	p := NewGoModParser(newTestLogger())
	dir := t.TempDir()
	path := writeGoMod(t, dir, `module example.com/app

go 1.21

require github.com/some/module latest
`)

	pkgs, err := p.Parse(ctx, path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pkgs) != 0 {
		t.Errorf("expected 0 packages (non-semver skipped), got %d", len(pkgs))
	}
}

func TestGoModParser_Parse_HandlesUppercaseModules(t *testing.T) {
	ctx := context.Background()
	p := NewGoModParser(newTestLogger())
	dir := t.TempDir()
	path := writeGoMod(t, dir, `module example.com/app

go 1.21

require (
	github.com/Azure/azure-sdk-for-go v68.0.0+incompatible
	github.com/BurntSushi/toml v1.3.2
)
`)

	pkgs, err := p.Parse(ctx, path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d: %v", len(pkgs), pkgs)
	}
	byName := make(map[string]string)
	for _, pkg := range pkgs {
		byName[pkg.Name] = pkg.Version
	}
	if v := byName["github.com/Azure/azure-sdk-for-go"]; v != "v68.0.0+incompatible" {
		t.Errorf("expected v68.0.0+incompatible for Azure SDK, got %q", v)
	}
	if v := byName["github.com/BurntSushi/toml"]; v != "v1.3.2" {
		t.Errorf("expected v1.3.2 for BurntSushi/toml, got %q", v)
	}
}

func TestGoModParser_Parse_IgnoresNonRequireBlocks(t *testing.T) {
	ctx := context.Background()
	p := NewGoModParser(newTestLogger())
	dir := t.TempDir()
	path := writeGoMod(t, dir, `module example.com/app

go 1.21

require github.com/google/uuid v1.3.0

exclude github.com/bad/module v1.0.0

replace (
	github.com/foo/bar v1.0.0 => github.com/foo/bar-patched v1.0.1
)
`)

	pkgs, err := p.Parse(ctx, path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package (exclude and replace not treated as requires), got %d: %v", len(pkgs), pkgs)
	}
	if pkgs[0].Name != "github.com/google/uuid" {
		t.Errorf("expected github.com/google/uuid, got %q", pkgs[0].Name)
	}
}

// --- Patch tests ---

func TestGoModParser_Patch_AddsReplaceDirectives(t *testing.T) {
	ctx := context.Background()
	p := NewGoModParser(newTestLogger())
	dir := t.TempDir()
	path := writeGoMod(t, dir, `module example.com/app

go 1.21

require github.com/google/uuid v1.3.0
`)

	updates := []GoModUpdate{
		{
			Module:         "github.com/google/uuid",
			CurrentVersion: "v1.3.0",
			AliasName:      "pkg.root.io/golang/github.com/google/uuid",
			AliasVersion:   "v1.3.0-rootio.1",
		},
	}

	result, err := p.Patch(ctx, path, updates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "replace github.com/google/uuid v1.3.0 => pkg.root.io/golang/github.com/google/uuid v1.3.0-rootio.1"
	if !containsLine(result, expected) {
		t.Errorf("expected result to contain line:\n  %q\ngot:\n%s", expected, result)
	}
}

func TestGoModParser_Patch_OverwritesExistingStandaloneReplace(t *testing.T) {
	ctx := context.Background()
	p := NewGoModParser(newTestLogger())
	dir := t.TempDir()
	path := writeGoMod(t, dir, `module example.com/app

go 1.21

require github.com/google/uuid v1.3.0

replace github.com/google/uuid v1.3.0 => old/replacement v1.3.0-old
`)

	updates := []GoModUpdate{
		{
			Module:         "github.com/google/uuid",
			CurrentVersion: "v1.3.0",
			AliasName:      "pkg.root.io/golang/github.com/google/uuid",
			AliasVersion:   "v1.3.0-rootio.1",
		},
	}

	result, err := p.Patch(ctx, path, updates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	newDirective := "replace github.com/google/uuid v1.3.0 => pkg.root.io/golang/github.com/google/uuid v1.3.0-rootio.1"
	oldDirective := "replace github.com/google/uuid v1.3.0 => old/replacement v1.3.0-old"

	if !containsLine(result, newDirective) {
		t.Errorf("expected updated replace directive:\n  %q\ngot:\n%s", newDirective, result)
	}
	if containsLine(result, oldDirective) {
		t.Errorf("old replace directive should have been replaced:\n%s", result)
	}
}

func TestGoModParser_Patch_OverwritesExistingBlockReplace(t *testing.T) {
	ctx := context.Background()
	p := NewGoModParser(newTestLogger())
	dir := t.TempDir()
	path := writeGoMod(t, dir, `module example.com/app

go 1.21

require github.com/google/uuid v1.3.0

replace (
	github.com/google/uuid v1.3.0 => old/replacement v1.3.0-old
)
`)

	updates := []GoModUpdate{
		{
			Module:         "github.com/google/uuid",
			CurrentVersion: "v1.3.0",
			AliasName:      "pkg.root.io/golang/github.com/google/uuid",
			AliasVersion:   "v1.3.0-rootio.1",
		},
	}

	result, err := p.Patch(ctx, path, updates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !containsLine(result, "\tgithub.com/google/uuid v1.3.0 => pkg.root.io/golang/github.com/google/uuid v1.3.0-rootio.1") {
		t.Errorf("expected updated replace in block, got:\n%s", result)
	}
	if containsLine(result, "old/replacement") {
		t.Errorf("old replace directive should have been replaced, got:\n%s", result)
	}
}

func TestGoModParser_Patch_PreservesUnrelatedReplaces(t *testing.T) {
	ctx := context.Background()
	p := NewGoModParser(newTestLogger())
	dir := t.TempDir()
	path := writeGoMod(t, dir, `module example.com/app

go 1.21

require (
	github.com/google/uuid v1.3.0
	github.com/pkg/errors v0.9.1
)

replace github.com/pkg/errors v0.9.1 => github.com/pkg/errors-fork v0.9.1
`)

	updates := []GoModUpdate{
		{
			Module:         "github.com/google/uuid",
			CurrentVersion: "v1.3.0",
			AliasName:      "pkg.root.io/golang/github.com/google/uuid",
			AliasVersion:   "v1.3.0-rootio.1",
		},
	}

	result, err := p.Patch(ctx, path, updates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both directives must be present
	newDirective := "replace github.com/google/uuid v1.3.0 => pkg.root.io/golang/github.com/google/uuid v1.3.0-rootio.1"
	existing := "replace github.com/pkg/errors v0.9.1 => github.com/pkg/errors-fork v0.9.1"

	if !containsLine(result, newDirective) {
		t.Errorf("expected new replace directive:\n  %q\ngot:\n%s", newDirective, result)
	}
	if !containsLine(result, existing) {
		t.Errorf("expected unrelated replace directive to be preserved:\n  %q\ngot:\n%s", existing, result)
	}
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
