package apt

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"rootio_patcher/cmd/rootio_patcher/common"
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

// InstalledPackage is an alias for common.InstalledPackage
type InstalledPackage = common.InstalledPackage

// SystemProbe is an alias for common.SystemProbe
type SystemProbe = common.SystemProbe

type realProbe struct{}

func (r *realProbe) ReadOSRelease(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", `. /etc/os-release && echo "$ID $VERSION_ID $VERSION_CODENAME"`)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to read /etc/os-release: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (r *realProbe) QueryInstalledPackages(ctx context.Context) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "dpkg-query",
		"--show",
		"--showformat=${Package}\t${Version}\n",
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("dpkg-query failed: %w", err)
	}
	return out, nil
}

// Scanner reads OS info and installed packages from the running system
type Scanner = common.Scanner[OSInfo]

func NewScanner() Scanner { return NewScannerWithProbe(&realProbe{}) }

func NewScannerWithProbe(probe SystemProbe) Scanner {
	return &common.OsScanner[OSInfo]{
		Probe:         probe,
		ParseRelease:  parseOSRelease,
		ParsePackages: parseDpkgQueryOutput,
	}
}

func parseOSRelease(line string) (*OSInfo, error) {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return nil, fmt.Errorf("unexpected /etc/os-release output: %q", line)
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
	case "ubuntu":
		ecosystem = "ubuntu"
	default:
		return nil, fmt.Errorf("unsupported OS %q: apt remediate supports debian and ubuntu only", id)
	}

	return &OSInfo{
		Ecosystem:     ecosystem,
		DistroVersion: versionID,
		Codename:      codename,
	}, nil
}

func parseDpkgQueryOutput(out []byte) ([]InstalledPackage, error) {
	var packages []InstalledPackage
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			continue
		}
		packages = append(packages, InstalledPackage{
			Name:    parts[0],
			Version: parts[1],
		})
	}
	return packages, sc.Err()
}
