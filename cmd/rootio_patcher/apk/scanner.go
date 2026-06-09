package apk

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"rootio_patcher/cmd/rootio_patcher/common"
)

// OSInfo holds the detected Alpine OS version
type OSInfo struct {
	// DistroVersion is the major.minor version (e.g. "3.18")
	DistroVersion string
}

// InstalledPackage is an alias for common.InstalledPackage
type InstalledPackage = common.InstalledPackage

// SystemProbe is an alias for common.SystemProbe
type SystemProbe = common.SystemProbe

type realProbe struct {
	fs fileSystem
}

func (r *realProbe) ReadOSRelease(_ context.Context) (string, error) {
	data, err := r.fs.ReadFile("/etc/alpine-release")
	if err != nil {
		return "", fmt.Errorf("failed to read /etc/alpine-release: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

func (r *realProbe) QueryInstalledPackages(ctx context.Context) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "apk", "info", "-v")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("apk info -v failed: %w", err)
	}
	return out, nil
}

// Scanner reads OS info and installed packages from the running system
type Scanner = common.Scanner[OSInfo]

func NewScanner() Scanner { return NewScannerWithProbe(&realProbe{fs: realFS{}}) }

func NewScannerWithProbe(probe SystemProbe) Scanner {
	return &common.OsScanner[OSInfo]{
		Probe:         probe,
		ParseRelease:  parseAlpineRelease,
		ParsePackages: parseApkInfoOutput,
	}
}

func parseAlpineRelease(line string) (*OSInfo, error) {
	parts := strings.SplitN(line, ".", 3)
	if len(parts) < 2 {
		return nil, fmt.Errorf("unexpected /etc/alpine-release content: %q", line)
	}
	return &OSInfo{DistroVersion: parts[0] + "." + parts[1]}, nil
}

// parseApkInfoOutput parses `apk info -v` output.
// Each line is "<name>-<version>" where version starts after the last hyphen
// preceded by a digit: e.g. "curl-8.5.0-r0", "ca-certificates-20240226-r0".
func parseApkInfoOutput(out []byte) ([]InstalledPackage, error) {
	var packages []InstalledPackage
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		name, version, ok := splitApkNameVersion(line)
		if !ok {
			continue
		}
		packages = append(packages, InstalledPackage{Name: name, Version: version})
	}
	return packages, sc.Err()
}

// splitApkNameVersion splits "curl-8.5.0-r0" → ("curl", "8.5.0").
// Splits on "-", finds the first segment that starts with a digit — that marks
// the start of the version. Mirrors the logic in the backend alpine package manager.
// The Alpine revision suffix (-rN) is stripped because the API stores only the
// upstream version (e.g. "8.5.0", not "8.5.0-r0").
func splitApkNameVersion(s string) (name, version string, ok bool) {
	segments := strings.Split(s, "-")
	for i, seg := range segments {
		if len(seg) > 0 && seg[0] >= '0' && seg[0] <= '9' {
			ver := strings.Join(segments[i:], "-")
			return strings.Join(segments[:i], "-"), stripAlpineRevision(ver), true
		}
	}
	return "", "", false
}

// stripAlpineRevision removes the trailing Alpine build revision suffix (-rN)
// from a version string, e.g. "8.5.0-r0" → "8.5.0", "643-r2" → "643".
// The API stores versions without this suffix.
func stripAlpineRevision(ver string) string {
	if idx := strings.LastIndex(ver, "-r"); idx >= 0 {
		suffix := ver[idx+2:]
		allDigits := len(suffix) > 0
		for _, c := range suffix {
			if c < '0' || c > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			return ver[:idx]
		}
	}
	return ver
}
