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

// UpdatePackageJSON updates package.json with yarn resolutions
func (p *YarnParser) UpdatePackageJSON(ctx context.Context, overrides map[string]string) error {
	patcher := NewPackageJSONPatcher()
	return patcher.Patch(ctx, PatchOptions{
		Updates:                  overrides,
		UpdateDirectDependencies: true,
		OverridesPath:            "resolutions",
	})
}
