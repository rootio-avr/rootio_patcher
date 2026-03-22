package npm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"rootio_patcher/cmd/rootio_patcher/common"

	"gopkg.in/yaml.v3"
)

// npmParser is the interface required by the npm App — it extends the common Parser
// with npm-specific package.json patching. Defined here (not in common) to avoid
// polluting the shared interface with JS/npm concerns (e.g. Maven shouldn't need this).
type npmParser interface {
	common.Parser
	UpdatePackageJSON(ctx context.Context, overrides map[string]string, packageJSONPath string) error
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
