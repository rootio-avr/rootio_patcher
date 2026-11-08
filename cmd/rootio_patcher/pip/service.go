package pip

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/pkg/rootio"
)

// Service defines the interface for pip operations
type Service interface {
	// ListPackages lists all installed packages
	ListPackages(ctx context.Context) ([]common.InstalledPackage, error)
	
	// ApplyPatch applies a patch to a package (uninstall + install)
	ApplyPatch(ctx context.Context, patch rootio.PackagePatch, breakSystem bool) error
	
	// ApplyPatchForPip applies a patch specifically for pip itself
	ApplyPatchForPip(ctx context.Context, patch rootio.PackagePatch, breakSystem bool) error
}

// PipService implements Service for pip operations
type PipService struct {
	pythonPath  string
	pkgURL      string
	apiKey      string
	pipIndexURL string // overrides constructed index URL when non-empty
	logger      *slog.Logger
}

// NewService creates a new pip service
func NewService(pythonPath, pkgURL, apiKey, pipIndexURL string, logger *slog.Logger) *PipService {
	return &PipService{
		pythonPath:  pythonPath,
		pkgURL:      pkgURL,
		apiKey:      apiKey,
		pipIndexURL: pipIndexURL,
		logger:      logger,
	}
}

// ListPackages collects all installed packages using pip list
func (s *PipService) ListPackages(ctx context.Context) ([]common.InstalledPackage, error) {
	s.logger.DebugContext(ctx, "Using Python executable", slog.String("path", s.pythonPath))

	// -v adds each package's location and installer
	output, err := pipCmd(ctx, s.pythonPath, false, "list", "-v", "--format=json").Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run pip list: %w", err)
	}

	// Decode only the first JSON value: some pips print notices to stdout after it.
	var packages []common.InstalledPackage
	if err := json.NewDecoder(bytes.NewReader(output)).Decode(&packages); err != nil {
		return nil, fmt.Errorf("failed to parse pip list output: %w", err)
	}

	return packages, nil
}

// ApplyPatch applies a single package patch (uninstall + install)
// breakSystem sets PIP_BREAK_SYSTEM_PACKAGES=1 (PEP 668); only pass true for a pip-owned package.
func (s *PipService) ApplyPatch(ctx context.Context, patch rootio.PackagePatch, breakSystem bool) error {
	// 1. Uninstall vulnerable package (original name)
	s.logger.DebugContext(ctx, "Uninstalling package", slog.String("package", patch.PackageName))
	uninstallCmd := pipCmd(ctx, s.pythonPath, breakSystem, "uninstall", "-y", patch.PackageName)
	if output, err := uninstallCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("uninstall failed: %w (output: %s)", err, string(output))
	}

	patchInfo := patch.Patch

	s.logger.DebugContext(ctx, "Installing patched package",
		slog.String("package_name", patchInfo.Name),
		slog.String("version", patchInfo.Version))

	indexURL := s.constructIndexURL()
	packageSpec := fmt.Sprintf("%s==%s", patchInfo.Name, patchInfo.Version)

	installCmd := pipCmd(
		ctx, s.pythonPath, breakSystem, "install",
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
func (s *PipService) ApplyPatchForPip(ctx context.Context, patch rootio.PackagePatch, breakSystem bool) error {
	patchInfo := patch.Patch

	s.logger.DebugContext(ctx, "Upgrading pip package",
		slog.String("version", patchInfo.Version))

	indexURL := s.constructIndexURL()
	packageSpec := fmt.Sprintf("%s==%s", patchInfo.Name, patchInfo.Version)

	installCmd := pipCmd(
		ctx, s.pythonPath, breakSystem, "install",
		"--no-deps",
		"--no-cache-dir",
		"--upgrade",
		"--index-url", indexURL,
		packageSpec,
	)

	if output, err := installCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("pip upgrade failed: %w (output: %s)", err, string(output))
	}

	// --upgrade can't uninstall pip mid-run, so the old pip-<ver>.dist-info stays on
	// disk. Trivy reads dist-info to detect versions, causing a false CVE flag. Remove
	// it now that rootio_pip's dist-info is safely written. Warn-and-continue: cleanup
	// failure is a scanner annoyance, not a reason to roll back a successful patch.
	if err := s.removeLegacyDistInfo(ctx, patch.PackageName); err != nil {
		s.logger.WarnContext(ctx, "could not remove legacy dist-info (scanner may still flag pip CVEs)",
			slog.String("package", patch.PackageName), slog.Any("err", err))
	}

	return nil
}

// removeLegacyDistInfo deletes <site-packages>/<pkg>-*.dist-info directories left on disk
// after a pip --upgrade (uninstalling pip mid-run is not possible).
func (s *PipService) removeLegacyDistInfo(ctx context.Context, packageName string) error {
	//nolint:gosec // safe: fixed args, pythonPath from config
	out, err := exec.CommandContext(ctx, s.pythonPath, "-c",
		"import sysconfig; print(sysconfig.get_paths()['purelib'])").Output()
	if err != nil {
		return fmt.Errorf("locate site-packages: %w", err)
	}
	sitePackages := strings.TrimSpace(string(out))

	// filepath.Base ensures a malformed package name can't escape sitePackages.
	pattern := filepath.Join(sitePackages, filepath.Base(packageName)+"-*.dist-info")
	matches, _ := filepath.Glob(pattern) // pattern is machine-constructed; ErrBadPattern is impossible

	for _, dir := range matches {
		s.logger.DebugContext(ctx, "removing legacy dist-info", slog.String("dir", dir))
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("remove %s: %w", dir, err)
		}
	}
	return nil
}

// constructIndexURL builds the authenticated PyPI index URL.
// If ROOTIO_PIP_INDEX_URL is set it is returned as-is.
func (s *PipService) constructIndexURL() string {
	if s.pipIndexURL != "" {
		return s.pipIndexURL
	}

	parsedURL, err := url.Parse(s.pkgURL)
	if err != nil {
		s.logger.Warn("Failed to parse package URL, using as-is", slog.String("url", s.pkgURL))
		return s.pkgURL + "/pypi/simple/"
	}

	parsedURL.User = url.UserPassword("root", s.apiKey)
	parsedURL.Path = "/pypi/simple/"

	return parsedURL.String()
}

// pipCmd runs "python -m pip <args>" without the self-version check (it prints to stdout).
// breakSystem lets pip modify a PEP 668 (externally managed) Python; older pip ignores it.
func pipCmd(ctx context.Context, pythonPath string, breakSystem bool, args ...string) *exec.Cmd {
	//nolint:gosec // safe: pythonPath from config, package names from our API
	cmd := exec.CommandContext(ctx, pythonPath, append([]string{"-m", "pip", "--disable-pip-version-check"}, args...)...)
	if breakSystem {
		cmd.Env = append(os.Environ(), "PIP_BREAK_SYSTEM_PACKAGES=1")
	}
	return cmd
}

// GetArch returns the system architecture
func GetArch() string {
	return runtime.GOARCH
}
