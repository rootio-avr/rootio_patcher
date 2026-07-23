package npm

import (
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/pkg/rootio"
)

func TestNpmApp_Run_FileNotFound(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	app := NewAppWithServices(
		"test-key",
		"https://api.root.io",
		"https://pkg.root.io",
		"/nonexistent/package-lock.json",
		true,  // dryRun
		true,  // useAlias
		nil,
		logger,
		&MockParser{},
		&MockAPIClient{},
		&MockCommandRunner{},
	)

	err := app.Run(ctx)
	if err == nil {
		t.Fatal("Expected error for nonexistent file, got nil")
	}
}

func TestNpmApp_Run_NoPackages(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create empty lock file
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "package-lock.json")
	content := `{"name": "test", "version": "1.0.0", "lockfileVersion": 3, "packages": {"": {}}}`
	if err := os.WriteFile(lockFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	app := NewAppWithServices(
		"test-key",
		"https://api.root.io",
		"https://pkg.root.io",
		lockFile,
		true,  // dryRun
		true,  // useAlias
		nil,
		logger,
		&MockParser{},
		&MockAPIClient{},
		&MockCommandRunner{},
	)

	err := app.Run(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
}

func TestNpmApp_Run_APIError(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "package-lock.json")
	content := `{
  "name": "test",
  "lockfileVersion": 3,
  "packages": {
    "": {},
    "node_modules/lodash": {"version": "4.17.20"}
  }
}`
	if err := os.WriteFile(lockFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	expectedError := errors.New("API error")
	mockAPIClient := &MockAPIClient{
		AnalyzePackagesFunc: func(ctx context.Context, packages []rootio.Package, ignore []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error) {
			return nil, expectedError
		},
	}

	mockParser := &MockParser{
		ParseFunc: func(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
			return []common.PackageInfo{
				{Name: "lodash", Version: "4.17.20"},
			}, nil
		},
	}

	app := NewAppWithServices(
		"test-key",
		"https://api.root.io",
		"https://pkg.root.io",
		lockFile,
		true,  // dryRun
		true,  // useAlias
		nil,
		logger,
		mockParser,
		mockAPIClient,
		&MockCommandRunner{},
	)

	err := app.Run(ctx)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if !errors.Is(err, expectedError) {
		t.Fatalf("Expected error to wrap API error, got: %v", err)
	}
}

func TestNpmApp_Run_NoPatches(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "package-lock.json")
	content := `{
  "name": "test",
  "lockfileVersion": 3,
  "packages": {
    "": {},
    "node_modules/lodash": {"version": "4.17.21"}
  }
}`
	if err := os.WriteFile(lockFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	mockAPIClient := &MockAPIClient{
		AnalyzePackagesFunc: func(ctx context.Context, packages []rootio.Package, ignore []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error) {
			return &rootio.AnalyzePackagesResponse{
				Patches: []rootio.PackagePatch{},
			}, nil
		},
	}

	app := NewAppWithServices(
		"test-key",
		"https://api.root.io",
		"https://pkg.root.io",
		lockFile,
		true,  // dryRun
		true,  // useAlias
		nil,
		logger,
		&MockParser{},
		mockAPIClient,
		&MockCommandRunner{},
	)

	err := app.Run(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
}

func TestNpmApp_Run_DryRun(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "package-lock.json")
	content := `{
  "name": "test",
  "lockfileVersion": 3,
  "packages": {
    "": {"dependencies": {"lodash": "^4.17.20"}},
    "node_modules/lodash": {"version": "4.17.20"}
  }
}`
	if err := os.WriteFile(lockFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	// Create package.json (should not be modified in dry-run)
	packageJSON := filepath.Join(tmpDir, "package.json")
	packageContent := `{
  "name": "test",
  "version": "1.0.0",
  "dependencies": {
    "lodash": "^4.17.20"
  }
}`
	if err := os.WriteFile(packageJSON, []byte(packageContent), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	mockAPIClient := &MockAPIClient{
		AnalyzePackagesFunc: func(ctx context.Context, packages []rootio.Package, ignore []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error) {
			return &rootio.AnalyzePackagesResponse{
				Patches: []rootio.PackagePatch{
					{
						PackageName: "lodash",
						Version:     "4.17.20",
						PatchAlias:  rootio.PatchInfo{Name: "@rootio/lodash", Version: "4.17.21"},
						CVEIDs:      []string{"CVE-2021-23337"},
					},
				},
			}, nil
		},
	}

	mockParser := &MockParser{
		ParseFunc: func(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
			return []common.PackageInfo{
				{Name: "lodash", Version: "4.17.20"},
			}, nil
		},
	}

	mockCmd := &MockCommandRunner{}

	app := NewAppWithServices(
		"test-key",
		"https://api.root.io",
		"https://pkg.root.io",
		lockFile,
		true,  // dry-run
		true,  // useAlias
		nil,
		logger,
		mockParser,
		mockAPIClient,
		mockCmd,
	)

	err := app.Run(ctx)
	if !errors.Is(err, common.ErrPatchesAvailable) {
		t.Fatalf("Expected ErrPatchesAvailable, got: %v", err)
	}

	// Verify that install was NOT invoked in dry-run mode
	if len(mockCmd.Calls) != 0 {
		t.Errorf("Expected no command calls in dry-run, got %d", len(mockCmd.Calls))
	}

	// Verify package.json was NOT modified in dry-run mode
	updatedContent, err := os.ReadFile(packageJSON)
	if err != nil {
		t.Fatalf("Failed to read package.json: %v", err)
	}
	if strings.Contains(string(updatedContent), "overrides") {
		t.Error("package.json should not be modified in dry-run mode")
	}
	if strings.Contains(string(updatedContent), "@rootio/lodash") {
		t.Error("package.json should not contain patches in dry-run mode")
	}
}

func TestNpmApp_Run_ApplyPatches(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "package-lock.json")
	lockContent := `{
  "name": "test",
  "lockfileVersion": 3,
  "packages": {
    "": {"dependencies": {"lodash": "^4.17.20"}},
    "node_modules/lodash": {
      "version": "4.17.20",
      "resolved": "https://registry.npmjs.org/lodash/-/lodash-4.17.20.tgz"
    }
  }
}`
	if err := os.WriteFile(lockFile, []byte(lockContent), 0644); err != nil {
		t.Fatalf("Failed to create lock file: %v", err)
	}

	// Create package.json
	packageJSON := filepath.Join(tmpDir, "package.json")
	packageContent := `{
  "name": "test",
  "version": "1.0.0",
  "dependencies": {
    "lodash": "^4.17.20"
  }
}`
	if err := os.WriteFile(packageJSON, []byte(packageContent), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	// Change to tmpDir so UpdatePackageJSON can find package.json
	t.Chdir(tmpDir)

	mockAPIClient := &MockAPIClient{
		AnalyzePackagesFunc: func(ctx context.Context, packages []rootio.Package, ignore []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error) {
			return &rootio.AnalyzePackagesResponse{
				Patches: []rootio.PackagePatch{
					{
						PackageName: "lodash",
						Version:     "4.17.20",
						PatchAlias:  rootio.PatchInfo{Name: "@rootio/lodash", Version: "4.17.21"},
						CVEIDs:      []string{"CVE-2021-23337"},
					},
				},
			}, nil
		},
	}

	// Use MockNpmParser that uses real UpdatePackageJSON but mocks Parse
	mockParser := &MockNpmParser{
		NpmParser: NewNpmParser(),
		ParseFunc: func(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
			return []common.PackageInfo{
				{Name: "lodash", Version: "4.17.20"},
			}, nil
		},
	}

	mockCmd := &MockCommandRunner{}

	app := NewAppWithServices(
		"test-key",
		"https://api.root.io",
		"https://pkg.root.io",
		lockFile,
		false, // NOT dry-run
		true,  // useAlias
		nil,
		logger,
		mockParser,
		mockAPIClient,
		mockCmd,
	)

	err := app.Run(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify package.json was patched. lodash is direct + vulnerable, so the
	// dependency VALUE is rewritten to the npm: alias while keeping the original
	// "lodash" key — the only form npm can resolve for a -root.io build (which lives
	// solely under the @rootio scope). The key is NOT renamed to @rootio/lodash, and
	// NO overrides entry is emitted for the direct dep (that triggers EOVERRIDE).
	updatedContent, err := os.ReadFile(packageJSON)
	if err != nil {
		t.Fatalf("Failed to read package.json: %v", err)
	}
	content := string(updatedContent)
	if !strings.Contains(content, `"lodash": "npm:@rootio/lodash@4.17.21"`) {
		t.Errorf("package.json should rewrite lodash direct dep to the npm: alias; got:\n%s", content)
	}
	if strings.Contains(content, `"@rootio/lodash": "4.17.21"`) {
		t.Error("direct dep key must not be renamed to @rootio/lodash")
	}
	if strings.Contains(content, `"lodash@4.17.20":`) {
		t.Error("direct dep must not also get an overrides entry (EOVERRIDE)")
	}
}

func TestApp_isYarnClassic(t *testing.T) {
	dir := t.TempDir()
	classic := filepath.Join(dir, "classic.lock")
	berry := filepath.Join(dir, "berry.lock")
	missing := filepath.Join(dir, "missing.lock")

	if err := os.WriteFile(classic, []byte("# THIS IS AUTOGENERATED\n# yarn lockfile v1\n\nlodash@4.17.20:\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(berry, []byte("__metadata:\n  version: 8\n  cacheKey: 10c0\n"), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name           string
		packageManager string
		lockFilePath   string
		want           bool
	}{
		{"classic yarn1 lockfile + pm=yarn", "yarn", classic, true},
		{"berry lockfile + pm=yarn", "yarn", berry, false},
		{"classic header but pm=npm", "npm", classic, false},
		{"classic header but pm=pnpm", "pnpm", classic, false},
		{"missing lockfile", "yarn", missing, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &App{packageManager: tt.packageManager, lockFilePath: tt.lockFilePath}
			if got := a.isYarnClassic(); got != tt.want {
				t.Errorf("isYarnClassic() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNpmApp_Run_InvokesInstallAfterPatch(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "package-lock.json")
	lockContent := `{
  "name": "test",
  "lockfileVersion": 3,
  "packages": {
    "": {"dependencies": {"lodash": "^4.17.20"}},
    "node_modules/lodash": {
      "version": "4.17.20",
      "resolved": "https://registry.npmjs.org/lodash/-/lodash-4.17.20.tgz"
    }
  }
}`
	if err := os.WriteFile(lockFile, []byte(lockContent), 0644); err != nil {
		t.Fatalf("Failed to create lock file: %v", err)
	}

	// Create package.json
	packageJSON := filepath.Join(tmpDir, "package.json")
	packageContent := `{
  "name": "test",
  "version": "1.0.0",
  "dependencies": {
    "lodash": "^4.17.20"
  }
}`
	if err := os.WriteFile(packageJSON, []byte(packageContent), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	// Change to tmpDir so UpdatePackageJSON can find package.json
	t.Chdir(tmpDir)

	mockAPIClient := &MockAPIClient{
		AnalyzePackagesFunc: func(ctx context.Context, packages []rootio.Package, ignore []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error) {
			return &rootio.AnalyzePackagesResponse{
				Patches: []rootio.PackagePatch{
					{
						PackageName: "lodash",
						Version:     "4.17.20",
						PatchAlias:  rootio.PatchInfo{Name: "@rootio/lodash", Version: "4.17.21"},
						CVEIDs:      []string{"CVE-2021-23337"},
					},
				},
			}, nil
		},
	}

	// Use MockNpmParser that uses real UpdatePackageJSON but mocks Parse
	mockParser := &MockNpmParser{
		NpmParser: NewNpmParser(),
		ParseFunc: func(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
			return []common.PackageInfo{
				{Name: "lodash", Version: "4.17.20"},
			}, nil
		},
	}

	mockCmd := &MockCommandRunner{}

	app := NewAppWithServices(
		"test-key",
		"https://api.root.io",
		"https://pkg.root.io",
		lockFile,
		false, // NOT dry-run
		true,  // useAlias
		nil,
		logger,
		mockParser,
		mockAPIClient,
		mockCmd,
	)
	// Inject stub lookPath so test doesn't depend on real npm binary
	app.lookPath = func(name string) (string, error) {
		return "/usr/bin/" + name, nil
	}

	err := app.Run(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify that npm install was invoked
	if len(mockCmd.Calls) != 1 {
		t.Fatalf("Expected 1 command call, got %d", len(mockCmd.Calls))
	}
	if mockCmd.Calls[0].Name != "npm" {
		t.Errorf("Expected command 'npm', got '%s'", mockCmd.Calls[0].Name)
	}
	found := false
	for _, arg := range mockCmd.Calls[0].Args {
		if arg == "install" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'install' in args, got %v", mockCmd.Calls[0].Args)
	}

	// The resolver MUST run with Root.io registry + auth env, or `npm install`
	// cannot fetch the @rootio-scoped override packages (401/ETARGET).
	env := strings.Join(mockCmd.Calls[0].Env, "\n")
	if !strings.Contains(env, "npm_config_@rootio:registry=https://pkg.root.io/npm/") {
		t.Errorf("Expected @rootio scoped registry env, got: %v", mockCmd.Calls[0].Env)
	}
	if !strings.Contains(env, ":username=root") {
		t.Errorf("Expected basic-auth username=root env, got: %v", mockCmd.Calls[0].Env)
	}
	if !strings.Contains(env, ":_password=") {
		t.Errorf("Expected basic-auth _password env, got: %v", mockCmd.Calls[0].Env)
	}
}

func TestNpmEnv(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// With pkgURL + apiKey → scoped registry + base64 basic-auth env.
	app := &App{apiKey: "sk_testkey", pkgURL: "https://pkg.root.io", logger: logger}
	env := app.npmEnv()
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "npm_config_@rootio:registry=https://pkg.root.io/npm/") {
		t.Errorf("missing scoped registry, got: %v", env)
	}
	if !strings.Contains(joined, "npm_config_//pkg.root.io/npm/:username=root") {
		t.Errorf("missing username, got: %v", env)
	}
	// password is base64(apiKey)
	wantPass := "npm_config_//pkg.root.io/npm/:_password=" + base64.StdEncoding.EncodeToString([]byte("sk_testkey"))
	if !strings.Contains(joined, wantPass) {
		t.Errorf("missing/incorrect base64 password, got: %v", env)
	}

	// Missing pkgURL → nil (nothing to configure).
	if got := (&App{apiKey: "k", logger: logger}).npmEnv(); got != nil {
		t.Errorf("expected nil env when pkgURL empty, got: %v", got)
	}
	// Missing apiKey → nil.
	if got := (&App{pkgURL: "https://pkg.root.io", logger: logger}).npmEnv(); got != nil {
		t.Errorf("expected nil env when apiKey empty, got: %v", got)
	}
}

func TestNpmApp_Run_SkipsInstallWhenPMAbsent(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "package-lock.json")
	lockContent := `{
  "name": "test",
  "lockfileVersion": 3,
  "packages": {
    "": {"dependencies": {"lodash": "^4.17.20"}},
    "node_modules/lodash": {
      "version": "4.17.20",
      "resolved": "https://registry.npmjs.org/lodash/-/lodash-4.17.20.tgz"
    }
  }
}`
	if err := os.WriteFile(lockFile, []byte(lockContent), 0644); err != nil {
		t.Fatalf("Failed to create lock file: %v", err)
	}

	// Create package.json
	packageJSON := filepath.Join(tmpDir, "package.json")
	packageContent := `{
  "name": "test",
  "version": "1.0.0",
  "dependencies": {
    "lodash": "^4.17.20"
  }
}`
	if err := os.WriteFile(packageJSON, []byte(packageContent), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	// Change to tmpDir so UpdatePackageJSON can find package.json
	t.Chdir(tmpDir)

	mockAPIClient := &MockAPIClient{
		AnalyzePackagesFunc: func(ctx context.Context, packages []rootio.Package, ignore []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error) {
			return &rootio.AnalyzePackagesResponse{
				Patches: []rootio.PackagePatch{
					{
						PackageName: "lodash",
						Version:     "4.17.20",
						PatchAlias:  rootio.PatchInfo{Name: "@rootio/lodash", Version: "4.17.21"},
						CVEIDs:      []string{"CVE-2021-23337"},
					},
				},
			}, nil
		},
	}

	// Use MockNpmParser that uses real UpdatePackageJSON but mocks Parse
	mockParser := &MockNpmParser{
		NpmParser: NewNpmParser(),
		ParseFunc: func(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
			return []common.PackageInfo{
				{Name: "lodash", Version: "4.17.20"},
			}, nil
		},
	}

	mockCmd := &MockCommandRunner{}

	app := NewAppWithServices(
		"test-key",
		"https://api.root.io",
		"https://pkg.root.io",
		lockFile,
		false, // NOT dry-run
		true,  // useAlias
		nil,
		logger,
		mockParser,
		mockAPIClient,
		mockCmd,
	)
	// Inject stub lookPath that simulates missing package manager
	app.lookPath = func(name string) (string, error) {
		return "", exec.ErrNotFound
	}

	err := app.Run(ctx)
	if err != nil {
		t.Fatalf("Expected no error (graceful degrade), got: %v", err)
	}

	// Verify that install was NOT invoked when PM is absent
	if len(mockCmd.Calls) != 0 {
		t.Errorf("Expected no command calls when package manager not found, got %d calls", len(mockCmd.Calls))
	}

	// Verify package.json was still patched (manifest update happens before resolver)
	updatedContent, err := os.ReadFile(packageJSON)
	if err != nil {
		t.Fatalf("Failed to read package.json: %v", err)
	}
	content := string(updatedContent)
	if !strings.Contains(content, `"lodash": "npm:@rootio/lodash@4.17.21"`) {
		t.Error("package.json should still be patched (direct dep rewritten to npm: alias) even when PM not found")
	}
}

func TestNpmApp_Run_DryRunDoesNotInvokeInstall(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "package-lock.json")
	lockContent := `{
  "name": "test",
  "lockfileVersion": 3,
  "packages": {
    "": {"dependencies": {"lodash": "^4.17.20"}},
    "node_modules/lodash": {
      "version": "4.17.20",
      "resolved": "https://registry.npmjs.org/lodash/-/lodash-4.17.20.tgz"
    }
  }
}`
	if err := os.WriteFile(lockFile, []byte(lockContent), 0644); err != nil {
		t.Fatalf("Failed to create lock file: %v", err)
	}

	// Create package.json
	packageJSON := filepath.Join(tmpDir, "package.json")
	packageContent := `{
  "name": "test",
  "version": "1.0.0",
  "dependencies": {
    "lodash": "^4.17.20"
  }
}`
	if err := os.WriteFile(packageJSON, []byte(packageContent), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	mockAPIClient := &MockAPIClient{
		AnalyzePackagesFunc: func(ctx context.Context, packages []rootio.Package, ignore []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error) {
			return &rootio.AnalyzePackagesResponse{
				Patches: []rootio.PackagePatch{
					{
						PackageName: "lodash",
						Version:     "4.17.20",
						PatchAlias:  rootio.PatchInfo{Name: "@rootio/lodash", Version: "4.17.21"},
						CVEIDs:      []string{"CVE-2021-23337"},
					},
				},
			}, nil
		},
	}

	mockParser := &MockNpmParser{
		NpmParser: NewNpmParser(),
		ParseFunc: func(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
			return []common.PackageInfo{
				{Name: "lodash", Version: "4.17.20"},
			}, nil
		},
	}

	mockCmd := &MockCommandRunner{}

	app := NewAppWithServices(
		"test-key",
		"https://api.root.io",
		"https://pkg.root.io",
		lockFile,
		true, // DRY-RUN
		true, // useAlias
		nil,
		logger,
		mockParser,
		mockAPIClient,
		mockCmd,
	)

	err := app.Run(ctx)
	if !errors.Is(err, common.ErrPatchesAvailable) {
		t.Fatalf("Expected ErrPatchesAvailable in dry-run, got: %v", err)
	}

	// Verify that install was NOT invoked in dry-run mode
	if len(mockCmd.Calls) != 0 {
		t.Errorf("Expected no command calls in dry-run, got %d calls", len(mockCmd.Calls))
	}
}