package npm

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/tidwall/pretty"
	"github.com/tidwall/sjson"
)

// PatchOptions defines how to patch package.json. Each entry in Sets is a
// fully-qualified sjson path (e.g. "overrides.dockerode.uuid") mapped to the
// value to write. Each parser is responsible for building the right paths for
// its ecosystem (nested for npm, slash for yarn, version-scoped flat for pnpm).
type PatchOptions struct {
	Sets            map[string]string
	PackageJSONPath string
}

// PackageJSONPatcher writes a set of values to package.json without disturbing
// surrounding fields, indentation, or key order.
type PackageJSONPatcher struct{}

func NewPackageJSONPatcher() *PackageJSONPatcher {
	return &PackageJSONPatcher{}
}

// Patch applies the given sjson sets to the file at PackageJSONPath, preserving
// existing formatting.
func (p *PackageJSONPatcher) Patch(ctx context.Context, opts PatchOptions) error {
	packageJSONPath := opts.PackageJSONPath
	if packageJSONPath == "" {
		packageJSONPath = "package.json"
	}

	content, err := os.ReadFile(packageJSONPath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", packageJSONPath, err)
	}

	indent := detectIndentation(content)

	for path, value := range opts.Sets {
		content, err = sjson.SetBytes(content, path, value)
		if err != nil {
			return fmt.Errorf("failed to set %s: %w", path, err)
		}
	}

	content = pretty.PrettyOptions(content, &pretty.Options{
		Width:    80,
		Prefix:   "",
		Indent:   indent,
		SortKeys: false,
	})

	if err := os.WriteFile(packageJSONPath, content, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", packageJSONPath, err)
	}

	return nil
}

// detectIndentation detects the indentation style used in JSON content
func detectIndentation(content []byte) string {
	re := regexp.MustCompile(`(?m)^(\s+)"`)
	matches := re.FindSubmatch(content)
	if len(matches) > 1 {
		return string(matches[1])
	}
	return "  "
}

// escapeSjsonKey escapes characters that sjson treats as special in key paths
// (`\`, `@`, `.`). The result is safe to use as a single path component.
func escapeSjsonKey(key string) string {
	key = strings.ReplaceAll(key, `\`, `\\`)
	key = strings.ReplaceAll(key, `@`, `\@`)
	key = strings.ReplaceAll(key, `.`, `\.`)
	return key
}
