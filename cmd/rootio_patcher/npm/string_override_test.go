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

// kubeflow pins `router` to 1.3.7 with a plain string override (express 5 would otherwise pull
// router 2.x, which needs path-to-regexp 8). The patcher then needs to add a child override under
// `router`. Writing `overrides.router.path-to-regexp` on top of the string used to replace the
// string with an object and silently drop the pin.
func TestNpmApp_KeepsStringOverrideWhenAddingChildOverride(t *testing.T) {
	tmpDir := t.TempDir()
	pkg := `{
  "name": "server",
  "dependencies": { "express": "^5.1.0" },
  "overrides": { "path-to-regexp": "0.1.12", "router": "1.3.7" }
}`
	lock := `{
  "lockfileVersion": 3,
  "packages": {
    "": { "dependencies": { "express": "^5.1.0" } },
    "node_modules/express": { "version": "5.1.0", "dependencies": { "router": "^2.2.0" } },
    "node_modules/router": { "version": "1.3.7", "dependencies": { "path-to-regexp": "0.1.7" } },
    "node_modules/path-to-regexp": { "version": "0.1.12" }
  }
}`
	if err := os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(pkg), 0644); err != nil {
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
				Patches: []rootio.PackagePatch{{
					PackageName: "path-to-regexp", Version: "0.1.12",
					Patch: rootio.PatchInfo{Name: "path-to-regexp", Version: "0.1.12-aikido.2"},
				}},
			}, nil
		},
	}
	app := NewAppWithServices("k", "https://api.root.io", "npm", false, nil,
		slog.New(slog.NewTextHandler(os.Stdout, nil)), NewParser(), api)
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("app run failed: %v", err)
	}

	out, err := os.ReadFile(filepath.Join(tmpDir, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Overrides map[string]interface{} `json:"overrides"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("bad package.json: %v\n%s", err, out)
	}
	router, ok := got.Overrides["router"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected overrides.router to be an object, got %#v", got.Overrides["router"])
	}
	if router["."] != "1.3.7" {
		t.Errorf(`the "router": "1.3.7" pin must survive as {".": "1.3.7"}, got %#v`, router)
	}
	if router["path-to-regexp"] != "0.1.12-aikido.2" {
		t.Errorf("expected the child override under router, got %#v", router)
	}
}
