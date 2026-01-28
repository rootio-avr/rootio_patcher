package pip

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os/exec"
	"runtime"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/pkg/rootio"
)

// Service defines the interface for pip operations
type Service interface {
	// ListPackages lists all installed packages
	ListPackages(ctx context.Context) ([]common.InstalledPackage, error)
	
	// ApplyPatch applies a patch to a package (uninstall + install)
	ApplyPatch(ctx context.Context, patch rootio.PackagePatch) error
	
	// ApplyPatchForPip applies a patch specifically for pip itself
	ApplyPatchForPip(ctx context.Context, patch rootio.PackagePatch) error
}

// PipService implements Service for pip operations
type PipService struct {
	pythonPath string
	pkgURL     string
	apiKey     string
	useAlias   bool
	logger     *slog.Logger
}

// NewService creates a new pip service
func NewService(pythonPath, pkgURL, apiKey string, useAlias bool, logger *slog.Logger) *PipService {
	return &PipService{
		pythonPath: pythonPath,
		pkgURL:     pkgURL,
		apiKey:     apiKey,
		useAlias:   useAlias,
		logger:     logger,
	}
}

// ListPackages collects all installed packages using pip list
func (s *PipService) ListPackages(ctx context.Context) ([]common.InstalledPackage, error) {
	s.logger.DebugContext(ctx, "Using Python executable", slog.String("path", s.pythonPath))

	// Run: python -m pip list --format=json
	//nolint:gosec // Subprocess is safe - using fixed pip list arguments, pythonPath from config
	cmd := exec.CommandContext(ctx, s.pythonPath, "-m", "pip", "list", "--format=json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run pip list: %w", err)
	}

	// Parse JSON output
	var packages []common.InstalledPackage
	if err := json.Unmarshal(output, &packages); err != nil {
		return nil, fmt.Errorf("failed to parse pip list output: %w", err)
	}

	return packages, nil
}

// ApplyPatch applies a single package patch (uninstall + install)
func (s *PipService) ApplyPatch(ctx context.Context, patch rootio.PackagePatch) error {
	// 1. Uninstall vulnerable package (original name)
	s.logger.DebugContext(ctx, "Uninstalling package", slog.String("package", patch.PackageName))
	//nolint:gosec // Subprocess command is safe - using validated package names from our API
	uninstallCmd := exec.CommandContext(ctx, s.pythonPath, "-m", "pip", "uninstall", "-y", patch.PackageName)
	if output, err := uninstallCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("uninstall failed: %w (output: %s)", err, string(output))
	}

	// 2. Select patch based on useAlias flag
	var patchInfo rootio.PatchInfo
	if s.useAlias {
		patchInfo = patch.PatchAlias
	} else {
		patchInfo = patch.Patch
	}

	// 3. Install patched package
	s.logger.DebugContext(ctx, "Installing patched package",
		slog.String("package_name", patchInfo.Name),
		slog.String("version", patchInfo.Version),
		slog.Bool("use_alias", s.useAlias))

	indexURL := s.constructIndexURL()
	packageSpec := fmt.Sprintf("%s==%s", patchInfo.Name, patchInfo.Version)

	//nolint:gosec // Subprocess command is safe - using package names from our API
	installCmd := exec.CommandContext(
		ctx, s.pythonPath, "-m", "pip", "install",
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

// ApplyPatchForPip applies a patch for pip itself using upgrade
func (s *PipService) ApplyPatchForPip(ctx context.Context, patch rootio.PackagePatch) error {
	var patchInfo rootio.PatchInfo
	if s.useAlias {
		patchInfo = patch.PatchAlias
	} else {
		patchInfo = patch.Patch
	}

	s.logger.DebugContext(ctx, "Upgrading pip package",
		slog.String("version", patchInfo.Version))

	indexURL := s.constructIndexURL()
	packageSpec := fmt.Sprintf("%s==%s", patchInfo.Name, patchInfo.Version)

	//nolint:gosec // Subprocess command is safe - using package names from our API
	installCmd := exec.CommandContext(
		ctx, s.pythonPath, "-m", "pip", "install",
		"--no-deps",
		"--no-cache-dir",
		"--upgrade",
		"--index-url", indexURL,
		packageSpec,
	)

	if output, err := installCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("pip upgrade failed: %w (output: %s)", err, string(output))
	}

	return nil
}

// constructIndexURL builds the authenticated PyPI index URL
func (s *PipService) constructIndexURL() string {
	parsedURL, err := url.Parse(s.pkgURL)
	if err != nil {
		s.logger.Warn("Failed to parse package URL, using as-is", slog.String("url", s.pkgURL))
		return s.pkgURL + "/pypi/simple/"
	}

	parsedURL.User = url.UserPassword("root", s.apiKey)
	parsedURL.Path = "/pypi/simple/"

	return parsedURL.String()
}

// GetArch returns the system architecture
func GetArch() string {
	return runtime.GOARCH
}
