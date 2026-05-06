package npm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"rootio_patcher/cmd/rootio_patcher/common"
)

// NpmParser handles parsing of npm package-lock.json files
type NpmParser struct{}

// NewNpmParser creates a new npm parser
func NewNpmParser() *NpmParser {
	return &NpmParser{}
}

// Ecosystem returns the ecosystem name
func (p *NpmParser) Ecosystem() common.Ecosystem {
	return common.EcosystemNpm
}

// FilePatterns returns file patterns this parser handles
func (p *NpmParser) FilePatterns() []string {
	return []string{"package-lock.json"}
}

// CanHandle checks if this parser can handle the given file
func (p *NpmParser) CanHandle(fileName string) bool {
	return fileName == "package-lock.json" || strings.HasSuffix(fileName, "/package-lock.json")
}

// PackageLockJSON represents the structure of package-lock.json
type PackageLockJSON struct {
	Name            string                      `json:"name"`
	Version         string                      `json:"version"`
	LockfileVersion int                         `json:"lockfileVersion"`
	Packages        map[string]PackageLockEntry `json:"packages"`
	Dependencies    map[string]DependencyEntry  `json:"dependencies,omitempty"`
}

// PackageLockEntry represents a package entry in the "packages" section
type PackageLockEntry struct {
	Version         string            `json:"version,omitempty"`
	Resolved        string            `json:"resolved,omitempty"`
	Integrity       string            `json:"integrity,omitempty"`
	Dev             bool              `json:"dev,omitempty"`
	Dependencies    map[string]string `json:"dependencies,omitempty"`
	DevDependencies map[string]string `json:"devDependencies,omitempty"`
}

// DependencyEntry represents a dependency in the legacy "dependencies" section
type DependencyEntry struct {
	Version  string            `json:"version"`
	Resolved string            `json:"resolved,omitempty"`
	Dev      bool              `json:"dev,omitempty"`
	Requires map[string]string `json:"requires,omitempty"`
}

// Parse parses package-lock.json and returns all packages
func (p *NpmParser) Parse(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var lockfile PackageLockJSON
	if err := json.Unmarshal(content, &lockfile); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	var packages []common.PackageInfo
	seen := make(map[string]bool)

	// Get direct dependencies from root package (the "" key)
	rootPkg, hasRoot := lockfile.Packages[""]
	directDeps := make(map[string]bool)
	directDevDeps := make(map[string]bool)

	if hasRoot {
		for dep := range rootPkg.Dependencies {
			directDeps[dep] = true
		}
		for dep := range rootPkg.DevDependencies {
			directDevDeps[dep] = true
		}
	}

	// Parse packages from the "packages" object (lockfile v2/v3)
	for pkgPath, pkgData := range lockfile.Packages {
		// Skip root package
		if pkgPath == "" {
			continue
		}

		name := extractPackageName(pkgPath)
		if name == "" {
			continue
		}

		version := pkgData.Version
		if version == "" {
			continue
		}

		// Create unique key to avoid duplicates
		key := fmt.Sprintf("%s@%s", name, version)
		if seen[key] {
			continue
		}
		seen[key] = true

		// Determine if direct or transitive
		isDirect := directDeps[name] || directDevDeps[name]
		isDev := directDevDeps[name] || pkgData.Dev

		packages = append(packages, common.PackageInfo{
			Name:              name,
			Version:           version,
			VersionConstraint: version, // Lock file has exact versions
			Ecosystem:         common.EcosystemNpm,
			Direct:            isDirect,
			Dev:               isDev,
		})
	}

	return packages, nil
}

// extractPackageName extracts package name from node_modules path
func extractPackageName(pkgPath string) string {
	if !strings.HasPrefix(pkgPath, "node_modules/") {
		return pkgPath
	}

	remainder := strings.TrimPrefix(pkgPath, "node_modules/")

	// Handle nested node_modules
	if idx := strings.Index(remainder, "/node_modules/"); idx != -1 {
		parts := strings.Split(remainder, "/node_modules/")
		return parts[len(parts)-1]
	}

	return remainder
}

