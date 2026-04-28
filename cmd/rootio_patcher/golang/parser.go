package golang

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"regexp"

	"golang.org/x/mod/modfile"

	"rootio_patcher/cmd/rootio_patcher/common"
)

// GoModUpdate describes a single version-pinned replace directive to add or update in go.mod.
type GoModUpdate struct {
	Module         string
	CurrentVersion string
	AliasName      string
	AliasVersion   string
}

var (
	semverRe        = regexp.MustCompile(`^v\d+\.\d+\.\d+`)
	pseudoVersionRe = regexp.MustCompile(`^v\d+\.\d+\.\d+-\d{14}-[0-9a-f]+$`)
)

func isPinnedVersion(v string) bool {
	return semverRe.MatchString(v) && !pseudoVersionRe.MatchString(v)
}

// GoModParser is the interface for parsing and patching go.mod files.
type GoModParser interface {
	Parse(ctx context.Context, filePath string) ([]common.PackageInfo, error)
	Patch(ctx context.Context, filePath string, updates []GoModUpdate) (string, error)
}

// goModParser parses and patches go.mod files.
type goModParser struct {
	logger *slog.Logger
}

// NewGoModParser creates a new goModParser.
func NewGoModParser(logger *slog.Logger) GoModParser {
	return &goModParser{logger: logger}
}

// Parse reads go.mod and returns all require entries with pinned semver versions.
// Entries with pseudo-versions or non-semver versions are skipped and logged at debug level.
func (p *goModParser) Parse(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filePath, err)
	}

	parsedModFile, err := modfile.Parse(filePath, data, nil)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filePath, err)
	}

	var packages []common.PackageInfo
	for _, req := range parsedModFile.Require {
		modPath := req.Mod.Path
		version := req.Mod.Version

		if !isPinnedVersion(version) {
			p.logger.DebugContext(ctx, "skipping require entry with unpinned version",
				slog.String("module", modPath),
				slog.String("version", version),
				slog.String("reason", "not a pinned semver release (pseudo-version or non-semver)"))
			continue
		}

		packages = append(packages, common.PackageInfo{
			Name:      modPath,
			Version:   version,
			Ecosystem: common.EcosystemGolang,
			Direct:    !req.Indirect,
		})
	}

	return packages, nil
}

// Patch reads go.mod, adds or overwrites version-pinned replace directives for each update,
// preserves existing replace directives for other modules, and returns the new file content.
func (p *goModParser) Patch(ctx context.Context, filePath string, updates []GoModUpdate) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", filePath, err)
	}

	f, err := modfile.Parse(filePath, data, nil)
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", filePath, err)
	}

	for _, u := range updates {
		if err := f.AddReplace(u.Module, u.CurrentVersion, u.AliasName, u.AliasVersion); err != nil {
			return "", fmt.Errorf("add replace for %s %s: %w", u.Module, u.CurrentVersion, err)
		}
		fmt.Printf("  - replace %s %s => %s %s\n", u.Module, u.CurrentVersion, u.AliasName, u.AliasVersion)
	}

	out := modfile.Format(f.Syntax)

	return string(out), nil
}
