package npm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/pretty"
	"github.com/tidwall/sjson"
)

// PatchOptions defines how to patch package.json. Each entry in Sets is a
// fully-qualified sjson path (e.g. "overrides.dockerode.uuid") mapped to the
// value to write. Deletes contains paths to remove from the JSON.
// Each parser is responsible for building the right paths for its ecosystem.
type PatchOptions struct {
	Sets            map[string]string
	Deletes         []string
	PackageJSONPath string
}

// PackageJSONPatcher writes a set of values to package.json without disturbing
// surrounding fields, indentation, or key order.
type PackageJSONPatcher struct{}

func NewPackageJSONPatcher() *PackageJSONPatcher {
	return &PackageJSONPatcher{}
}

// Patch applies the given sjson sets and deletes to the file at PackageJSONPath,
// preserving existing formatting.
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

	// First apply deletions
	for _, path := range opts.Deletes {
		content, err = sjson.DeleteBytes(content, path)
		if err != nil {
			return fmt.Errorf("failed to delete %s: %w", path, err)
		}
	}

	// A plain string override on a package ("router": "1.3.7") is a version pin. Adding a child
	// override under it would make sjson replace the string with an object and lose the pin, so
	// turn it into {".": "<pin>"} first. That is npm's syntax for "this package, plus these children".
	for path := range opts.Sets {
		content = keepStringOverridePins(content, path)
	}

	// Then apply sets
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

// keepStringOverridePins rewrites every string override that sits on the way to path (below the
// top-level "overrides" key) into {".": "<string>"}, so a nested set under it keeps the pin.
func keepStringOverridePins(content []byte, path string) []byte {
	segs := splitSjsonPath(path)
	if len(segs) < 3 || segs[0] != npmOverridesPath {
		return content
	}
	for n := 2; n < len(segs); n++ {
		prefix := strings.Join(segs[:n], ".")
		v := gjsonGet(content, prefix)
		if v.Type != gjson.String {
			continue
		}
		pin, err := json.Marshal(v.String())
		if err != nil {
			continue
		}
		if out, err := sjson.SetRawBytes(content, prefix, []byte(`{".":`+string(pin)+`}`)); err == nil {
			content = out
		}
	}
	return content
}

// splitSjsonPath splits an sjson path on its unescaped dots. Escapes stay in the segments, so
// joining them with "." gives back a valid path.
func splitSjsonPath(path string) []string {
	var segs []string
	start := 0
	for i := 0; i < len(path); i++ {
		switch path[i] {
		case '\\':
			i++
		case '.':
			segs = append(segs, path[start:i])
			start = i + 1
		}
	}
	return append(segs, path[start:])
}
