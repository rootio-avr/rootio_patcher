package npm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"rootio_patcher/cmd/rootio_patcher/common"

	"github.com/tidwall/gjson"
)

// gjsonGet wraps gjson.GetBytes to centralize the dependency import.
func gjsonGet(content []byte, path string) gjson.Result { return gjson.GetBytes(content, path) }

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

	sort.Slice(packages, func(i, j int) bool {
		return packages[i].Name < packages[j].Name
	})

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

// IsDirectVulnerable reports whether the root package declares packageName
// as a direct dependency (in dependencies or devDependencies) AND the
// resolved top-level copy is at the given version. The lock file's root
// entry "" lists direct deps, and the resolved version of any direct dep
// lives at packages["node_modules/<name>"].
func (p *NpmParser) IsDirectVulnerable(ctx context.Context, lockFilePath, packageJSONPath, packageName, version string) (bool, error) {
	content, err := os.ReadFile(lockFilePath)
	if err != nil {
		return false, fmt.Errorf("failed to read file: %w", err)
	}
	var lockfile PackageLockJSON
	if err := json.Unmarshal(content, &lockfile); err != nil {
		return false, fmt.Errorf("failed to parse JSON: %w", err)
	}
	root, ok := lockfile.Packages[""]
	if !ok {
		return false, nil
	}
	_, inDeps := root.Dependencies[packageName]
	_, inDev := root.DevDependencies[packageName]
	if !inDeps && !inDev {
		return false, nil
	}
	topLevel, ok := lockfile.Packages["node_modules/"+packageName]
	if !ok {
		return false, nil
	}
	return topLevel.Version == version, nil
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

// UpdatePackageJSON writes npm overrides as version-scoped flat keys:
//
//	"overrides": { "<package>@<version>": "<alias>" }
//
// This form works universally for transitive and aliased dependencies.
// When the user's direct dep is vulnerable (RewriteDirect=true), the old package
// is removed from dependencies and replaced with the new @rootio package to avoid
// npm's EOVERRIDE error (which occurs when both a direct dependency and an override
// target the same package@version).
func (p *NpmParser) UpdatePackageJSON(ctx context.Context, overrides []ScopedOverride, packageJSONPath string) error {
	sets, deletes, err := buildNpmOverrideSets(overrides, packageJSONPath)
	if err != nil {
		return err
	}

	return NewPackageJSONPatcher().Patch(ctx, PatchOptions{
		Sets:            sets,
		Deletes:         deletes,
		PackageJSONPath: packageJSONPath,
	})
}

func buildNpmOverrideSets(overrides []ScopedOverride, packageJSONPath string) (map[string]string, []string, error) {
	sets := make(map[string]string)
	// deletes stays empty: the direct-dep rewrite keeps the original dependency key
	// (only its value changes to the npm: alias), so nothing is removed. Kept for the
	// shared PatchOptions.Deletes contract other ecosystems/paths may use.
	var deletes []string

	pkgJsonContent, err := os.ReadFile(packageJSONPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read %s: %w", packageJSONPath, err)
	}

	for _, ov := range overrides {
		// Nested/path-scoped overrides ({"<parent>": {"<pkg>": "..."}}) apply to
		// transitive consumers and take precedence over a flat key. Emit/refresh them
		// for every parent that already targets this package, whether the package is a
		// direct dep, a transitive dep, or both — a pre-existing nested entry would
		// otherwise shadow our patch forever. (Independent of the direct-vs-transitive
		// choice below, which only concerns the ROOT dependency line.)
		for parent, nestedKey := range findNestedOverrideParents(pkgJsonContent, ov.PackageName, ov.Version) {
			path := npmOverridesPath + "." + escapeSjsonKey(parent) + "." + escapeSjsonKey(nestedKey)
			sets[path] = ov.Value
		}

		if ov.RewriteDirect {
			// The vulnerable package is (also) a DIRECT dependency. A flat `overrides`
			// entry does NOT apply to a direct dep and conflicts with it (→ EOVERRIDE),
			// so we rewrite the dependency line ITSELF to the aliased package.
			//
			// We keep the ORIGINAL dependency key and set its value to the `npm:` alias
			// form: `"lodash": "npm:@rootio/lodash@4.17.20-root.io.3"`. This is the only
			// form npm can resolve for a `-root.io.N` build (which exists solely under the
			// @rootio scope, never under the original name on the public registry) — even
			// with a stale lockfile. Renaming the key to `@rootio/lodash` or pinning the
			// original name to a `-root.io.N` version both fail (ETARGET), regardless of
			// useAlias. Direct deps therefore always use the alias value.
			//
			// ov.Value is already the `npm:<alias>@<version>` form (built in applyPatches
			// from patch.PatchAlias). Note: any transitive consumers of the SAME package
			// at this version are still covered by the nested-parent overrides emitted
			// above, so those copies are patched too.
			rewritten := false
			for _, field := range []string{"dependencies", "devDependencies"} {
				depPath := field + "." + escapeSjsonKey(ov.PackageName)
				if gjsonGet(pkgJsonContent, depPath).Exists() {
					sets[depPath] = ov.Value
					rewritten = true
				}
			}
			if rewritten {
				// The direct-dep rewrite handles the root line; a flat override for the
				// same package@version would trigger EOVERRIDE, so skip it here.
				continue
			}
			// Defensive: RewriteDirect was set but no direct dep entry found — fall through
			// to the flat override form below.
		}

		// Transitive-only dependency: version-scoped flat override
		// (e.g., "uuid@9.0.1": "npm:@rootio/uuid@...") which npm applies to nested deps.
		key := ov.PackageName + "@" + ov.Version
		sets[npmOverridesPath+"."+escapeSjsonKey(key)] = ov.Value
	}
	return sets, deletes, nil
}

// findNestedOverrideParents scans the existing "overrides" object in
// package.json for nested/path-scoped entries that target packageName under
// some parent, e.g. "overrides": {"@apollo/query-planner": {"@apollo/federation-internals": "..."}}.
// It returns a map of parent key -> the exact nested key used (which may be
// version-scoped, e.g. "uuid@9.0.1"), so callers can overwrite that nested
// value in place instead of leaving it to shadow a new flat key.
func findNestedOverrideParents(pkgContent []byte, packageName, packageVersion string) map[string]string {
	result := make(map[string]string)
	gjsonGet(pkgContent, npmOverridesPath).ForEach(func(parent, value gjson.Result) bool {
		if !value.IsObject() {
			return true
		}
		value.ForEach(func(nestedKey, nestedValue gjson.Result) bool {
			if nestedValue.Type != gjson.String {
				return true
			}
			key := nestedKey.String()
			name := key
			version := ""
			if idx := strings.LastIndex(key, "@"); idx > 0 {
				name = key[:idx]
				version = key[idx+1:]
			}
			if name == packageName && (version == "" || version == packageVersion) {
				result[parent.String()] = nestedKey.String()
			}
			return true
		})
		return true
	})
	return result
}
