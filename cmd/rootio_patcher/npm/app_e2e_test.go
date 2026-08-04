package npm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"rootio_patcher/pkg/rootio"
)

// axios patch info as returned by the live /v3/analyze/npm endpoint (captured
// 2026-08-04) for all three input shapes exercised below: an already-aliased
// direct dep left over from a prior version of the tool (@rootio/axios), a
// plain direct dep on the pre-rebrand root.io version suffix, and a plain
// direct dep already on the aikido suffix. All three converge on the same
// patch target.
var axiosPatch = rootio.PatchInfo{Name: "axios", Version: "1.15.0-aikido.12"}

// TestNpmApp_E2E_NonAliased_AxiosScenarios runs npm/yarn/pnpm remediation for
// the three input scenarios above, and asserts every package manager
// converges on a plain "axios": "1.15.0-aikido.12" direct dependency — no
// "npm:" alias descriptor and no leftover @rootio/axios key. The npm
// ecosystem never aliases packages: direct deps and overrides always use the
// plain patched name/version.
func TestNpmApp_E2E_NonAliased_AxiosScenarios(t *testing.T) {
	scenarios := []struct {
		name    string
		pkgName string
		version string
	}{
		{name: "already_aliased_rootio_suffix", pkgName: "@rootio/axios", version: "1.15.0-root.io.11"},
		{name: "plain_rootio_suffix", pkgName: "axios", version: "1.15.0-root.io.11"},
		{name: "plain_aikido_suffix", pkgName: "axios", version: "1.15.0-aikido.11"},
	}

	packageManagers := []string{"npm", "yarn", "pnpm"}

	for _, sc := range scenarios {
		for _, pm := range packageManagers {
			t.Run(sc.name+"/"+pm, func(t *testing.T) {
				runNonAliasedAxiosScenario(t, pm, sc.pkgName, sc.version)
			})
		}
	}
}

// runNonAliasedAxiosScenario builds a package.json + lock file fixture for
// the given package manager with pkgName@version as a direct dependency,
// runs the real App against a mocked analyze API response, and asserts the
// resulting package.json.
func runNonAliasedAxiosScenario(t *testing.T, packageManager, pkgName, version string) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	tmpDir := t.TempDir()
	packageJSON := filepath.Join(tmpDir, "package.json")
	if err := os.WriteFile(packageJSON, []byte(fmt.Sprintf(`{
  "name": "test-project",
  "version": "1.0.0",
  "dependencies": {
    %q: %q
  }
}`, pkgName, version)), 0644); err != nil {
		t.Fatalf("failed to create package.json: %v", err)
	}

	var parser npmParser
	switch packageManager {
	case "npm":
		writeNpmLockFixture(t, tmpDir, pkgName, version)
		parser = NewParser()
	case "yarn":
		writeYarnLockFixture(t, tmpDir, pkgName, version)
		parser = NewYarnParser()
	case "pnpm":
		writePnpmLockFixture(t, tmpDir, pkgName, version)
		parser = NewPnpmParser()
	default:
		t.Fatalf("unsupported package manager: %s", packageManager)
	}

	originalDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(originalDir) }()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	mockAPIClient := &MockAPIClient{
		AnalyzePackagesFunc: func(_ context.Context, packages []rootio.Package, _ []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error) {
			if ecosystem != "npm" {
				t.Errorf("expected ecosystem %q, got %q", "npm", ecosystem)
			}
			found := false
			for _, p := range packages {
				if p.Name == pkgName && p.Version == version {
					found = true
				}
			}
			if !found {
				t.Errorf("expected %s@%s to be sent to the analyze API, got %+v", pkgName, version, packages)
			}
			return &rootio.AnalyzePackagesResponse{
				Patches: []rootio.PackagePatch{
					{
						PackageName: pkgName,
						Version:     version,
						Patch:       axiosPatch,
						PatchAlias:  rootio.PatchInfo{Name: "@rootio/axios", Version: axiosPatch.Version},
					},
				},
			}, nil
		},
	}

	app := NewAppWithServices(
		"test-key",
		"https://api.root.io",
		packageManager,
		false, // not dry-run
		nil,
		logger,
		parser,
		mockAPIClient,
	)

	if err := app.Run(ctx); err != nil {
		t.Fatalf("app run failed: %v", err)
	}

	updatedContent, err := os.ReadFile(packageJSON)
	if err != nil {
		t.Fatalf("failed to read updated package.json: %v", err)
	}
	var pkgJSON map[string]interface{}
	if err := json.Unmarshal(updatedContent, &pkgJSON); err != nil {
		t.Fatalf("failed to parse updated package.json: %v\n%s", err, updatedContent)
	}
	deps, _ := pkgJSON["dependencies"].(map[string]interface{})

	if pkgName != axiosPatch.Name {
		if _, exists := deps[pkgName]; exists {
			t.Errorf("expected old key %q to be removed from dependencies, got %+v", pkgName, deps)
		}
	}

	got, ok := deps[axiosPatch.Name]
	if !ok {
		t.Fatalf("expected dependencies[%q] to be set, got %+v", axiosPatch.Name, deps)
	}
	if got != axiosPatch.Version {
		t.Errorf("expected dependencies[%q] = %q (plain, de-aliased), got %q", axiosPatch.Name, axiosPatch.Version, got)
	}
}

// writeNpmLockFixture creates a minimal but valid package-lock.json (v3)
// declaring pkgName@version as both a direct dependency and its resolved
// top-level node_modules entry.
func writeNpmLockFixture(t *testing.T, dir, pkgName, version string) {
	t.Helper()
	lockFile := filepath.Join(dir, "package-lock.json")
	content := fmt.Sprintf(`{
  "name": "test-project",
  "lockfileVersion": 3,
  "packages": {
    "": {
      "dependencies": { %q: %q }
    },
    "node_modules/%s": {
      "version": %q
    }
  }
}`, pkgName, version, pkgName, version)
	if err := os.WriteFile(lockFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create package-lock.json: %v", err)
	}
}

// writeYarnLockFixture creates a minimal Yarn 1 (classic) yarn.lock entry
// resolving pkgName's exact version-pinned range to version.
func writeYarnLockFixture(t *testing.T, dir, pkgName, version string) {
	t.Helper()
	lockFile := filepath.Join(dir, "yarn.lock")
	content := fmt.Sprintf(`# THIS IS AN AUTOGENERATED FILE. DO NOT EDIT THIS FILE DIRECTLY.
# yarn lockfile v1

%q:
  version %q
  resolved "https://registry.yarnpkg.com/axios/-/axios-1.15.0.tgz"
  integrity sha512-test
`, pkgName+"@"+version, version)
	if err := os.WriteFile(lockFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create yarn.lock: %v", err)
	}
}

// writePnpmLockFixture creates a minimal pnpm-lock.yaml declaring pkgName as
// a root importer dependency resolved to version.
func writePnpmLockFixture(t *testing.T, dir, pkgName, version string) {
	t.Helper()
	lockFile := filepath.Join(dir, "pnpm-lock.yaml")
	content := fmt.Sprintf(`lockfileVersion: '9.0'
importers:
  .:
    dependencies:
      '%s':
        specifier: %q
        version: %q
packages:
  '%s@%s':
    resolution: {integrity: sha512-test}
`, pkgName, version, version, pkgName, version)
	if err := os.WriteFile(lockFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create pnpm-lock.yaml: %v", err)
	}
}
