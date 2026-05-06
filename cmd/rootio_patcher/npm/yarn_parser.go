package npm

import (
	"context"
	"fmt"
	"os"
	"strings"

	"rootio_patcher/cmd/rootio_patcher/common"
)

// YarnParser handles parsing of yarn.lock files
type YarnParser struct{}

// NewYarnParser creates a new yarn parser
func NewYarnParser() *YarnParser {
	return &YarnParser{}
}

// Ecosystem returns the ecosystem name
func (p *YarnParser) Ecosystem() common.Ecosystem {
	return common.EcosystemNpm
}

// FilePatterns returns file patterns this parser handles
func (p *YarnParser) FilePatterns() []string {
	return []string{"yarn.lock"}
}

// CanHandle checks if this parser can handle the given file
func (p *YarnParser) CanHandle(fileName string) bool {
	return fileName == "yarn.lock" || strings.HasSuffix(fileName, "/yarn.lock")
}

// Parse parses yarn.lock files (v1 format only)
func (p *YarnParser) Parse(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	lines := strings.Split(string(content), "\n")

	// Check version - must be v1
	if len(lines) < 2 {
		return nil, fmt.Errorf("invalid yarn.lock file: too short")
	}

	// Second line should be "# yarn lockfile v1"
	if !strings.Contains(lines[1], "# yarn lockfile v1") {
		return nil, fmt.Errorf("yarn.lock version not supported (only v1 is implemented)")
	}

	var packages []common.PackageInfo
	seen := make(map[string]bool)

	var currentPkg string
	var currentVersion string

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Skip empty lines and comments
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}

		// Package declaration line (starts at column 0, ends with :)
		if len(line) > 0 && line[0] != ' ' && strings.HasSuffix(line, ":") {
			// Parse package name from declaration
			// Format: "package@version:" or "package@^version, package@~version:"
			declaration := strings.TrimSuffix(line, ":")

			// Take first package spec (before comma)
			firstSpec := declaration
			if idx := strings.Index(declaration, ","); idx != -1 {
				firstSpec = strings.TrimSpace(declaration[:idx])
			}

			// Remove quotes if present (scoped packages are quoted)
			firstSpec = strings.Trim(firstSpec, "\"")

			// Extract package name (everything before last @)
			lastAt := strings.LastIndex(firstSpec, "@")
			if lastAt > 0 { // > 0 to handle scoped packages like @babel/runtime
				currentPkg = firstSpec[:lastAt]
				// Handle scoped packages: "@babel/runtime@7.25.0" -> "@babel/runtime"
				if strings.HasPrefix(currentPkg, "@") && !strings.Contains(currentPkg[1:], "@") {
					// This is a scoped package, name is everything before version
					currentPkg = firstSpec[:lastAt]
				}
			} else {
				currentPkg = firstSpec
			}
			currentVersion = ""
			continue
		}

		// Version line (indented, starts with "version")
		if strings.HasPrefix(strings.TrimSpace(line), "version ") {
			versionLine := strings.TrimSpace(line)
			versionParts := strings.SplitN(versionLine, " ", 2)
			if len(versionParts) == 2 {
				currentVersion = strings.Trim(versionParts[1], "\"")

				// Add package if we have both name and version
				if currentPkg != "" && currentVersion != "" {
					key := fmt.Sprintf("%s@%s", currentPkg, currentVersion)
					if !seen[key] {
						seen[key] = true
						packages = append(packages, common.PackageInfo{
							Name:              currentPkg,
							Version:           currentVersion,
							VersionConstraint: currentVersion,
							Ecosystem:         common.EcosystemNpm,
							Direct:            false, // yarn.lock doesn't distinguish direct vs transitive easily
							Dev:               false,
						})
					}
				}
			}
		}
	}

	return packages, nil
}

// Update updates package versions in yarn.lock (not implemented - yarn regenerates lock file)
func (p *YarnParser) Update(ctx context.Context, filePath string, updates map[string]string) (string, error) {
	return "", fmt.Errorf("yarn.lock update not supported - run 'yarn install' to regenerate")
}

// Validate validates yarn.lock format
func (p *YarnParser) Validate(content string) bool {
	lines := strings.Split(content, "\n")
	if len(lines) < 2 {
		return false
	}
	return strings.Contains(lines[1], "# yarn lockfile v1")
}