// Update updates package versions in package-lock.json
func (p *NpmParser) Update(ctx context.Context, filePath string, updates map[string]string) (string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	var lockfile PackageLockJSON
	if err := json.Unmarshal(content, &lockfile); err != nil {
		return "", fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Update packages in the "packages" object
	for pkgPath, pkgData := range lockfile.Packages {
		if pkgPath == "" {
			continue
		}

		name := extractPackageName(pkgPath)
		if newVersion, ok := updates[name]; ok {
			oldVersion := pkgData.Version
			pkgData.Version = newVersion

			// Update resolved URL if present
			if pkgData.Resolved != "" && oldVersion != "" {
				pkgData.Resolved = strings.Replace(pkgData.Resolved, oldVersion, newVersion, 1)
			}

			lockfile.Packages[pkgPath] = pkgData
		}
	}

	// Marshal back to JSON with indentation
	updatedContent, err := json.MarshalIndent(lockfile, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return string(updatedContent) + "\n", nil
}

// Validate validates JSON syntax
func (p *NpmParser) Validate(content string) bool {
	var lockfile PackageLockJSON
	return json.Unmarshal([]byte(content), &lockfile) == nil
}

// FindParents returns the names of packages in the lock file that consume
// packageName at the given version. It walks every package entry that
// declares packageName as a dependency and, using npm's resolution rules
// (nested copy first, then ancestor hoisted copies, then root-level), checks
// which version actually resolves for that consumer. This covers both
// physically-nested copies (".../<parent>/node_modules/<child>") and hoisted
// transitives where the only on-disk copy is at the top level.
//
// An empty result means no transitive consumer matches — the vulnerable copy
// is only the user's direct dependency. In that case a flat override would
// trigger npm's EOVERRIDE check, so the caller must handle direct-dep
// vulnerabilities differently (e.g. skip patching or rewrite the direct dep).
func (p *NpmParser) FindParents(ctx context.Context, lockFilePath, packageName, version string) ([]string, error) {
	content, err := os.ReadFile(lockFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var lockfile PackageLockJSON
	if err := json.Unmarshal(content, &lockfile); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	seen := make(map[string]bool)
	var parents []string

	for pkgPath, pkgData := range lockfile.Packages {
		if pkgPath == "" {
			continue
		}
		if _, declares := pkgData.Dependencies[packageName]; !declares {
			continue
		}
		if resolveNpmDep(lockfile.Packages, pkgPath, packageName) != version {
			continue
		}

		parentName := extractPackageName(pkgPath)
		if parentName == "" || parentName == packageName || seen[parentName] {
			continue
		}
		seen[parentName] = true
		parents = append(parents, parentName)
	}

	return parents, nil
}

// resolveNpmDep mimics npm's lookup order: check the consumer's own
// node_modules first, then walk up each ancestor level, then fall back to
// the top-level node_modules. Returns "" if no copy is found.
func resolveNpmDep(packages map[string]PackageLockEntry, parentPath, childName string) string {
	parts := strings.Split(parentPath, "/node_modules/")
	for i := len(parts); i >= 1; i-- {
		prefix := strings.Join(parts[:i], "/node_modules/")
		if entry, ok := packages[prefix+"/node_modules/"+childName]; ok {
			return entry.Version
		}
	}
	if entry, ok := packages["node_modules/"+childName]; ok {
		return entry.Version
	}
	return ""
}

// extractParentName returns the parent package name for a nested package path,
// or "" for top-level paths.
//
//	"node_modules/uuid"                          → ""
//	"node_modules/dockerode/node_modules/uuid"   → "dockerode"
//	"node_modules/@scope/foo/node_modules/uuid"  → "@scope/foo"
func extractParentName(pkgPath string) string {
	idx := strings.LastIndex(pkgPath, "/node_modules/")
	if idx == -1 {
		return ""
	}
	prefix := pkgPath[:idx]
	if !strings.HasPrefix(prefix, "node_modules/") {
		return ""
	}
	return extractPackageName(prefix)
}

// UpdatePackageJSON writes npm overrides as parent-nested objects:
//
//	"overrides": { "<parent>": { "<child>": "<alias>" } }
//
// When no parent is known the override falls back to a flat top-level entry.
// Direct dependencies are never modified.
func (p *NpmParser) UpdatePackageJSON(ctx context.Context, overrides []ScopedOverride, packageJSONPath string) error {
	sets := make(map[string]string)
	for _, ov := range overrides {
		if len(ov.Parents) == 0 {
			sets[npmOverridesPath+"."+escapeSjsonKey(ov.PackageName)] = ov.Value
			continue
		}
		for _, parent := range ov.Parents {
			sets[npmOverridesPath+"."+escapeSjsonKey(parent)+"."+escapeSjsonKey(ov.PackageName)] = ov.Value
		}
	}
	return NewPackageJSONPatcher().Patch(ctx, PatchOptions{
		Sets:            sets,
		PackageJSONPath: packageJSONPath,
	})
}
