package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os/exec"

	"rootio_patcher/pkg/rootio"
)

// PipExecutor handles pip operations for patching packages
type PipExecutor struct {
	pythonPath string
	pkgURL     string
	apiKey     string
	useAlias   bool
	logger     *slog.Logger
}

// NewPipExecutor creates a new pip executor
func NewPipExecutor(pythonPath, pkgURL, apiKey string, useAlias bool, logger *slog.Logger) *PipExecutor {
	return &PipExecutor{
		pythonPath: pythonPath,
		pkgURL:     pkgURL,
		apiKey:     apiKey,
		useAlias:   useAlias,
		logger:     logger,
	}
}

// ApplyPatch applies a single package patch (uninstall + install)
func (e *PipExecutor) ApplyPatch(ctx context.Context, patch rootio.PackagePatch) error {
	// 1. Uninstall vulnerable package (original name)
	e.logger.DebugContext(ctx, "Uninstalling package",
		slog.String("package", patch.PackageName))
	//nolint:gosec // Subprocess command is safe - using validated package names from our API
	uninstallCmd := exec.CommandContext(ctx, e.pythonPath, "-m", "pip", "uninstall", "-y", patch.PackageName)
	if output, err := uninstallCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("uninstall failed: %w (output: %s)", err, string(output))
	}

	// 2. Select patch based on useAlias flag
	var patchInfo rootio.PatchInfo
	if e.useAlias {
		patchInfo = patch.PatchAlias
	} else {
		patchInfo = patch.Patch
	}

	// 3. Install patched package with --no-deps using index-url
	e.logger.DebugContext(ctx, "Installing patched package",
		slog.String("package_name", patchInfo.Name),
		slog.String("version", patchInfo.Version),
		slog.Bool("use_alias", e.useAlias))

	// Construct authenticated index URL: https://root:{api_key}@<host>/pypi/simple/
	indexURL := e.constructIndexURL()

	// Package specification: package==version (pip handles normalization internally)
	packageSpec := fmt.Sprintf("%s==%s", patchInfo.Name, patchInfo.Version)

	//nolint:gosec // Subprocess command is safe - using package names from our API
	installCmd := exec.CommandContext(
		ctx, e.pythonPath, "-m", "pip", "install",
		"--no-deps",
		"--no-cache-dir",
		"--index-url", indexURL,
		packageSpec,
	)

	if output, err := installCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("install failed: %w (output: %s)", err, string(output))
	}

	return nil
}

// constructIndexURL builds the authenticated PyPI index URL
// Returns: https://root:<api_key>@<host>/pypi/simple/
func (e *PipExecutor) constructIndexURL() string {
	// Parse the package URL to extract the host
	parsedURL, err := url.Parse(e.pkgURL)
	if err != nil {
		// Fallback to using pkgURL as-is if parsing fails
		e.logger.Warn("Failed to parse PKGURL, using as-is", slog.String("url", e.pkgURL), slog.Any("error", err))
		return e.pkgURL
	}

	indexURL := fmt.Sprintf("%s://root:%s@%s/pypi/simple/",
		parsedURL.Scheme,
		url.QueryEscape(e.apiKey),
		parsedURL.Host)

	e.logger.Debug("Constructed index URL", slog.String("host", parsedURL.Host))
	return indexURL
}
