package npm

import (
	"fmt"
	"strings"

	"rootio_patcher/cmd/rootio_patcher/common"
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
		return NewYarnParser(), nil
	}
	if strings.HasSuffix(filePath, "pnpm-lock.yaml") {
		return NewPnpmParser(), nil
	}
	return nil, fmt.Errorf("unsupported lock file: %s", filePath)
}

// GetParserForPackageManager returns the appropriate parser for the given package manager
func GetParserForPackageManager(packageManager string) (common.Parser, error) {
	switch packageManager {
	case "npm":
		return NewNpmParser(), nil
	case "yarn":
		return NewYarnParser(), nil
	case "pnpm":
		return NewPnpmParser(), nil
	default:
		return nil, fmt.Errorf("unsupported package manager: %s", packageManager)
	}
}
