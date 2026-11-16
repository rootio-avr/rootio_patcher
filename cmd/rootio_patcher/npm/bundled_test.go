package npm

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/pkg/rootio"
)

// writePkg creates <dir>/package.json declaring name@version, plus an index.js holding body.
func writePkg(t *testing.T, dir, name, version, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	pj, _ := json.Marshal(map[string]string{"name": name, "version": version})
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), pj, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.js"), []byte(body), 0o644))
}

// npmTarball builds an npm-style tarball (entries under package/).
func npmTarball(t *testing.T, name, version, body string, extra map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	pj, _ := json.Marshal(map[string]string{"name": name, "version": version})
	files := map[string]string{"package/package.json": string(pj), "package/index.js": body}
	for k, v := range extra {
		files[k] = v
	}
	for path, content := range files {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: path, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}))
		_, err := tw.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func sri(data []byte) string {
	sum := sha512.Sum512(data)
	return "sha512-" + base64.StdEncoding.EncodeToString(sum[:])
}

// registry serves packuments and tarballs for name -> version -> tarball; integrity can be overridden.
type registry struct {
	tarballs  map[string]map[string][]byte
	integrity map[string]string // "name@version" -> SRI to advertise instead of the real one
	requests  []string
}

func (r *registry) server(t *testing.T) *httptest.Server {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		user, pass, ok := req.BasicAuth()
		if !ok || user != "root" || pass != "key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		r.requests = append(r.requests, req.URL.EscapedPath())
		if strings.HasPrefix(req.URL.Path, "/tgz/") {
			rest := strings.TrimPrefix(req.URL.Path, "/tgz/")
			i := strings.LastIndex(rest, "/")
			_, _ = w.Write(r.tarballs[rest[:i]][rest[i+1:]])
			return
		}
		name := strings.Replace(strings.TrimPrefix(req.URL.EscapedPath(), "/npm/"), "%2f", "/", 1)
		versions, ok := r.tarballs[name]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		doc := map[string]any{"versions": map[string]any{}}
		for v, data := range versions {
			integrity := sri(data)
			if over, ok := r.integrity[name+"@"+v]; ok {
				integrity = over
			}
			doc["versions"].(map[string]any)[v] = map[string]any{"dist": map[string]string{
				"tarball": srv.URL + "/tgz/" + name + "/" + v, "integrity": integrity}}
		}
		_ = json.NewEncoder(w).Encode(doc)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newTestBundledApp(t *testing.T, trees []string, srvURL string, dryRun bool, api *MockAPIClient) *BundledApp {
	t.Helper()
	return &BundledApp{
		trees: trees, pkgURL: srvURL, apiKey: "key", dryRun: dryRun, ignoreSet: map[string]struct{}{},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)), apiClient: api, httpClient: http.DefaultClient,
	}
}

func patchFor(name, from, to string) rootio.PackagePatch {
	return rootio.PackagePatch{PackageName: name, Version: from, Patch: rootio.PatchInfo{Name: name, Version: to}}
}

// npm's own tree, nested one level (npm/node_modules/x) and two levels deep (x/node_modules/y), plus a scoped package.
func npmTree(t *testing.T) string {
	root := filepath.Join(t.TempDir(), "lib", "node_modules")
	nm := filepath.Join(root, "npm", "node_modules")
	writePkg(t, filepath.Join(root, "npm"), "npm", "10.9.0", "npm")
	writePkg(t, filepath.Join(nm, "picomatch"), "picomatch", "4.0.4", "old picomatch")
	writePkg(t, filepath.Join(nm, "glob"), "glob", "10.4.5", "glob")
	require.NoError(t, os.WriteFile(filepath.Join(nm, "glob", "only-in-old.js"), []byte("stale"), 0o644))
	writePkg(t, filepath.Join(nm, "glob", "node_modules", "minimatch"), "minimatch", "9.0.0", "nested minimatch")
	writePkg(t, filepath.Join(nm, "@sigstore", "core"), "@sigstore/core", "1.0.0", "old core")
	return root
}

