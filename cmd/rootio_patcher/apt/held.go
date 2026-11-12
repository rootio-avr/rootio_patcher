package apt

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"sort"
	"strings"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/pkg/rootio"
)

// OutputRunner is a CommandRunner that can also capture a command's combined
// output. The apt executor uses it for read-only queries (apt-mark showhold,
// apt-get -s). A runner that does not implement it disables held-package
// handling, so the install path stays exactly as before.
type OutputRunner interface {
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
}

// aptRunner adds Output to the shared streaming runner.
type aptRunner struct{ common.CommandRunner }

func (aptRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// heldPackages returns the names held with apt-mark hold. Any error (no
// OutputRunner, apt-mark missing or failing) yields an empty set, which keeps
// today's behaviour.
func (e *Executor) heldPackages(ctx context.Context) map[string]bool {
	or, ok := e.runner.(OutputRunner)
	if !ok {
		return nil
	}
	out, err := or.Output(ctx, "apt-mark", "showhold")
	if err != nil {
		e.logf("apt-mark showhold failed, not checking holds: %v", err)
		return nil
	}
	held := map[string]bool{}
	for _, f := range strings.Fields(string(out)) {
		held[stripArch(f)] = true
	}
	return held
}

// excludeHeld returns the subset of names whose install leaves every held
// package untouched. A name that is itself held is dropped; the rest are
// simulated with `apt-get -s <installArgs> <names>`, and if the transaction
// would change a held package, each name is simulated alone to find the ones
// responsible. Every skip is logged at WARN. With no held packages it returns names
// unchanged and runs nothing.
// versions maps a name to the version being skipped, for the WARN record and
// the HELD_SKIPPED summary; a name missing from it is a distro upgrade.
func (e *Executor) excludeHeld(ctx context.Context, held map[string]bool, installArgs, names []string, versions map[string]string) []string {
	if len(held) == 0 || len(names) == 0 {
		return names
	}
	or := e.runner.(OutputRunner) // held is only non-empty when the runner is an OutputRunner

	var candidates []string
	for _, n := range names {
		if held[stripArch(n)] {
			e.skip(ctx, n, versions[n], "held by the base image")
			continue
		}
		candidates = append(candidates, n)
	}
	if len(candidates) == 0 {
		return nil
	}

	simulate := func(ns []string) []string {
		args := append(append([]string{"-s"}, installArgs...), ns...)
		out, _ := or.Output(ctx, "apt-get", args...) // a real resolver error surfaces in the real install
		return heldChanges(out, held)
	}
	if len(simulate(candidates)) == 0 {
		return candidates
	}

	var keep []string
	for _, n := range candidates {
		if hit := simulate([]string{n}); len(hit) > 0 {
			e.skip(ctx, n, versions[n], "would change held package "+strings.Join(hit, " "))
			continue
		}
		keep = append(keep, n)
	}
	return keep
}

// dropHeldPatches removes patches that target a held package or whose install
// would change one, so the remaining patches install in one transaction that
// apt accepts. With no held packages it returns patches unchanged.
func (e *Executor) dropHeldPatches(ctx context.Context, patches []rootio.PackagePatch, useAlias bool) []rootio.PackagePatch {
	held := e.heldPackages(ctx)
	if len(held) == 0 {
		return patches
	}
	byName := map[string]rootio.PackagePatch{}
	versions := map[string]string{}
	var names []string
	for _, p := range patches {
		if held[p.PackageName] {
			e.skip(ctx, p.PackageName, p.Patch.Version, "held by the base image")
			continue
		}
		n := p.Patch.Name
		if useAlias {
			n = p.PatchAlias.Name
		}
		byName[n] = p
		versions[n] = p.Patch.Version
		names = append(names, n)
	}
	installArgs := []string{"-o", "Dpkg::Options::=--force-overwrite", "install", "--allow-downgrades"}
	if useAlias {
		installArgs = []string{"-o", "Dpkg::Options::=--force-overwrite", "install", "--allow-remove-essential", "--no-install-recommends"}
	}
	var keep []rootio.PackagePatch
	for _, n := range e.excludeHeld(ctx, held, installArgs, names, versions) {
		keep = append(keep, byName[n])
	}
	return keep
}

// HeldSkip is one patch or upgrade left out because of a held package.
type HeldSkip struct{ Package, Version, Reason string }

// skip logs a held-package skip at WARN and records it for HeldSummary.
func (e *Executor) skip(ctx context.Context, pkg, version, reason string) {
	if version == "" {
		version = "upgrade"
	}
	e.logger.WarnContext(ctx, "skipped patch: held package",
		slog.String("ecosystem", "apt"),
		slog.String("package", pkg),
		slog.String("version", version),
		slog.String("reason", reason),
	)
	e.skipped = append(e.skipped, HeldSkip{pkg, version, reason})
}

// HeldSummary returns the machine-readable summary line for skipped held
// packages ("HELD_SKIPPED <n>: <pkg>=<version>,..."), or "" when none were.
func (e *Executor) HeldSummary() string {
	if len(e.skipped) == 0 {
		return ""
	}
	parts := make([]string, len(e.skipped))
	for i, s := range e.skipped {
		parts[i] = s.Package + "=" + s.Version
	}
	return fmt.Sprintf("HELD_SKIPPED %d: %s", len(e.skipped), strings.Join(parts, ","))
}

// heldChanges parses apt-get -s output and returns the held packages the
// transaction would change: names under "The following held packages will be
// changed:" plus any Inst/Remv/Purg line for a held name.
func heldChanges(out []byte, held map[string]bool) []string {
	hit := map[string]bool{}
	inBlock := false
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "The following held packages will be changed") {
			inBlock = true
			continue
		}
		if inBlock && strings.HasPrefix(line, " ") {
			for _, f := range strings.Fields(line) {
				hit[stripArch(f)] = true
			}
			continue
		}
		inBlock = false
		f := strings.Fields(line)
		if len(f) >= 2 && (f[0] == "Inst" || f[0] == "Remv" || f[0] == "Purg") && held[stripArch(f[1])] {
			hit[stripArch(f[1])] = true
		}
	}
	res := make([]string, 0, len(hit))
	for n := range hit {
		res = append(res, n)
	}
	sort.Strings(res)
	return res
}

func stripArch(name string) string {
	if i := strings.IndexByte(name, ':'); i >= 0 {
		return name[:i]
	}
	return name
}
