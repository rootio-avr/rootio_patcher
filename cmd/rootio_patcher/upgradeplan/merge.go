package upgradeplan

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/pkg/rootio"
)

// Resolve loads the plan for a project directory.
// An explicit path that cannot be read is an error. A missing default file
// (DefaultFileName in dir) returns a nil plan, which keeps the Root API lookup.
func Resolve(explicitPath, dir string) (*Plan, error) {
	path := explicitPath
	if path == "" {
		path = filepath.Join(dir, DefaultFileName)
	}
	plan, err := Load(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && explicitPath == "" {
			return nil, nil
		}
		return nil, fmt.Errorf("read upgrade plan %s: %w", path, err)
	}
	return plan, nil
}

// Analyze is the one analyze step every ecosystem app calls instead of AnalyzePackages.
// Installed packages that the plan upgrades become patches to the plan version. Every
// installed package goes to the Root API, with each planned package added to the ignore
// list as name@installed. The result holds the planned patches first, then the API patches.
// A nil plan sends the request to the API unchanged.
func Analyze(
	ctx context.Context,
	logger *slog.Logger,
	client common.APIClient,
	plan *Plan,
	ecosystem string,
	installed []rootio.Package,
	ignore []rootio.Package,
) (*rootio.AnalyzePackagesResponse, error) {
	var planned Selection
	if plan != nil {
		pkgs := make([]Package, len(installed))
		for i, pkg := range installed {
			pkgs[i] = Package{Name: pkg.Name, Version: pkg.Version}
		}
		var err error
		if planned, err = plan.Split(ecosystem, pkgs); err != nil {
			return nil, err
		}
		for _, name := range planned.Absent {
			logger.InfoContext(ctx, "Upgrade plan package is not installed, skipping",
				slog.String("package", name),
				slog.String("ecosystem", ecosystem))
		}
		for _, name := range planned.Current {
			logger.InfoContext(ctx, "Upgrade plan package already at plan version, skipping",
				slog.String("package", name),
				slog.String("ecosystem", ecosystem))
		}
	}

	logger.DebugContext(ctx, "Analyzing packages for vulnerabilities",
		slog.String("ecosystem", ecosystem),
		slog.Int("planned", len(planned.Ignore)))
	response, err := client.AnalyzePackages(ctx, installed, mergeIgnore(ignore, planned.Ignore), ecosystem)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze packages: %w", err)
	}

	merged := &rootio.AnalyzePackagesResponse{
		Patches: append([]rootio.PackagePatch(nil), planned.Patches...),
		Skipped: response.Skipped,
	}
	for _, patch := range planned.Patches {
		logPackage(ctx, logger, "Package upgraded", "plan", patch)
	}
	plannedKeys := make(map[rootio.Package]struct{}, len(planned.Ignore))
	for _, pkg := range planned.Ignore {
		plannedKeys[pkg] = struct{}{}
	}
	for _, patch := range response.Patches {
		if _, isPlanned := plannedKeys[rootio.Package{Name: patch.PackageName, Version: patch.Version}]; isPlanned {
			continue
		}
		logPackage(ctx, logger, "Package patched", "api", patch)
		merged.Patches = append(merged.Patches, patch)
	}
	return merged, nil
}

func logPackage(ctx context.Context, logger *slog.Logger, msg, source string, patch rootio.PackagePatch) {
	logger.InfoContext(ctx, msg,
		slog.String("package", patch.PackageName),
		slog.String("from", patch.Version),
		slog.String("to", patch.Patch.Version),
		slog.String("source", source))
}

// mergeIgnore appends extra to base, dropping repeated name@version pairs.
func mergeIgnore(base, extra []rootio.Package) []rootio.Package {
	seen := make(map[rootio.Package]struct{}, len(base)+len(extra))
	var merged []rootio.Package
	for _, pkg := range append(append([]rootio.Package(nil), base...), extra...) {
		if _, dup := seen[pkg]; dup {
			continue
		}
		seen[pkg] = struct{}{}
		merged = append(merged, pkg)
	}
	return merged
}