func TestBundled_RotatesEveryPackageWithARootBuild_KeepingNestedNodeModules(t *testing.T) {
	root := npmTree(t)
	reg := &registry{tarballs: map[string]map[string][]byte{
		"picomatch":      {"4.0.4-aikido.1": npmTarball(t, "picomatch", "4.0.4-aikido.1", "fixed picomatch", nil)},
		"glob":           {"10.4.5-aikido.2": npmTarball(t, "glob", "10.4.5-aikido.2", "fixed glob", nil)},
		"@sigstore/core": {"1.0.0-aikido.1": npmTarball(t, "@sigstore/core", "1.0.0-aikido.1", "fixed core", nil)},
	}}
	srv := reg.server(t)
	var analyzed []rootio.Package
	api := &MockAPIClient{AnalyzePackagesFunc: func(_ context.Context, pkgs, _ []rootio.Package, eco string) (*rootio.AnalyzePackagesResponse, error) {
		require.Equal(t, "npm", eco)
		analyzed = pkgs
		return &rootio.AnalyzePackagesResponse{Patches: []rootio.PackagePatch{
			patchFor("picomatch", "4.0.4", "4.0.4-aikido.1"),
			patchFor("glob", "10.4.5", "10.4.5-aikido.2"),
			patchFor("@sigstore/core", "1.0.0", "1.0.0-aikido.1"),
		}}, nil
	}}

	res, err := newTestBundledApp(t, []string{root}, srv.URL, false, api).run(context.Background())

	require.NoError(t, err)
	assert.Len(t, analyzed, 5, "every installed package is analyzed, nested and scoped included")
	assert.Len(t, res.Rotated, 3)
	assert.Empty(t, res.Failed)
	nm := filepath.Join(root, "npm", "node_modules")
	for dir, want := range map[string]string{"picomatch": "fixed picomatch", "glob": "fixed glob", "@sigstore/core": "fixed core"} {
		got, err := os.ReadFile(filepath.Join(nm, dir, "index.js"))
		require.NoError(t, err)
		assert.Equal(t, want, string(got))
	}
	_, err = os.Stat(filepath.Join(nm, "glob", "only-in-old.js"))
	assert.True(t, os.IsNotExist(err), "files the new version doesn't ship are removed")
	nested, err := os.ReadFile(filepath.Join(nm, "glob", "node_modules", "minimatch", "index.js"))
	require.NoError(t, err)
	assert.Equal(t, "nested minimatch", string(nested), "glob's own node_modules survives its swap")
	assert.Contains(t, reg.requests, "/npm/@sigstore%2fcore", "scoped packument path")
	entries, _ := os.ReadDir(nm)
	for _, e := range entries {
		assert.False(t, strings.Contains(e.Name(), ".rootio-"), "no temp dir left behind: %s", e.Name())
	}
}

func TestBundled_DryRunChangesNothingAndReportsPatchesAvailable(t *testing.T) {
	root := npmTree(t)
	api := &MockAPIClient{AnalyzePackagesFunc: func(context.Context, []rootio.Package, []rootio.Package, string) (*rootio.AnalyzePackagesResponse, error) {
		return &rootio.AnalyzePackagesResponse{Patches: []rootio.PackagePatch{patchFor("picomatch", "4.0.4", "4.0.4-aikido.1")}}, nil
	}}

	err := newTestBundledApp(t, []string{root}, "http://unused.invalid", true, api).Run(context.Background())

	assert.True(t, errors.Is(err, common.ErrPatchesAvailable))
	got, _ := os.ReadFile(filepath.Join(root, "npm", "node_modules", "picomatch", "index.js"))
	assert.Equal(t, "old picomatch", string(got))
}

func TestBundled_AFixThatCannotBeSwappedFailsTheRun(t *testing.T) {
	root := npmTree(t)
	good := npmTarball(t, "glob", "10.4.5-aikido.2", "fixed glob", nil)
	reg := &registry{
		tarballs: map[string]map[string][]byte{
			"glob":           {"10.4.5-aikido.2": good},
			"picomatch":      {"4.0.4-aikido.1": npmTarball(t, "picomatch", "4.0.4-aikido.1", "x", nil)},
			"@sigstore/core": {"1.0.0-aikido.1": npmTarball(t, "@sigstore/core", "1.0.0-aikido.1", "x", nil)},
		},
		integrity: map[string]string{"picomatch@4.0.4-aikido.1": sri([]byte("something else"))},
	}
	srv := reg.server(t)
	api := &MockAPIClient{AnalyzePackagesFunc: func(context.Context, []rootio.Package, []rootio.Package, string) (*rootio.AnalyzePackagesResponse, error) {
		return &rootio.AnalyzePackagesResponse{Patches: []rootio.PackagePatch{
			patchFor("picomatch", "4.0.4", "4.0.4-aikido.1"), // integrity mismatch
			patchFor("glob", "10.4.5", "10.4.5-aikido.2"),
			patchFor("@sigstore/core", "1.0.0", "1.0.0-aikido.9"), // the registry only has aikido.1
		}}, nil
	}}

	app := newTestBundledApp(t, []string{root}, srv.URL, false, api)
	res, err := app.run(context.Background())
	require.NoError(t, err)
	assert.Len(t, res.Rotated, 1, "glob still rotates")
	require.Len(t, res.Failed, 2)
	assert.Contains(t, strings.Join(res.Failed, "\n"), "integrity mismatch")
	assert.Contains(t, strings.Join(res.Failed, "\n"), "not in the Root registry")
	old, _ := os.ReadFile(filepath.Join(root, "npm", "node_modules", "picomatch", "index.js"))
	assert.Equal(t, "old picomatch", string(old), "a failed swap leaves the original in place")

	require.Error(t, newTestBundledApp(t, []string{npmTree(t)}, srv.URL, false, api).Run(context.Background()),
		"Run exits non-zero when a package with a Root build was not rotated")
}

