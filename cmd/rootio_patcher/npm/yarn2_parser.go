package npm

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"rootio_patcher/cmd/rootio_patcher/common"

	"gopkg.in/yaml.v3"
)

// Yarn2Parser handles parsing of yarn.lock files (v2+ / Yarn Berry format)
type Yarn2Parser struct{}

// NewYarn2Parser creates a new yarn2 parser
func NewYarn2Parser() *Yarn2Parser {
	return &Yarn2Parser{}
}

// Ecosystem returns the ecosystem name
func (p *Yarn2Parser) Ecosystem() common.Ecosystem {
	return common.EcosystemNpm
}

// FilePatterns returns file patterns this parser handles
func (p *Yarn2Parser) FilePatterns() []string {
	return []string{"yarn.lock"}
}

// CanHandle checks if this parser can handle the given file
func (p *Yarn2Parser) CanHandle(fileName string) bool {
	return fileName == "yarn.lock" || strings.HasSuffix(fileName, "/yarn.lock")
}

// Yarn2LockFile represents the structure of Yarn 2+ lock file
type Yarn2LockFile struct {
	Metadata map[string]interface{} `yaml:"__metadata"`
	// All other keys are package entries
}

// Yarn2PackageEntry represents a package entry in Yarn 2+ lock file
type Yarn2PackageEntry struct {
	Version      string            `yaml:"version"`
	Resolution   string            `yaml:"resolution"`
	Dependencies map[string]string `yaml:"dependencies"`
	Checksum     string            `yaml:"checksum"`
	LanguageName string            `yaml:"languageName"`
	LinkType     string            `yaml:"linkType"`
}

// Parse parses yarn.lock files (v2+ / Yarn Berry format)
func (p *Yarn2Parser) Parse(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Parse YAML
	var data map[string]interface{}
	if err := yaml.Unmarshal(content, &data); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Verify it's a Yarn 2+ lock file by checking for __metadata
	metadata, hasMetadata := data["__metadata"]
	if !hasMetadata {
		return nil, fmt.Errorf("not a Yarn 2+ lock file (missing __metadata)")
	}

	// Verify version
	metadataMap, ok := metadata.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid __metadata format")
	}

	version, hasVersion := metadataMap["version"]
	if !hasVersion {
		return nil, fmt.Errorf("missing version in __metadata")
	}

	versionStr := fmt.Sprintf("%v", version)
	versionNum, err := strconv.Atoi(strings.TrimSpace(versionStr))
	if err != nil || versionNum < 2 {
		return nil, fmt.Errorf("yarn.lock version not supported (expected v2+, got v%s)", versionStr)
	}

	var packages []common.PackageInfo
	seen := make(map[string]bool)

	// Process all entries except __metadata
	for key, value := range data {
		if key == "__metadata" {
			continue
		}

		// Parse package entry
		valueMap, ok := value.(map[string]interface{})
		if !ok {
			continue
		}

		// Extract version
		versionVal, hasVersion := valueMap["version"]
		if !hasVersion {
			continue
		}
		version := fmt.Sprintf("%v", versionVal)

		// Parse package name from key
		// Key format: "package@npm:version" or "@scope/package@npm:version" or multiple specs
		name := parseYarn2PackageName(key)
		if name == "" {
			continue
		}

		// Create unique key to avoid duplicates
		uniqueKey := fmt.Sprintf("%s@%s", name, version)
		if seen[uniqueKey] {
			continue
		}
		seen[uniqueKey] = true

		packages = append(packages, common.PackageInfo{
			Name:              name,
			Version:           version,
			VersionConstraint: version,
			Ecosystem:         common.EcosystemNpm,
			Direct:            false, // Yarn 2+ doesn't easily distinguish direct vs transitive
			Dev:               false,
		})
	}

	return packages, nil
}

// parseYarn2PackageName extracts package name from Yarn 2+ lock file key
// Examples:
//   - "lodash@npm:4.17.21" -> "lodash"
//   - "@babel/runtime@npm:7.25.0" -> "@babel/runtime"
//   - "lodash@npm:^4.17.0, lodash@npm:^4.17.21" -> "lodash"
func parseYarn2PackageName(key string) string {
	// Handle multiple specs separated by comma
	firstSpec := key
	if idx := strings.Index(key, ","); idx != -1 {
		firstSpec = strings.TrimSpace(key[:idx])
	}

	// Remove quotes if present
	firstSpec = strings.Trim(firstSpec, "\"")

	// Find the npm: or workspace: part and extract package name before it
	// Format: "package@npm:version" or "@scope/package@npm:version"
	parts := strings.Split(firstSpec, "@")

	// Handle scoped packages
	if strings.HasPrefix(firstSpec, "@") {
		// Scoped package: @scope/package@npm:version
		if len(parts) >= 3 {
			// parts[0] = ""
			// parts[1] = "scope/package"
			// parts[2] = "npm:version"
			return "@" + parts[1]
		}
	} else {
		// Regular package: package@npm:version
		if len(parts) >= 2 {
			// parts[0] = "package"
			// parts[1] = "npm:version"
			return parts[0]
		}
	}

	return ""
}