// FindParents scans yarn.lock for packages that depend on packageName at a
// range resolving to the given version. It returns the parent package names
// (e.g., "dockerode"). Empty result means the vulnerable copy is only at the
// top level.
func (p *YarnParser) FindParents(ctx context.Context, lockFilePath, packageName, version string) ([]string, error) {
	content, err := os.ReadFile(lockFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	entries := parseYarnLockEntries(content)

	matchingRanges := make(map[string]bool)
	for _, e := range entries {
		if e.Version != version {
			continue
		}
		for _, spec := range e.Specs {
			name, rng := splitYarnSpec(spec)
			if name == packageName {
				matchingRanges[rng] = true
			}
		}
	}
	if len(matchingRanges) == 0 {
		return nil, nil
	}

	seen := make(map[string]bool)
	var parents []string
	for _, e := range entries {
		depRange, hasDep := e.Dependencies[packageName]
		if !hasDep || !matchingRanges[depRange] {
			continue
		}
		if len(e.Specs) == 0 {
			continue
		}
		parentName, _ := splitYarnSpec(e.Specs[0])
		if parentName == "" || parentName == packageName || seen[parentName] {
			continue
		}
		seen[parentName] = true
		parents = append(parents, parentName)
	}
	return parents, nil
}

// UpdatePackageJSON writes yarn resolutions as parent/child slash paths:
//
//	"resolutions": { "<parent>/<child>": "<alias>" }
//
// When no parent is known the entry falls back to a flat "<child>" key.
// Direct dependencies are never modified.
func (p *YarnParser) UpdatePackageJSON(ctx context.Context, overrides []ScopedOverride, packageJSONPath string) error {
	sets, err := buildResolutionSetsWithDirect(overrides, packageJSONPath)
	if err != nil {
		return err
	}
	return NewPackageJSONPatcher().Patch(ctx, PatchOptions{
		Sets:            sets,
		PackageJSONPath: packageJSONPath,
	})
}

// IsDirectVulnerable reports whether the user's package.json declares
// packageName as a direct dep that yarn resolves to the given version.
// yarn.lock's entry keys map specs ("uuid@^11.0.3") to a single resolved
// version; if the direct spec resolves to the vulnerable version, the user's
// own usage is vulnerable too.
func (p *YarnParser) IsDirectVulnerable(ctx context.Context, lockFilePath, packageJSONPath, packageName, version string) (bool, error) {
	specs, err := readDirectDepSpecs(packageJSONPath, packageName)
	if err != nil || len(specs) == 0 {
		return false, err
	}
	content, err := os.ReadFile(lockFilePath)
	if err != nil {
		return false, fmt.Errorf("failed to read file: %w", err)
	}
	for _, e := range parseYarnLockEntries(content) {
		if e.Version != version {
			continue
		}
		for _, s := range e.Specs {
			name, rng := splitYarnSpec(s)
			if name != packageName {
				continue
			}
			for _, directSpec := range specs {
				if rng == directSpec {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

// yarnLockEntry is the structured shape of one entry in a yarn.lock file (v1
// format), used only for FindParents. Specs is the list of comma-separated
// keys before the `:` (e.g. `["uuid@^10.0.0", "uuid@^10.5.0"]`).
type yarnLockEntry struct {
	Specs        []string
	Version      string
	Dependencies map[string]string
}

// parseYarnLockEntries walks a yarn.lock v1 file and returns every entry's
// specs, resolved version, and dependencies block. It tolerates the
// indentation conventions yarn uses (2 spaces for entry body, 4 spaces for
// nested dependencies block).
func parseYarnLockEntries(content []byte) []yarnLockEntry {
	lines := strings.Split(string(content), "\n")
	var entries []yarnLockEntry
	var current *yarnLockEntry
	inDeps := false

	flush := func() {
		if current != nil {
			entries = append(entries, *current)
			current = nil
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if line[0] != ' ' && strings.HasSuffix(line, ":") {
			flush()
			current = &yarnLockEntry{Dependencies: make(map[string]string)}
			decl := strings.TrimSuffix(line, ":")
			for _, spec := range strings.Split(decl, ",") {
				spec = strings.TrimSpace(spec)
				spec = strings.Trim(spec, `"`)
				if spec != "" {
					current.Specs = append(current.Specs, spec)
				}
			}
			inDeps = false
			continue
		}

		if current == nil {
			continue
		}

		if strings.HasPrefix(line, "    ") && inDeps {
			parts := strings.SplitN(strings.TrimSpace(line), " ", 2)
			if len(parts) == 2 {
				name := strings.Trim(parts[0], `"`)
				rng := strings.Trim(parts[1], `"`)
				current.Dependencies[name] = rng
			}
			continue
		}

		if strings.HasPrefix(line, "  ") {
			inDeps = false
			body := strings.TrimSpace(line)
			if body == "dependencies:" {
				inDeps = true
				continue
			}
			if strings.HasPrefix(body, "version ") {
				parts := strings.SplitN(body, " ", 2)
				if len(parts) == 2 {
					current.Version = strings.Trim(parts[1], `"`)
				}
			}
		}
	}
	flush()
	return entries
}

// splitYarnSpec splits a yarn spec like "uuid@^10.0.0" or "@scope/name@npm:1.0"
// into (name, range). Returns the spec as name and "" if no version separator
// is found.
func splitYarnSpec(spec string) (name, rng string) {
	lastAt := strings.LastIndex(spec, "@")
	if lastAt <= 0 {
		return spec, ""
	}
	return spec[:lastAt], spec[lastAt+1:]
}
