package composer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"rootio_patcher/cmd/rootio_patcher/common"
)

// ComposerParser handles parsing of composer.json and composer.lock files.
type ComposerParser struct {
	logger *slog.Logger
}

// NewParser creates a new ComposerParser.
func NewParser(logger *slog.Logger) *ComposerParser {
	return &ComposerParser{logger: logger}
}

func (p *ComposerParser) Ecosystem() common.Ecosystem { return common.EcosystemComposer }
func (p *ComposerParser) FilePatterns() []string      { return []string{"composer.json"} }
func (p *ComposerParser) CanHandle(fileName string) bool {
	return filepath.Base(fileName) == "composer.json"
}

type lockFile struct {
	Packages    []lockEntry `json:"packages"`
	PackagesDev []lockEntry `json:"packages-dev"`
}

type lockEntry struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Parse reads exact resolved versions from composer.lock (required) and returns
// all installable packages (skipping platform entries that have no vendor prefix).
func (p *ComposerParser) Parse(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
	lockPath := filepath.Join(filepath.Dir(filePath), "composer.lock")
	if _, err := os.Stat(lockPath); err != nil {
		return nil, fmt.Errorf("composer.lock not found at %s: run 'composer install' first", lockPath)
	}

	data, err := os.ReadFile(lockPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", lockPath, err)
	}

	var lock lockFile
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", lockPath, err)
	}

	var packages []common.PackageInfo
	for _, entry := range lock.Packages {
		if !strings.Contains(entry.Name, "/") {
			p.logger.DebugContext(ctx, "skipping platform requirement", slog.String("name", entry.Name))
			continue
		}
		packages = append(packages, common.PackageInfo{
			Name:      entry.Name,
			Version:   entry.Version,
			Ecosystem: common.EcosystemComposer,
			Direct:    true,
			Dev:       false,
		})
	}
	for _, entry := range lock.PackagesDev {
		if !strings.Contains(entry.Name, "/") {
			p.logger.DebugContext(ctx, "skipping platform requirement", slog.String("name", entry.Name))
			continue
		}
		packages = append(packages, common.PackageInfo{
			Name:      entry.Name,
			Version:   entry.Version,
			Ecosystem: common.EcosystemComposer,
			Direct:    true,
			Dev:       true,
		})
	}

	return packages, nil
}

// composerJSON is used to detect which section a package lives in.
type composerJSON struct {
	Require    map[string]string `json:"require"`
	RequireDev map[string]string `json:"require-dev"`
}

// Update rewrites composer.json with patched versions.
// updates map: original package name -> "aliasName:newVersion"
// Direct deps are updated in place; transitive deps are added to require.
// Also injects a repositories entry for pkg.root.io if not already present.
func (p *ComposerParser) Update(ctx context.Context, filePath string, updates map[string]string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", filePath, err)
	}

	var manifest composerJSON
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", fmt.Errorf("failed to parse %s: %w", filePath, err)
	}
	if manifest.Require == nil {
		manifest.Require = map[string]string{}
	}
	if manifest.RequireDev == nil {
		manifest.RequireDev = map[string]string{}
	}

	content := string(data)

	for originalName, updateValue := range updates {
		aliasName, newVersion := parseUpdateValue(originalName, updateValue)

		switch {
		case manifest.Require[originalName] != "":
			if aliasName != originalName {
				// swap: add alias, remove original
				content, err = sjson.Set(content, "require."+aliasName, newVersion)
				if err != nil {
					return "", fmt.Errorf("failed to set require.%s: %w", aliasName, err)
				}
				content, err = sjson.Delete(content, "require."+originalName)
				if err != nil {
					return "", fmt.Errorf("failed to delete require.%s: %w", originalName, err)
				}
				p.logger.DebugContext(ctx, "swapped aliased package in require",
					slog.String("original", originalName), slog.String("alias", aliasName), slog.String("version", newVersion))
			} else {
				content, err = sjson.Set(content, "require."+originalName, newVersion)
				if err != nil {
					return "", fmt.Errorf("failed to set require.%s: %w", originalName, err)
				}
				p.logger.DebugContext(ctx, "updated require", slog.String("package", originalName), slog.String("version", newVersion))
			}

		case manifest.RequireDev[originalName] != "":
			if aliasName != originalName {
				content, err = sjson.Set(content, "require-dev."+aliasName, newVersion)
				if err != nil {
					return "", fmt.Errorf("failed to set require-dev.%s: %w", aliasName, err)
				}
				content, err = sjson.Delete(content, "require-dev."+originalName)
				if err != nil {
					return "", fmt.Errorf("failed to delete require-dev.%s: %w", originalName, err)
				}
				p.logger.DebugContext(ctx, "swapped aliased package in require-dev",
					slog.String("original", originalName), slog.String("alias", aliasName), slog.String("version", newVersion))
			} else {
				content, err = sjson.Set(content, "require-dev."+originalName, newVersion)
				if err != nil {
					return "", fmt.Errorf("failed to set require-dev.%s: %w", originalName, err)
				}
				p.logger.DebugContext(ctx, "updated require-dev", slog.String("package", originalName), slog.String("version", newVersion))
			}

		default:
			// Transitive dependency — pin it in require
			content, err = sjson.Set(content, "require."+aliasName, newVersion)
			if err != nil {
				return "", fmt.Errorf("failed to add transitive dep require.%s: %w", aliasName, err)
			}
			p.logger.DebugContext(ctx, "pinned transitive dep in require",
				slog.String("package", aliasName), slog.String("version", newVersion))
		}
	}

	content, err = p.injectRepository(content)
	if err != nil {
		return "", err
	}

	return content, nil
}

// injectRepository adds the pkg.root.io Composer repository entry if not already present.
func (p *ComposerParser) injectRepository(content string) (string, error) {
	const repoURL = "https://pkg.root.io/composer"

	repos := gjson.Get(content, "repositories")
	if repos.IsArray() {
		for _, r := range repos.Array() {
			if r.Get("url").String() == repoURL {
				return content, nil
			}
		}
	}

	entry := map[string]string{
		"type": "composer",
		"url":  repoURL,
	}
	idx := 0
	if repos.IsArray() {
		idx = len(repos.Array())
	}
	updated, err := sjson.Set(content, fmt.Sprintf("repositories.%d", idx), entry)
	if err != nil {
		return "", fmt.Errorf("failed to inject repositories entry: %w", err)
	}
	return updated, nil
}

// parseUpdateValue splits "aliasName:newVersion" into its parts.
// If no colon is present, the original name is kept.
func parseUpdateValue(originalName, updateValue string) (aliasName, newVersion string) {
	if idx := strings.LastIndex(updateValue, ":"); idx >= 0 {
		return updateValue[:idx], updateValue[idx+1:]
	}
	return originalName, updateValue
}

// Validate checks that content is valid JSON.
func (p *ComposerParser) Validate(content string) bool {
	return json.Valid([]byte(content))
}