// Update updates package versions in yarn.lock (not implemented - yarn regenerates lock file)
func (p *Yarn2Parser) Update(ctx context.Context, filePath string, updates map[string]string) (string, error) {
	return "", fmt.Errorf("yarn.lock update not supported - run 'yarn install' to regenerate")
}

// Validate validates yarn.lock format for Yarn 2+
func (p *Yarn2Parser) Validate(content string) bool {
	var data map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &data); err != nil {
		return false
	}

	// Check for __metadata which is present in Yarn 2+ but not Yarn 1
	metadata, hasMetadata := data["__metadata"]
	if !hasMetadata {
		return false
	}

	metadataMap, ok := metadata.(map[string]interface{})
	if !ok {
		return false
	}

	// Must have version field
	_, hasVersion := metadataMap["version"]
	return hasVersion
}

// FindParents scans a Yarn 2+ lock file for packages that depend on
// packageName at a range resolving to the given version. Returns parent
// package names. Empty result means the vulnerable copy is only top-level.
func (p *Yarn2Parser) FindParents(ctx context.Context, lockFilePath, packageName, version string) ([]string, error) {
	content, err := os.ReadFile(lockFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	var data map[string]interface{}
	if err := yaml.Unmarshal(content, &data); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	type entry struct {
		Specs        []string
		Version      string
		Dependencies map[string]string
	}
	var entries []entry

	for key, value := range data {
		if key == "__metadata" {
			continue
		}
		valueMap, ok := value.(map[string]interface{})
		if !ok {
			continue
		}
		versionVal, _ := valueMap["version"]
		entryVersion := fmt.Sprintf("%v", versionVal)

		e := entry{Version: entryVersion, Dependencies: map[string]string{}}

		// Parse the entry key (may be comma-separated, may have npm: prefix).
		for _, spec := range strings.Split(key, ",") {
			spec = strings.TrimSpace(spec)
			spec = strings.Trim(spec, `"`)
			if spec == "" {
				continue
			}
			e.Specs = append(e.Specs, spec)
		}

		if depsRaw, ok := valueMap["dependencies"].(map[string]interface{}); ok {
			for depName, depRange := range depsRaw {
				e.Dependencies[depName] = fmt.Sprintf("%v", depRange)
			}
		}
		entries = append(entries, e)
	}

	matchingRanges := make(map[string]bool)
	for _, e := range entries {
		if e.Version != version {
			continue
		}
		for _, spec := range e.Specs {
			name, rng := splitYarnSpec(spec)
			rng = strings.TrimPrefix(rng, "npm:")
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
		if !hasDep {
			continue
		}
		depRange = strings.TrimPrefix(depRange, "npm:")
		if !matchingRanges[depRange] {
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
func (p *Yarn2Parser) UpdatePackageJSON(ctx context.Context, overrides []ScopedOverride, packageJSONPath string) error {
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
// packageName as a direct dep that yarn 2+ resolves to the given version.
// We match each direct dep range against the Yarn 2+ lockfile entry specs;
// when any spec for that name resolves to the vulnerable version and the
// declared range matches, the user's own usage is vulnerable.
func (p *Yarn2Parser) IsDirectVulnerable(ctx context.Context, lockFilePath, packageJSONPath, packageName, version string) (bool, error) {
	specs, err := readDirectDepSpecs(packageJSONPath, packageName)
	if err != nil || len(specs) == 0 {
		return false, err
	}
	content, err := os.ReadFile(lockFilePath)
	if err != nil {
		return false, fmt.Errorf("failed to read file: %w", err)
	}
	var data map[string]interface{}
	if err := yaml.Unmarshal(content, &data); err != nil {
		return false, fmt.Errorf("failed to parse YAML: %w", err)
	}
	for key, value := range data {
		if key == "__metadata" {
			continue
		}
		valueMap, ok := value.(map[string]interface{})
		if !ok {
			continue
		}
		entryVersion := fmt.Sprintf("%v", valueMap["version"])
		if entryVersion != version {
			continue
		}
		for _, spec := range strings.Split(key, ",") {
			name, rng := splitYarnSpec(strings.Trim(strings.TrimSpace(spec), `"`))
			rng = strings.TrimPrefix(rng, "npm:")
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
