package npm

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"rootio_patcher/pkg/rootio"
)

var wsPatch = rootio.PatchInfo{Name: "ws", Version: "7.5.10-aikido.1"}

// Lock shape from the kfp-frontend rebuild that crashed with
// "Cannot find module 'ws'": isomorphic-ws only *peer*-depends on ws, so the
// only thing that satisfies it is the hoisted top-level node_modules/ws.
// Overrides scoped to @kubernetes/client-node / jsdom make npm re-resolve ws
// as nested copies and drop the hoisted one.
const peerConsumerLock = `{
  "name": "server",
  "lockfileVersion": 3,
  "packages": {
    "": { "dependencies": { "@kubernetes/client-node": "^0.16.3" } },
    "node_modules/@kubernetes/client-node": {
      "version": "0.16.3",
      "dependencies": { "isomorphic-ws": "^4.0.1", "ws": "^7.3.1" }
    },
    "node_modules/isomorphic-ws": {
      "version": "4.0.1",
      "peerDependencies": { "ws": "*" }
    },
    "node_modules/ws": { "version": "7.5.10" }
  }
}`

func runPeerPinScenario(t *testing.T, pkgJSON, lock string) map[string]interface{} {
	t.Helper()
	ctx := context.Background()
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(pkgJSON), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "package-lock.json"), []byte(lock), 0644); err != nil {
		t.Fatal(err)
	}

	originalDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(originalDir) }()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	api := &MockAPIClient{
		AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, _ []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
			return &rootio.AnalyzePackagesResponse{
				Patches: []rootio.PackagePatch{{PackageName: "ws", Version: "7.5.10", Patch: wsPatch}},
			}, nil
		},
	}
	app := NewAppWithServices("k", "https://api.root.io", "npm", false, nil,
		slog.New(slog.NewTextHandler(os.Stdout, nil)), NewParser(), api)
	if err := app.Run(ctx); err != nil {
		t.Fatalf("app run failed: %v", err)
	}

	out, err := os.ReadFile(filepath.Join(tmpDir, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("bad package.json: %v\n%s", err, out)
	}
	return got
}

func depsOf(m map[string]interface{}, field string) map[string]interface{} {
	d, _ := m[field].(map[string]interface{})
	return d
}

// A hoisted package that a peer-only consumer relies on is pinned as a direct
// dependency at the patched version, so npm keeps a top-level copy.
func TestNpmApp_PinsHoistedPackageWithPeerConsumer(t *testing.T) {
	got := runPeerPinScenario(t, `{
  "name": "server",
  "dependencies": { "@kubernetes/client-node": "^0.16.3" }
}`, peerConsumerLock)

	if v := depsOf(got, "dependencies")["ws"]; v != wsPatch.Version {
		t.Errorf(`expected dependencies.ws = %q, got %v`, wsPatch.Version, v)
	}
	if v := depsOf(got, "overrides")["ws@7.5.10"]; v != wsPatch.Version {
		t.Errorf(`expected overrides["ws@7.5.10"] = %q, got %v`, wsPatch.Version, v)
	}
	if v := depsOf(got, "dependencies")["@kubernetes/client-node"]; v != "^0.16.3" {
		t.Errorf("existing direct dependency must be left alone, got %v", v)
	}
}

// Optional peers (peerDependenciesMeta.optional) are not required to be
// present, so they must not turn into direct dependencies.
func TestNpmApp_DoesNotPinOptionalPeer(t *testing.T) {
	lock := `{
  "lockfileVersion": 3,
  "packages": {
    "": { "dependencies": { "@kubernetes/client-node": "^0.16.3" } },
    "node_modules/@kubernetes/client-node": { "version": "0.16.3", "dependencies": { "ws": "^7.3.1" } },
    "node_modules/some-lib": {
      "version": "1.0.0",
      "peerDependencies": { "ws": "*" },
      "peerDependenciesMeta": { "ws": { "optional": true } }
    },
    "node_modules/ws": { "version": "7.5.10" }
  }
}`
	got := runPeerPinScenario(t, `{
  "name": "server",
  "dependencies": { "@kubernetes/client-node": "^0.16.3" }
}`, lock)

	if _, ok := depsOf(got, "dependencies")["ws"]; ok {
		t.Errorf("optional peer must not be pinned as a direct dependency: %v", got["dependencies"])
	}
}

// No peer consumer means nothing relies on the hoisted copy: package.json
// dependencies stay exactly as they were.
func TestNpmApp_DoesNotPinWithoutPeerConsumer(t *testing.T) {
	lock := `{
  "lockfileVersion": 3,
  "packages": {
    "": { "dependencies": { "@kubernetes/client-node": "^0.16.3" } },
    "node_modules/@kubernetes/client-node": { "version": "0.16.3", "dependencies": { "ws": "^7.3.1" } },
    "node_modules/ws": { "version": "7.5.10" }
  }
}`
	got := runPeerPinScenario(t, `{
  "name": "server",
  "dependencies": { "@kubernetes/client-node": "^0.16.3" }
}`, lock)

	if _, ok := depsOf(got, "dependencies")["ws"]; ok {
		t.Errorf("must not add ws when nothing peer-depends on it: %v", got["dependencies"])
	}
}

// The patched copy is only hoisted at a different version (the vulnerable
// copy is nested): a pin would force the wrong version at the top level.
func TestNpmApp_DoesNotPinWhenHoistedCopyIsADifferentVersion(t *testing.T) {
	lock := `{
  "lockfileVersion": 3,
  "packages": {
    "": { "dependencies": { "@kubernetes/client-node": "^0.16.3", "ws": "^8.0.0" } },
    "node_modules/@kubernetes/client-node": { "version": "0.16.3", "dependencies": { "ws": "^7.3.1" } },
    "node_modules/@kubernetes/client-node/node_modules/ws": { "version": "7.5.10" },
    "node_modules/isomorphic-ws": { "version": "4.0.1", "peerDependencies": { "ws": "*" } },
    "node_modules/ws": { "version": "8.18.0" }
  }
}`
	got := runPeerPinScenario(t, `{
  "name": "server",
  "dependencies": { "@kubernetes/client-node": "^0.16.3", "ws": "^8.0.0" }
}`, lock)

	if v := depsOf(got, "dependencies")["ws"]; v != "^8.0.0" {
		t.Errorf("direct ws spec must be untouched, got %v", v)
	}
}

// A package the project already declares directly keeps its own spec: pinning
// would overwrite the user's range.
func TestNpmApp_DoesNotPinAlreadyDirectDependency(t *testing.T) {
	got := runPeerPinScenario(t, `{
  "name": "server",
  "dependencies": { "@kubernetes/client-node": "^0.16.3" },
  "devDependencies": { "ws": "^7.5.0" }
}`, peerConsumerLock)

	if _, ok := depsOf(got, "dependencies")["ws"]; ok {
		t.Errorf("ws is already a devDependency; must not be duplicated into dependencies: %v", got["dependencies"])
	}
}
