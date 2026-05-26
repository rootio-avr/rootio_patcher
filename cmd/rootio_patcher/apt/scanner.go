package apt

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// OSInfo holds the detected OS ecosystem and distro version
type OSInfo struct {
	// Ecosystem is "debian" or "ubuntu"
	Ecosystem string
	// DistroVersion is the major version (e.g. "12" for Debian 12, "22.04" for Ubuntu 22.04)
	DistroVersion string
	// Codename is the release codename (e.g. "bookworm", "jammy")
	Codename string
}

// InstalledPackage is a dpkg-installed package name+version
type InstalledPackage struct {
	Name    string
	Version string
}

// Scanner reads OS info and installed packages from the running system
type Scanner interface {
	DetectOS(ctx context.Context) (*OSInfo, error)
	ListPackages(ctx context.Context) ([]InstalledPackage, error)
}

type realScanner struct{}

func NewScanner() Scanner { return &realScanner{} }

// DetectOS reads /etc/os-release to determine ecosystem and version
func (s *realScanner) DetectOS(ctx context.Context) (*OSInfo, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", ". /etc/os-release && echo \"$ID $VERSION_ID $VERSION_CODENAME\"")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to read /etc/os-release: %w", err)
	}

	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) < 2 {
		return nil, fmt.Errorf("unexpected /etc/os-release output: %q", string(out))
	}

	id := strings.ToLower(parts[0])
	versionID := parts[1]
	codename := ""
	if len(parts) >= 3 {
		codename = parts[2]
	}

	var ecosystem string
	switch id {
	case "debian":
		ecosystem = "debian"
		// VERSION_ID for debian is e.g. "12"
	case "ubuntu":
		ecosystem = "ubuntu"
		// VERSION_ID for ubuntu is e.g. "22.04"
	default:
		return nil, fmt.Errorf("unsupported OS %q: apt remediate supports debian and ubuntu only", id)
	}

	return &OSInfo{
		Ecosystem:     ecosystem,
		DistroVersion: versionID,
		Codename:      codename,
	}, nil
}

// ListPackages returns all packages installed via dpkg
func (s *realScanner) ListPackages(ctx context.Context) ([]InstalledPackage, error) {
	cmd := exec.CommandContext(ctx, "dpkg-query",
		"--show",
		"--showformat=${Package}\t${Version}\n",
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("dpkg-query failed: %w", err)
	}

	var packages []InstalledPackage
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			continue
		}
		packages = append(packages, InstalledPackage{
			Name:    parts[0],
			Version: parts[1],
		})
	}

	return packages, scanner.Err()
}
