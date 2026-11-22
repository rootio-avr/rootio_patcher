package apk

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"sort"
	"strings"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/pkg/rootio"
)

// fipsRevision matches the revision of a Root FIPS apk build: -r<pkgrel>008<N>, the sibling of the
// plain Root build -r<pkgrel>007<N>.
var fipsRevision = regexp.MustCompile(`-r\d+008\d+`)

// nameVersion splits an `apk info -v` entry, e.g. "curl-8.5.0-r0", at the first "-<digit>".
var nameVersion = regexp.MustCompile(`^(.+?)-(\d.*)$`)

func isFips(version string) bool { return fipsRevision.MatchString(version) }

// outputRunner is a CommandRunner that can also capture a command's output, for read-only queries.
// A runner without it turns FIPS handling off.
type outputRunner interface {
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
}

// apkRunner adds Output to the shared streaming runner.
type apkRunner struct{ common.CommandRunner }

func (apkRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// fipsPackages returns installed FIPS packages (name to full version, revision included) from
// `apk info -v`. Any error yields an empty set.
func (e *Executor) fipsPackages(ctx context.Context) map[string]string {
	or, ok := e.runner.(outputRunner)
	if !ok {
		return nil
	}
	out, err := or.Output(ctx, "apk", "info", "-v")
	if err != nil {
		e.logf("apk info -v failed, not checking FIPS packages", "error", err)
		return nil
	}
	fips := map[string]string{}
	for _, f := range strings.Fields(string(out)) {
		if m := nameVersion.FindStringSubmatch(f); m != nil && isFips(m[2]) {
			fips[m[1]] = m[2]
		}
	}
	return fips
}

// excludeFips drops installed FIPS packages from a distro upgrade, which would replace them with a
// stock build.
func (e *Executor) excludeFips(ctx context.Context, names []string) []string {
	fips := e.fipsPackages(ctx)
	if len(fips) == 0 {
		return names
	}
	var keep []string
	for _, n := range names {
		if _, ok := fips[n]; !ok {
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
		if _, ok := fips[p.PackageName]; ok && !isFips(p.Patch.Version) {
			e.logger.WarnContext(ctx, "skipped patch: installed FIPS package",
				slog.String("ecosystem", "apk"),
				slog.String("package", p.PackageName),
				slog.String("version", p.Patch.Version),
			)
			e.kept = append(e.kept, p.PackageName+"="+p.Patch.Version)
			continue
		}
		keep = append(keep, p)
	}
	return keep
}

// fipsSummary returns the machine-readable summary line for patches left out to keep a FIPS build
// ("FIPS_KEPT <n>: <pkg>=<version>,..."), or "" when none were.
func (e *Executor) fipsSummary() string {
	if len(e.kept) == 0 {
		return ""
	}
	return fmt.Sprintf("FIPS_KEPT %d: %s", len(e.kept), strings.Join(e.kept, ","))
}

// keepFips pins every installed FIPS package to its installed version in /etc/apk/world, so a later
// `apk upgrade` cannot replace it with a stock build. An explicit `apk add <name>` (a newer FIPS
// patch) still replaces the pin. Nothing is written when no FIPS package is installed.
func (e *Executor) keepFips(ctx context.Context) error {
	fips := e.fipsPackages(ctx)
	if len(fips) == 0 {
		return nil
	}
	var pins []string
	for name, version := range fips {
		pins = append(pins, name+"="+version)
	}
	sort.Strings(pins)
	return e.runner.Run(ctx, "apk", append([]string{"add"}, pins...)...)
}
