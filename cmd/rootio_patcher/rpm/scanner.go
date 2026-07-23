package rpm

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"rootio_patcher/cmd/rootio_patcher/common"
)

// InstalledPackage is an alias for common.InstalledPackage
type InstalledPackage = common.InstalledPackage

// Scanner lists packages installed on the running system
type Scanner interface {
	ListPackages(ctx context.Context) ([]InstalledPackage, error)
}

type realScanner struct{}

func NewScanner() Scanner { return &realScanner{} }

func (r *realScanner) ListPackages(ctx context.Context) ([]InstalledPackage, error) {
	cmd := exec.CommandContext(ctx, "rpm", "-qa", "--queryformat", "%{NAME}\t%{VERSION}-%{RELEASE}\n")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("rpm -qa failed: %w", err)
	}
	return parseRpmQaOutput(out)
}

// parseRpmQaOutput parses `rpm -qa --queryformat '%{NAME}\t%{VERSION}-%{RELEASE}\n'`
// output into installed packages. rpm is used for listing regardless of which
// front-end (yum/dnf/microdnf) manages installs, since all three store
// packages in the same RPM database.
func parseRpmQaOutput(out []byte) ([]InstalledPackage, error) {
	var packages []InstalledPackage
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		packages = append(packages, InstalledPackage{Name: parts[0], Version: parts[1]})
	}
	return packages, sc.Err()
}
