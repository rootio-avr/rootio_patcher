package golang

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"

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

// GoModParser parses and patches go.mod files.
type GoModParser struct {
	logger *slog.Logger
}

// NewGoModParser creates a new GoModParser.
func NewGoModParser(logger *slog.Logger) *GoModParser {
	return &GoModParser{logger: logger}
}

// Parse reads go.mod and returns all require entries with pinned semver versions.
// Entries with pseudo-versions or non-semver versions are skipped and logged at debug level.
func (p *GoModParser) Parse(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", filePath, err)
	}
	defer f.Close()

	var packages []common.PackageInfo
	scanner := bufio.NewScanner(f)

	inBlock := false
	blockType := ""

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Detect block start: "keyword ("
		if !inBlock {
			for _, kw := range []string{"require", "replace", "exclude", "retract"} {
				if trimmed == kw+" (" {
					inBlock = true
					blockType = kw
					break
				}
			}
			if inBlock {
				continue
			}
		}

		// Detect block end
		if inBlock && trimmed == ")" {
			inBlock = false
			blockType = ""
			continue
		}

		var modPath, version string
		var isIndirect bool

		if inBlock {
			if blockType != "require" {
				continue
			}
			// format inside require block: "module version [// indirect]"
			parts := strings.Fields(trimmed)
			if len(parts) < 2 {
				continue
			}
			modPath = parts[0]
			version = parts[1]
			isIndirect = strings.Contains(trimmed, "// indirect")
		} else if rest, ok := strings.CutPrefix(trimmed, "require "); ok {
			// standalone: "require module version [// indirect]"
			parts := strings.Fields(rest)
			if len(parts) < 2 {
				continue
			}
			modPath = parts[0]
			version = parts[1]
			isIndirect = strings.Contains(trimmed, "// indirect")
		} else {
			continue
		}

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
			Direct:    !isIndirect,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", filePath, err)
	}

	return packages, nil
}

// Patch reads go.mod, adds or overwrites version-pinned replace directives for each update,
// preserves existing replace directives for other modules, and returns the new file content.
func (p *GoModParser) Patch(ctx context.Context, filePath string, updates []GoModUpdate) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", filePath, err)
	}

	// Build lookup: "module version" → update index
	updateKeys := make(map[string]int, len(updates))
	for i, u := range updates {
		updateKeys[u.Module+" "+u.CurrentVersion] = i
	}

	lines := strings.Split(string(data), "\n")
	applied := make([]bool, len(updates))

	var result []string
	inReplaceBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "replace (" {
			inReplaceBlock = true
			result = append(result, line)
			continue
		}

		if inReplaceBlock {
			if trimmed == ")" {
				inReplaceBlock = false
				result = append(result, line)
				continue
			}
			if newLine, idx, ok := matchBlockReplace(trimmed, updates, updateKeys); ok {
				result = append(result, newLine)
				applied[idx] = true
			} else {
				result = append(result, line)
			}
			continue
		}

		if newLine, idx, ok := matchStandaloneReplace(trimmed, updates, updateKeys); ok {
			result = append(result, newLine)
			applied[idx] = true
		} else {
			result = append(result, line)
		}
	}

	// Append updates that had no existing replace directive
	for i, u := range updates {
		if !applied[i] {
			result = append(result, fmt.Sprintf("replace %s %s => %s %s", u.Module, u.CurrentVersion, u.AliasName, u.AliasVersion))
		}
	}

	return strings.Join(result, "\n"), nil
}

// matchBlockReplace tries to match a line inside a replace (...) block against known updates.
// Returns the replacement line, the update index, and whether a match was found.
func matchBlockReplace(trimmed string, updates []GoModUpdate, updateKeys map[string]int) (string, int, bool) {
	// Format: "module version => replacement_module replacement_version"
	parts := strings.SplitN(trimmed, " => ", 2)
	if len(parts) != 2 {
		return "", 0, false
	}
	lhs := strings.Fields(parts[0])
	if len(lhs) < 2 {
		return "", 0, false
	}
	key := lhs[0] + " " + lhs[1]
	idx, ok := updateKeys[key]
	if !ok {
		return "", 0, false
	}
	u := updates[idx]
	return fmt.Sprintf("\t%s %s => %s %s", u.Module, u.CurrentVersion, u.AliasName, u.AliasVersion), idx, true
}

// matchStandaloneReplace tries to match a standalone replace directive against known updates.
// Returns the replacement line, the update index, and whether a match was found.
func matchStandaloneReplace(trimmed string, updates []GoModUpdate, updateKeys map[string]int) (string, int, bool) {
	if !strings.HasPrefix(trimmed, "replace ") {
		return "", 0, false
	}
	rest := strings.TrimPrefix(trimmed, "replace ")
	parts := strings.SplitN(rest, " => ", 2)
	if len(parts) != 2 {
		return "", 0, false
	}
	lhs := strings.Fields(parts[0])
	if len(lhs) < 2 {
		return "", 0, false
	}
	key := lhs[0] + " " + lhs[1]
	idx, ok := updateKeys[key]
	if !ok {
		return "", 0, false
	}
	u := updates[idx]
	return fmt.Sprintf("replace %s %s => %s %s", u.Module, u.CurrentVersion, u.AliasName, u.AliasVersion), idx, true
}
