package npm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/pkg/rootio"

	"github.com/tidwall/gjson"
	"gopkg.in/yaml.v3"
)

// Override-section paths used by the parsers. Defined in one place so a
// rename (e.g. pnpm renaming the field in a future major) is a single edit.
const (
	npmOverridesPath    = "overrides"
	yarnResolutionsPath = "resolutions"
	pnpmOverridesPath   = "pnpm.overrides"
)

// ScopedOverride captures everything UpdatePackageJSON needs to write a single
// override entry. Each parser formats it according to its ecosystem's accepted
// override syntax (version-scoped flat keys for npm/pnpm, slash-paths for yarn).
//
// Parents lists the transitive consumers of PackageName at this Version.
// RewriteDirect is set when the user's own direct dependency resolves to this
// vulnerable Version — in that case the patcher rewrites the dependencies
// entry to the patched package. Parents and RewriteDirect are independent:
// a single patch can produce both (rewrite the direct line AND emit overrides
// for transitive consumers).
//
// PatchInfo contains the plain package name and patched version to rename a
// direct dependency to (e.g. "axios"@"1.15.0-aikido.12").
type ScopedOverride struct {
	PackageName   string
	Version       string
	Value         string           // Plain patched version, used for transitive overrides
	PatchInfo     rootio.PatchInfo // Patch info for direct dep rewrites
	Parents       []string
	RewriteDirect bool
}

// buildResolutionSetsWithDirect builds the sjson-path → value map for yarn
// and yarn2, which both use the parent/child slash-path resolution shape.
// Shared so YarnParser and Yarn2Parser don't drift. When an override has
// RewriteDirect=true the dependencies/devDependencies entry is renamed to
// PatchInfo's plain name/version, mirroring npm's own direct-dependency
// rewrite in buildNpmOverrideSets.
func buildResolutionSetsWithDirect(overrides []ScopedOverride, packageJSONPath string) (map[string]string, []string, error) {
	sets := make(map[string]string)
	var deletes []string
	var pkgContent []byte
	for _, ov := range overrides {
		for _, parent := range ov.Parents {
			sets[yarnResolutionsPath+"."+escapeSjsonKey(parent+"/"+ov.PackageName)] = ov.Value
		}
		if ov.RewriteDirect {
			if pkgContent == nil {
				c, err := os.ReadFile(packageJSONPath)
				if err != nil {
					return nil, nil, fmt.Errorf("failed to read %s: %w", packageJSONPath, err)
				}
				pkgContent = c
			}
			newPkgName := ov.PatchInfo.Name
			newVersion := ov.PatchInfo.Version
			for _, field := range []string{"dependencies", "devDependencies"} {
				oldPath := field + "." + escapeSjsonKey(ov.PackageName)
				if gjsonGet(pkgContent, oldPath).Exists() {
					deletes = append(deletes, oldPath)
					newPath := field + "." + escapeSjsonKey(newPkgName)
					sets[newPath] = newVersion
				}
			}
		}
	}
	return sets, deletes, nil
}

// readDirectDepSpecs returns the spec strings under dependencies and
// devDependencies for the given package name in package.json, or nil if
// the package isn't a direct dep.
func readDirectDepSpecs(packageJSONPath, packageName string) ([]string, error) {
	content, err := os.ReadFile(packageJSONPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", packageJSONPath, err)
	}
	var specs []string
	for _, field := range []string{"dependencies", "devDependencies"} {
		v := gjsonGet(content, field+"."+escapeSjsonKey(packageName))
		if v.Exists() && v.Type == gjson.String {
			specs = append(specs, v.String())
		}
	}
	return specs, nil
}

// npmParser is the interface required by the npm App — it extends the common Parser
// with npm-specific package.json patching. Defined here (not in common) to avoid
// polluting the shared interface with JS/npm concerns (e.g. Maven shouldn't need this).
type npmParser interface {
	common.Parser
	// FindParents returns the set of packages in the lock file that depend on
	// packageName at the given version. Empty result means no parent scoping
	// is possible (the vulnerable copy is at the top level).
	FindParents(ctx context.Context, lockFilePath, packageName, version string) ([]string, error)
	// IsDirectVulnerable reports whether the user's package.json declares
	// packageName as a direct dependency that resolves to the given
	// vulnerable version. The caller uses this to decide whether to rewrite
	// the dependencies entry to an alias.
	IsDirectVulnerable(ctx context.Context, lockFilePath, packageJSONPath, packageName, version string) (bool, error)
	// UpdatePackageJSON writes the given overrides to package.json using the
	// appropriate ecosystem-specific override shape. When ScopedOverride
	// has RewriteDirect=true it also rewrites the dependencies/devDependencies
	// entry for that package to the alias value.
	UpdatePackageJSON(ctx context.Context, overrides []ScopedOverride, packageJSONPath string) error
}

// NewParser creates a default npm parser.
func NewParser() npmParser {
	return NewNpmParser()
}

// GetParserForFile returns the appropriate parser for the given lock file path.
func GetParserForFile(filePath string) (npmParser, error) {
	if strings.HasSuffix(filePath, "package-lock.json") {
		return NewNpmParser(), nil
	}
	if strings.HasSuffix(filePath, "yarn.lock") {
		return detectYarnVersion(filePath)
	}
	if strings.HasSuffix(filePath, "pnpm-lock.yaml") {
		return NewPnpmParser(), nil
	}
	return nil, fmt.Errorf("unsupported lock file: %s", filePath)
}

// GetParserForPackageManager returns the appropriate parser for the given package manager.
// For yarn, it auto-detects the version from yarn.lock in the current directory.
func GetParserForPackageManager(packageManager string) (npmParser, error) {
	return GetParserForPackageManagerInDir(packageManager, ".")
}

// GetParserForPackageManagerInDir returns the appropriate parser for the given package manager,
// looking for the yarn.lock file in directory to detect Yarn v1 vs v2+.
func GetParserForPackageManagerInDir(packageManager, directory string) (npmParser, error) {
	switch packageManager {
	case "npm":
		return NewNpmParser(), nil
	case "yarn":
		lockFilePath := filepath.Join(directory, "yarn.lock")
		if _, err := os.Stat(lockFilePath); err == nil {
			return detectYarnVersion(lockFilePath)
		}
		// File doesn't exist yet — default to Yarn 1 for backwards compatibility
		return NewYarnParser(), nil
	case "pnpm":
		return NewPnpmParser(), nil
	default:
		return nil, fmt.Errorf("unsupported package manager: %s", packageManager)
	}
}

// detectYarnVersion detects whether a yarn.lock file is Yarn 1 or Yarn 2+.
func detectYarnVersion(filePath string) (npmParser, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read yarn.lock: %w", err)
	}

	contentStr := string(content)

	// Yarn 2+ lock files are YAML and have __metadata
	var data map[string]interface{}
	if err := yaml.Unmarshal(content, &data); err == nil {
		if _, hasMetadata := data["__metadata"]; hasMetadata {
			return NewYarn2Parser(), nil
		}
	}

	// Yarn 1 lock files have "# yarn lockfile v1" comment
	if strings.Contains(contentStr, "# yarn lockfile v1") {
		return NewYarnParser(), nil
	}

	// Default to Yarn 1 parser for backwards compatibility
	return NewYarnParser(), nil
}
