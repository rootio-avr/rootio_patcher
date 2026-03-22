package npm

import (
	"context"
	"fmt"
	"os"
	"strings"

	"rootio_patcher/cmd/rootio_patcher/common"

	"gopkg.in/yaml.v3"
)

// PnpmParser handles parsing of pnpm-lock.yaml files
type PnpmParser struct{}

// NewPnpmParser creates a new pnpm parser
func NewPnpmParser() *PnpmParser {
	return &PnpmParser{}
}

// Ecosystem returns the ecosystem name
func (p *PnpmParser) Ecosystem() common.Ecosystem {
	return common.EcosystemNpm
}

// FilePatterns returns file patterns this parser handles
func (p *PnpmParser) FilePatterns() []string {
	return []string{"pnpm-lock.yaml"}
}

// CanHandle checks if this parser can handle the given file
func (p *PnpmParser) CanHandle(fileName string) bool {
	return fileName == "pnpm-lock.yaml" || strings.HasSuffix(fileName, "/pnpm-lock.yaml")
}

// PnpmLockFile represents the structure of pnpm-lock.yaml
type PnpmLockFile struct {
	LockfileVersion string                       `yaml:"lockfileVersion"`
	Importers       map[string]PnpmImporter      `yaml:"importers"`
	Packages        map[string]PnpmPackageEntry  `yaml:"packages"`
}

// PnpmImporter represents an importer (project root or workspace)
type PnpmImporter struct {
	Dependencies    map[string]PnpmDependency `yaml:"dependencies"`
	DevDependencies map[string]PnpmDependency `yaml:"devDependencies"`
}

// PnpmDependency represents a dependency in the importers section
type PnpmDependency struct {
	Specifier string `yaml:"specifier"`
	Version   string `yaml:"version"`
}

// PnpmPackageEntry represents a package entry in the packages section
type PnpmPackageEntry struct {
	Resolution map[string]string `yaml:"resolution"`
}

// Parse parses pnpm-lock.yaml files
func (p *PnpmParser) Parse(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var lockfile PnpmLockFile
	if err := yaml.Unmarshal(content, &lockfile); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	var packages []common.PackageInfo
	seen := make(map[string]bool)

	// Build set of dev dependencies
	devDeps := make(map[string]bool)
	for _, importer := range lockfile.Importers {
		for pkgName := range importer.DevDependencies {
			devDeps[pkgName] = true
		}
	}

	// Parse packages from the "packages" section
	for pkgKey := range lockfile.Packages {
		// Package key format: "packageName@version" or "@scope/package@version"
		name, version := parsePnpmPackageKey(pkgKey)
		if name == "" || version == "" {
			continue
		}

		// Create unique key to avoid duplicates
		key := fmt.Sprintf("%s@%s", name, version)
		if seen[key] {
			continue
		}
		seen[key] = true

		// Check if it's a dev dependency
		isDev := devDeps[name]

		packages = append(packages, common.PackageInfo{
			Name:              name,
			Version:           version,
			VersionConstraint: version,
			Ecosystem:         common.EcosystemNpm,
			Direct:            false, // We could check importers to determine this
			Dev:               isDev,
		})
	}

	return packages, nil
}

// parsePnpmPackageKey parses package name and version from pnpm package key
// Examples:
//   - "lodash@4.17.23" -> ("lodash", "4.17.23")
//   - "@babel/runtime@7.25.0" -> ("@babel/runtime", "7.25.0")
func parsePnpmPackageKey(key string) (name, version string) {
	// Find the last @ which separates name from version
	// For scoped packages like @babel/runtime@7.25.0, we need to skip the first @
	lastAt := strings.LastIndex(key, "@")
	if lastAt <= 0 {
		return "", ""
	}

	name = key[:lastAt]
	version = key[lastAt+1:]

	return name, version
}

// Update updates package versions in pnpm-lock.yaml
func (p *PnpmParser) Update(ctx context.Context, filePath string, updates map[string]string) (string, error) {
	return "", fmt.Errorf("pnpm-lock.yaml update not supported - run 'pnpm install' to regenerate")
}

// Validate validates pnpm-lock.yaml format
func (p *PnpmParser) Validate(content string) bool {
	if content == "" {
		return false
	}

	var lockfile PnpmLockFile
	err := yaml.Unmarshal([]byte(content), &lockfile)
	if err != nil {
		return false
	}

	// Must have lockfileVersion field
	return lockfile.LockfileVersion != ""
}

// UpdatePackageJSON updates package.json with pnpm overrides (nested under "pnpm")
func (p *PnpmParser) UpdatePackageJSON(ctx context.Context, overrides map[string]string, packageJSONPath string) error {
	patcher := NewPackageJSONPatcher()
	return patcher.Patch(ctx, PatchOptions{
		Updates:                  overrides,
		UpdateDirectDependencies: true,
		OverridesPath:            "pnpm.overrides",
		PackageJSONPath:          packageJSONPath,
	})
}
