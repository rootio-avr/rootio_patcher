package composer

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/pkg/rootio"
)

func newTestApp(filePath string, dryRun, useAlias bool, parser common.Parser, apiClient common.APIClient, cmdRunner CommandRunner) *App {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	return NewAppWithServices(
		"test-key",
		"https://api.root.io",
		"https://pkg.root.io",
		filePath,
		dryRun,
		useAlias,
		nil,
		logger,
		parser,
		apiClient,
		cmdRunner,
	)
}

func TestComposerApp_Run_FileNotFound(t *testing.T) {
	app := newTestApp("/nonexistent/composer.json", true, false, &MockParser{}, &MockAPIClient{}, &MockCommandRunner{})
	err := app.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file not found")
}

func TestComposerApp_Run_NoPackages(t *testing.T) {
	tmpDir := t.TempDir()
	composerFile := filepath.Join(tmpDir, "composer.json")
	require.NoError(t, os.WriteFile(composerFile, []byte(`{"require":{}}`), 0644))

	app := newTestApp(composerFile, true, false, &MockParser{}, &MockAPIClient{}, &MockCommandRunner{})
	err := app.Run(context.Background())
	require.NoError(t, err)
}

func TestComposerApp_Run_APIError(t *testing.T) {
	tmpDir := t.TempDir()
	composerFile := filepath.Join(tmpDir, "composer.json")
	require.NoError(t, os.WriteFile(composerFile, []byte(`{"require":{"vendor/pkg":"^2.1.0"}}`), 0644))

	expectedErr := errors.New("API error")
	mockAPI := &MockAPIClient{
		AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, _ []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
			return nil, expectedErr
		},
	}
	mockParser := &MockParser{
		ParseFunc: func(_ context.Context, _ string) ([]common.PackageInfo, error) {
			return []common.PackageInfo{{Name: "vendor/pkg", Version: "2.1.0"}}, nil
		},
	}

	app := newTestApp(composerFile, true, false, mockParser, mockAPI, &MockCommandRunner{})
	err := app.Run(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, expectedErr)
}

func TestComposerApp_Run_NoPatches(t *testing.T) {
	tmpDir := t.TempDir()
	composerFile := filepath.Join(tmpDir, "composer.json")
	require.NoError(t, os.WriteFile(composerFile, []byte(`{"require":{"vendor/pkg":"^2.1.0"}}`), 0644))

	mockAPI := &MockAPIClient{
		AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, _ []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
			return &rootio.AnalyzePackagesResponse{Patches: []rootio.PackagePatch{}}, nil
		},
	}
	mockParser := &MockParser{
		ParseFunc: func(_ context.Context, _ string) ([]common.PackageInfo, error) {
			return []common.PackageInfo{{Name: "vendor/pkg", Version: "2.1.0"}}, nil
		},
	}

	app := newTestApp(composerFile, true, false, mockParser, mockAPI, &MockCommandRunner{})
	err := app.Run(context.Background())
	require.NoError(t, err)
}

func TestComposerApp_Run_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	composerFile := filepath.Join(tmpDir, "composer.json")
	original := `{"require":{"vendor/pkg":"^2.1.0"}}`
	require.NoError(t, os.WriteFile(composerFile, []byte(original), 0644))

	mockAPI := &MockAPIClient{
		AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, _ []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
			return &rootio.AnalyzePackagesResponse{
				Patches: []rootio.PackagePatch{
					{
						PackageName: "vendor/pkg",
						Version:     "2.1.0",
						Patch:       rootio.PatchInfo{Name: "vendor/pkg", Version: "2.1.4"},
						CVEIDs:      []string{"CVE-2024-1234"},
					},
				},
			}, nil
		},
	}
	mockParser := &MockParser{
		ParseFunc: func(_ context.Context, _ string) ([]common.PackageInfo, error) {
			return []common.PackageInfo{{Name: "vendor/pkg", Version: "2.1.0"}}, nil
		},
	}
	mockCmd := &MockCommandRunner{}

	app := newTestApp(composerFile, true, false, mockParser, mockAPI, mockCmd)
	err := app.Run(context.Background())
	assert.ErrorIs(t, err, common.ErrPatchesAvailable)

	// File must not be modified
	content, _ := os.ReadFile(composerFile)
	assert.Equal(t, original, string(content))

	// composer must not be invoked
	assert.Empty(t, mockCmd.Calls)
}

