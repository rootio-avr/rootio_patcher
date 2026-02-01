package npm

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/pretty"
	"github.com/tidwall/sjson"
)

// PatchOptions defines what and where to patch in package.json
type PatchOptions struct {
	// Updates to apply: package name -> new value (e.g., "npm:@rootio/pkg@1.0.0")
	Updates map[string]string

	// UpdateDirectDependencies: if true, update dependencies/devDependencies fields
	UpdateDirectDependencies bool

	// OverridesPath: JSON path where to add overrides (e.g., "overrides", "resolutions", "pnpm.overrides")
	// If empty, no overrides field will be created
	OverridesPath string

	// PackageJSONPath: path to package.json file (default: "package.json")
	PackageJSONPath string
}

// PackageJSONPatcher handles updating package.json files
type PackageJSONPatcher struct{}

// NewPackageJSONPatcher creates a new patcher
func NewPackageJSONPatcher() *PackageJSONPatcher {
	return &PackageJSONPatcher{}
}

// Patch updates package.json with the given options
// It preserves formatting, indentation, and key order
func (p *PackageJSONPatcher) Patch(ctx context.Context, opts PatchOptions) error {
	// Use default path if not specified
	packageJSONPath := opts.PackageJSONPath
	if packageJSONPath == "" {
		packageJSONPath = "package.json"
	}

	// Read package.json
	content, err := os.ReadFile(packageJSONPath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", packageJSONPath, err)
	}

	// Detect original indentation
	indent := detectIndentation(content)

	// Update direct dependencies if requested
	if opts.UpdateDirectDependencies {
		for pkgName, newValue := range opts.Updates {
			escapedPkg := escapeSjsonKey(pkgName)

			// Check and update dependencies
			if gjson.GetBytes(content, "dependencies."+escapedPkg).Exists() {
				content, err = sjson.SetBytes(content, "dependencies."+escapedPkg, newValue)
				if err != nil {
					return fmt.Errorf("failed to update dependency %s: %w", pkgName, err)
				}
			}

			// Check and update devDependencies
			if gjson.GetBytes(content, "devDependencies."+escapedPkg).Exists() {
				content, err = sjson.SetBytes(content, "devDependencies."+escapedPkg, newValue)
				if err != nil {
					return fmt.Errorf("failed to update devDependency %s: %w", pkgName, err)
				}
			}
		}
	}

	// Add to overrides field if specified
	if opts.OverridesPath != "" {
		for pkgName, newValue := range opts.Updates {
			escapedPkg := escapeSjsonKey(pkgName)
			overridePath := opts.OverridesPath + "." + escapedPkg
			content, err = sjson.SetBytes(content, overridePath, newValue)
			if err != nil {
				return fmt.Errorf("failed to add override for %s at %s: %w", pkgName, opts.OverridesPath, err)
			}
		}
	}

	// Pretty-print with detected indentation
	content = pretty.PrettyOptions(content, &pretty.Options{
		Width:    80,
		Prefix:   "",
		Indent:   indent,
		SortKeys: false,
	})

	// Write back to file
	if err := os.WriteFile(packageJSONPath, content, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", packageJSONPath, err)
	}

	return nil
}

// detectIndentation detects the indentation style used in JSON content
func detectIndentation(content []byte) string {
	// Look for the first indented line
	re := regexp.MustCompile(`(?m)^(\s+)"`)
	matches := re.FindSubmatch(content)
	if len(matches) > 1 {
		return string(matches[1])
	}
	// Default to 2 spaces if no indentation found
	return "  "
}

// escapeSjsonKey escapes special characters in sjson keys
// sjson uses @ and . as special characters, so we need to escape them
func escapeSjsonKey(key string) string {
	key = strings.ReplaceAll(key, `\`, `\\`)
	key = strings.ReplaceAll(key, `@`, `\@`)
	key = strings.ReplaceAll(key, `.`, `\.`)
	return key
}
