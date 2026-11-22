package apt

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"rootio_patcher/pkg/rootio"
)

// fipsMarker is in the version of every Root FIPS deb build.
const fipsMarker = "aikido.fips."

func isFips(version string) bool { return strings.Contains(version, fipsMarker) }

// fipsPackages returns the names of installed packages whose version is a FIPS build. Like
// heldPackages, any error (no OutputRunner, dpkg-query failing) yields an empty set.
func (e *Executor) fipsPackages(ctx context.Context) map[string]bool {
	or, ok := e.runner.(OutputRunner)
	if !ok {
		return nil
	}
	out, err := or.Output(ctx, "dpkg-query", "-W", "-f", "${Package} ${Version}\n")
	if err != nil {
		e.logf("dpkg-query failed, not checking FIPS packages: %v", err)
		return nil
	}
	fips := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		if f := strings.Fields(line); len(f) == 2 && isFips(f[1]) {
			fips[stripArch(f[0])] = true
		}
	}
	return fips
}

// excludeFips drops installed FIPS packages from a distro upgrade, which would replace them with a
// non-FIPS build.
func (e *Executor) excludeFips(ctx context.Context, names []string) []string {
	fips := e.fipsPackages(ctx)
	if len(fips) == 0 {
		return names
	}
	var keep []string
	for _, n := range names {
		if !fips[stripArch(n)] {
			keep = append(keep, n)
		}
	}
	return keep
}

// dropFipsLosses drops patches that would replace an installed FIPS package with a non-FIPS
// version. A newer FIPS patch stays.
func (e *Executor) dropFipsLosses(ctx context.Context, patches []rootio.PackagePatch) []rootio.PackagePatch {
	fips := e.fipsPackages(ctx)
	if len(fips) == 0 {
		return patches
	}
	var keep []rootio.PackagePatch
	for _, p := range patches {
		if fips[p.PackageName] && !isFips(p.Patch.Version) {
			e.logger.WarnContext(ctx, "skipped patch: installed FIPS package",
				slog.String("ecosystem", "apt"),
				slog.String("package", p.PackageName),
				slog.String("version", p.Patch.Version),
			)
			e.kept = append(e.kept, HeldSkip{p.PackageName, p.Patch.Version, "installed FIPS build kept"})
			continue
		}
		keep = append(keep, p)
	}
	return keep
}

// FipsSummary returns the machine-readable summary line for patches left out to keep a FIPS
// build ("FIPS_KEPT <n>: <pkg>=<version>,..."), or "" when none were.
func (e *Executor) FipsSummary() string {
	if len(e.kept) == 0 {
		return ""
	}
	parts := make([]string, len(e.kept))
	for i, k := range e.kept {
		parts[i] = k.Package + "=" + k.Version
	}
	return fmt.Sprintf("FIPS_KEPT %d: %s", len(e.kept), strings.Join(parts, ","))
}

// KeepFips pins every installed FIPS package away from the "main" components (Debian and Root's
// regular builds), so no later apt step can replace it with a non-FIPS build. The Root "fips"
// component stays allowed, so a newer FIPS build still installs. The file stays in the image.
// Nothing is written when no FIPS package is installed.
func (e *Executor) KeepFips(ctx context.Context) error {
	var names []string
	for n := range e.fipsPackages(ctx) {
		names = append(names, n)
	}
	if len(names) == 0 {
		return nil
	}
	sort.Strings(names)
	if err := e.runner.Run(ctx, "mkdir", "-p", prefsDir); err != nil {
		return err
	}
	script := fmt.Sprintf(`printf 'Package: %s\nPin: release c=main\nPin-Priority: -1\n' > %s/rootio-fips-keep`,
		strings.Join(names, " "), prefsDir)
	return e.runner.Run(ctx, "sh", "-c", script)
}