func TestComposerApp_Run_ApplyPatches_DirectPatch(t *testing.T) {
	tmpDir := t.TempDir()
	composerFile := filepath.Join(tmpDir, "composer.json")
	require.NoError(t, os.WriteFile(composerFile, []byte(`{"require":{"vendor/pkg":"^2.1.0"}}`), 0644))

	mockAPI := &MockAPIClient{
		AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, _ []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
			return &rootio.AnalyzePackagesResponse{
				Patches: []rootio.PackagePatch{
					{
						PackageName: "vendor/pkg",
						Version:     "2.1.0",
						Patch:       rootio.PatchInfo{Name: "vendor/pkg", Version: "2.1.4"},
						CVEIDs:      []string{"CVE-2024-1234"},
					},
				},
			}, nil
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	realParser := NewParser(logger, "https://pkg.root.io")
	mockParser := &MockComposerParser{
		ComposerParser: realParser,
		ParseFunc: func(_ context.Context, _ string) ([]common.PackageInfo, error) {
			return []common.PackageInfo{{Name: "vendor/pkg", Version: "2.1.0"}}, nil
		},
	}
	mockCmd := &MockCommandRunner{}

	app := newTestApp(composerFile, false, false, mockParser, mockAPI, mockCmd)
	err := app.Run(context.Background())
	require.NoError(t, err)

	content, _ := os.ReadFile(composerFile)
	assert.Contains(t, string(content), "2.1.4")
	assert.NotContains(t, string(content), "^2.1.0")

	require.Len(t, mockCmd.Calls, 1)
	assert.Equal(t, "composer", mockCmd.Calls[0].Name)
	assert.Contains(t, mockCmd.Calls[0].Args, "--with-dependencies")
	assert.Contains(t, mockCmd.Calls[0].Args, "vendor/pkg")
	assert.True(t, len(mockCmd.Calls[0].Env) > 0)
	assert.True(t, strings.HasPrefix(mockCmd.Calls[0].Env[0], "COMPOSER_AUTH="))
}

func TestComposerApp_Run_ApplyPatches_WithAlias(t *testing.T) {
	tmpDir := t.TempDir()
	composerFile := filepath.Join(tmpDir, "composer.json")
	require.NoError(t, os.WriteFile(composerFile, []byte(`{"require":{"vendor/pkg":"^2.1.0"}}`), 0644))

	mockAPI := &MockAPIClient{
		AnalyzePackagesFunc: func(_ context.Context, _ []rootio.Package, _ []rootio.Package, _ string) (*rootio.AnalyzePackagesResponse, error) {
			return &rootio.AnalyzePackagesResponse{
				Patches: []rootio.PackagePatch{
					{
						PackageName: "vendor/pkg",
						Version:     "2.1.0",
						Patch:       rootio.PatchInfo{Name: "vendor/pkg", Version: "2.1.4"},
						PatchAlias:  rootio.PatchInfo{Name: "rootio/vendor-pkg", Version: "2.1.4-rootio"},
						CVEIDs:      []string{"CVE-2024-1234"},
					},
				},
			}, nil
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	realParser := NewParser(logger, "https://pkg.root.io")
	mockParser := &MockComposerParser{
		ComposerParser: realParser,
		ParseFunc: func(_ context.Context, _ string) ([]common.PackageInfo, error) {
			return []common.PackageInfo{{Name: "vendor/pkg", Version: "2.1.0"}}, nil
		},
	}
	mockCmd := &MockCommandRunner{}

	app := newTestApp(composerFile, false, true, mockParser, mockAPI, mockCmd)
	err := app.Run(context.Background())
	require.NoError(t, err)

	content, _ := os.ReadFile(composerFile)
	assert.Contains(t, string(content), "rootio/vendor-pkg")
	assert.Contains(t, string(content), "2.1.4-rootio")
	assert.NotContains(t, string(content), `"vendor/pkg"`)

	require.Len(t, mockCmd.Calls, 1)
	assert.Contains(t, mockCmd.Calls[0].Args, "rootio/vendor-pkg")
}

// MockComposerParser uses the real Update/Validate but allows mocking Parse.
type MockComposerParser struct {
	*ComposerParser
	ParseFunc func(ctx context.Context, filePath string) ([]common.PackageInfo, error)
}

func (m *MockComposerParser) Parse(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
	if m.ParseFunc != nil {
		return m.ParseFunc(ctx, filePath)
	}
	return []common.PackageInfo{}, nil
}
