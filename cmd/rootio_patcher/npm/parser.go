package npm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"rootio_patcher/cmd/rootio_patcher/common"
)

// Parser handles parsing of npm package-lock.json files
type NpmParser struct{}

// NewParser creates a new npm parser
func NewParser() *NpmParser {
	return &NpmParser{}
}

// Ecosystem returns the ecosystem name
func (p *NpmParser) Ecosystem() common.Ecosystem {
	return common.EcosystemNpm
}

// FilePatterns returns file patterns this parser handles
func (p *NpmParser) FilePatterns() []string {
	return []string{"package-lock.json", "yarn.lock", "pnpm-lock.yaml"}
}

// CanHandle checks if this parser can handle the given file
func (p *NpmParser) CanHandle(fileName string) bool {
	for _, pattern := range p.FilePatterns() {
		if fileName == pattern || strings.HasSuffix(fileName, pattern) {
			return true
		}
	}
	return false
}

// PackageLockJSON represents the structure of package-lock.json
type PackageLockJSON struct {
	Name            string                       `json:"name"`
	Version         string                       `json:"version"`
	LockfileVersion int                          `json:"lockfileVersion"`
	Packages        map[string]PackageLockEntry  `json:"packages"`
	Dependencies    map[string]DependencyEntry   `json:"dependencies,omitempty"`
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
	Version  string                     `json:"version"`
	Resolved string                     `json:"resolved,omitempty"`
	Dev      bool                       `json:"dev,omitempty"`
	Requires map[string]string          `json:"requires,omitempty"`
}

// Parse parses lock files (package-lock.json, yarn.lock, pnpm-lock.yaml) and returns all packages
func (p *NpmParser) Parse(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
	// Detect file type based on file name
	if strings.HasSuffix(filePath, "yarn.lock") {
		return p.parseYarnLock(filePath)
	}
	if strings.HasSuffix(filePath, "pnpm-lock.yaml") {
		return p.parsePnpmLock(filePath)
	}

	// Default to npm package-lock.json parser
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

// parseYarnLock parses yarn.lock files
func (p *NpmParser) parseYarnLock(filePath string) ([]common.PackageInfo, error) {
	return nil, fmt.Errorf("yarn.lock parsing not yet implemented")
}

// parsePnpmLock parses pnpm-lock.yaml files
func (p *NpmParser) parsePnpmLock(filePath string) ([]common.PackageInfo, error) {
	return nil, fmt.Errorf("pnpm-lock.yaml parsing not yet implemented")
}
