package npm

import (
	"fmt"
	"os"
	"strings"

	"rootio_patcher/cmd/rootio_patcher/common"

	"gopkg.in/yaml.v3"
)

// NewParser creates the appropriate parser based on file name or package manager
// Returns NpmParser by default for backwards compatibility
func NewParser() common.Parser {
	return NewNpmParser()
}

// GetParserForFile returns the appropriate parser for the given file
func GetParserForFile(filePath string) (common.Parser, error) {
	if strings.HasSuffix(filePath, "package-lock.json") {
		return NewNpmParser(), nil
	}
	if strings.HasSuffix(filePath, "yarn.lock") {
		// Need to detect if it's Yarn 1 or Yarn 2+
		return detectYarnVersion(filePath)
	}
	if strings.HasSuffix(filePath, "pnpm-lock.yaml") {
		return NewPnpmParser(), nil
	}
	return nil, fmt.Errorf("unsupported lock file: %s", filePath)
}

// GetParserForPackageManager returns the appropriate parser for the given package manager
// For yarn, it auto-detects the version from yarn.lock on disk
func GetParserForPackageManager(packageManager string) (common.Parser, error) {
	switch packageManager {
	case "npm":
		return NewNpmParser(), nil
	case "yarn":
		// Auto-detect Yarn version from yarn.lock file on disk
		lockFilePath := "yarn.lock"
		if _, err := os.Stat(lockFilePath); err == nil {
			// File exists, detect version
			return detectYarnVersion(lockFilePath)
		}
		// If file doesn't exist, default to Yarn 1 for backwards compatibility
		return NewYarnParser(), nil
	case "pnpm":
		return NewPnpmParser(), nil
	default:
		return nil, fmt.Errorf("unsupported package manager: %s", packageManager)
	}
}

// detectYarnVersion detects whether a yarn.lock file is Yarn 1 or Yarn 2+
func detectYarnVersion(filePath string) (common.Parser, error) {
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
