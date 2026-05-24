package composer

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestParser() *ComposerParser {
	return NewParser(slog.New(slog.NewTextHandler(os.Stdout, nil)))
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0644))
	}
}

func TestComposerParser_Parse_RequireAndRequireDev(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"composer.json": `{"require":{"vendor/pkg":"^2.1.0"},"require-dev":{"vendor/test":"^1.0.0"}}`,
		"composer.lock": `{
			"packages": [{"name":"vendor/pkg","version":"2.1.3"}],
			"packages-dev": [{"name":"vendor/test","version":"1.0.5"}]
		}`,
	})

	p := newTestParser()
	pkgs, err := p.Parse(context.Background(), filepath.Join(dir, "composer.json"))
	require.NoError(t, err)
	require.Len(t, pkgs, 2)

	byName := map[string]string{}
	for _, pkg := range pkgs {
		byName[pkg.Name] = pkg.Version
	}
	assert.Equal(t, "2.1.3", byName["vendor/pkg"])
	assert.Equal(t, "1.0.5", byName["vendor/test"])
}

func TestComposerParser_Parse_SkipsPlatformRequirements(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"composer.json": `{}`,
		"composer.lock": `{
			"packages": [
				{"name":"vendor/pkg","version":"2.1.3"},
				{"name":"php","version":"8.1.0"},
				{"name":"ext-json","version":"*"},
				{"name":"lib-openssl","version":"1.1.1"}
			],
			"packages-dev": []
		}`,
	})

	p := newTestParser()
	pkgs, err := p.Parse(context.Background(), filepath.Join(dir, "composer.json"))
	require.NoError(t, err)
	require.Len(t, pkgs, 1)
	assert.Equal(t, "vendor/pkg", pkgs[0].Name)
}

func TestComposerParser_Parse_ErrorWhenLockMissing(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"composer.json": `{"require":{"vendor/pkg":"^2.1.0"}}`,
	})

	p := newTestParser()
	_, err := p.Parse(context.Background(), filepath.Join(dir, "composer.json"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "composer.lock not found")
}

func TestComposerParser_Update_DirectRequire(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"composer.json": `{"require":{"vendor/pkg":"^2.1.0","other/lib":"^1.0"}}`,
	})

	p := newTestParser()
	updated, err := p.Update(context.Background(), filepath.Join(dir, "composer.json"), map[string]string{
		"vendor/pkg": "vendor/pkg:2.1.4",
	})
	require.NoError(t, err)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(updated), &result))
	req := result["require"].(map[string]interface{})
	assert.Equal(t, "2.1.4", req["vendor/pkg"])
	assert.Equal(t, "^1.0", req["other/lib"]) // untouched
}

func TestComposerParser_Update_RequireDev(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"composer.json": `{"require":{},"require-dev":{"vendor/test":"^1.0.0"}}`,
	})

	p := newTestParser()
	updated, err := p.Update(context.Background(), filepath.Join(dir, "composer.json"), map[string]string{
		"vendor/test": "vendor/test:1.0.5",
	})
	require.NoError(t, err)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(updated), &result))
	requireDev := result["require-dev"].(map[string]interface{})
	assert.Equal(t, "1.0.5", requireDev["vendor/test"])
}

func TestComposerParser_Update_TransitiveDep(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"composer.json": `{"require":{"vendor/direct":"^1.0"}}`,
	})

	p := newTestParser()
	updated, err := p.Update(context.Background(), filepath.Join(dir, "composer.json"), map[string]string{
		"vendor/transitive": "vendor/transitive:3.2.1",
	})
	require.NoError(t, err)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(updated), &result))
	req := result["require"].(map[string]interface{})
	assert.Equal(t, "3.2.1", req["vendor/transitive"])
	assert.Equal(t, "^1.0", req["vendor/direct"]) // untouched
}

func TestComposerParser_Update_AliasedPackage(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"composer.json": `{"require":{"vendor/pkg":"^2.1.0"}}`,
	})

	p := newTestParser()
	updated, err := p.Update(context.Background(), filepath.Join(dir, "composer.json"), map[string]string{
		"vendor/pkg": "rootio/vendor-pkg:2.1.4-rootio",
	})
	require.NoError(t, err)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(updated), &result))
	req := result["require"].(map[string]interface{})
	assert.Equal(t, "2.1.4-rootio", req["rootio/vendor-pkg"])
	_, originalStillPresent := req["vendor/pkg"]
	assert.False(t, originalStillPresent)
}

func TestComposerParser_Update_InjectsRepository(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"composer.json": `{"require":{"vendor/pkg":"^2.1.0"}}`,
	})

	p := newTestParser()
	updated, err := p.Update(context.Background(), filepath.Join(dir, "composer.json"), map[string]string{
		"vendor/pkg": "vendor/pkg:2.1.4",
	})
	require.NoError(t, err)
	assert.Contains(t, updated, "pkg.root.io")
}

func TestComposerParser_Update_DoesNotDuplicateRepository(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"composer.json": `{"require":{"vendor/pkg":"^2.1.0"},"repositories":[{"type":"composer","url":"https://pkg.root.io/composer"}]}`,
	})

	p := newTestParser()
	updated, err := p.Update(context.Background(), filepath.Join(dir, "composer.json"), map[string]string{
		"vendor/pkg": "vendor/pkg:2.1.4",
	})
	require.NoError(t, err)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(updated), &result))
	repos := result["repositories"].([]interface{})
	count := 0
	for _, r := range repos {
		repo := r.(map[string]interface{})
		if repo["url"] == "https://pkg.root.io/composer" {
			count++
		}
	}
	assert.Equal(t, 1, count)
}

func TestComposerParser_Validate(t *testing.T) {
	p := newTestParser()
	assert.True(t, p.Validate(`{"require":{}}`))
	assert.False(t, p.Validate(`{invalid json`))
}