func TestBundled_AnalyzeErrorFailsInsteadOfSkipping(t *testing.T) {
	api := &MockAPIClient{AnalyzePackagesFunc: func(context.Context, []rootio.Package, []rootio.Package, string) (*rootio.AnalyzePackagesResponse, error) {
		return nil, errors.New("503")
	}}

	err := newTestBundledApp(t, []string{npmTree(t)}, "http://unused.invalid", false, api).Run(context.Background())

	require.ErrorContains(t, err, "analyze bundled npm packages")
}

func TestBundled_IgnoredPackagesAreSentAsIgnore(t *testing.T) {
	var ignore []rootio.Package
	api := &MockAPIClient{AnalyzePackagesFunc: func(_ context.Context, _, ign []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
		ignore = ign
		return &rootio.AnalyzePackagesResponse{}, nil
	}}
	app := newTestBundledApp(t, []string{npmTree(t)}, "http://unused.invalid", false, api)
	app.ignoreSet = common.LoadIgnoreList("", []string{"minimatch@9.0.0"})

	require.NoError(t, app.Run(context.Background()))
	assert.Equal(t, []rootio.Package{{Name: "minimatch", Version: "9.0.0"}}, ignore)
}

func TestBundled_PnpmStoreEntriesAreSkippedNotSwapped(t *testing.T) {
	root := filepath.Join(t.TempDir(), "node_modules")
	writePkg(t, filepath.Join(root, ".pnpm", "axios@1.19.0", "node_modules", "axios"), "axios", "1.19.0", "axios")
	api := &MockAPIClient{AnalyzePackagesFunc: func(context.Context, []rootio.Package, []rootio.Package, string) (*rootio.AnalyzePackagesResponse, error) {
		t.Fatal("nothing to analyze")
		return nil, nil
	}}

	res, err := newTestBundledApp(t, []string{root}, "http://unused.invalid", false, api).run(context.Background())

	require.NoError(t, err)
	require.Len(t, res.Skipped, 1)
	assert.Contains(t, res.Skipped[0], "pnpm store entry")
}

func TestExtractPackage_RefusesEscapesAndLinks(t *testing.T) {
	escape := npmTarball(t, "x", "1.0.0", "x", map[string]string{"package/../../evil.js": "evil"})
	require.ErrorContains(t, extractPackage(escape, t.TempDir()), "unsafe path")

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "package/link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"}))
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	require.ErrorContains(t, extractPackage(buf.Bytes(), t.TempDir()), "link in package tarball")
}

func TestBundled_IgnoredPackageIsNotRotatedEvenIfTheAPIReturnsIt(t *testing.T) {
	root := npmTree(t)
	api := &MockAPIClient{AnalyzePackagesFunc: func(context.Context, []rootio.Package, []rootio.Package, string) (*rootio.AnalyzePackagesResponse, error) {
		return &rootio.AnalyzePackagesResponse{Patches: []rootio.PackagePatch{patchFor("minimatch", "9.0.0", "9.0.0-aikido.4")}}, nil
	}}
	app := newTestBundledApp(t, []string{root}, "http://unused.invalid", false, api)
	app.ignoreSet = common.LoadIgnoreList("", []string{"minimatch@9.0.0"})

	res, err := app.run(context.Background())

	require.NoError(t, err)
	assert.Empty(t, res.Rotated)
	assert.Empty(t, res.Failed)
}
