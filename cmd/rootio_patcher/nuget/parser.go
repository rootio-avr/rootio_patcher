package nuget

import (
	"context"
	"encoding/xml"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"rootio_patcher/cmd/rootio_patcher/common"
)

// NuGetParser handles parsing of NuGet manifest files (.csproj and packages.config).
type NuGetParser struct {
	logger *slog.Logger
}

// NewParser creates a new NuGet parser.
func NewParser(logger *slog.Logger) *NuGetParser {
	return &NuGetParser{logger: logger}
}

// Ecosystem returns the NuGet ecosystem identifier.
func (p *NuGetParser) Ecosystem() common.Ecosystem {
	return common.EcosystemNuGet
}

// FilePatterns returns glob patterns for NuGet manifest files.
func (p *NuGetParser) FilePatterns() []string {
	return []string{"*.csproj", "packages.config"}
}

// CanHandle reports whether the given filename is a NuGet manifest.
func (p *NuGetParser) CanHandle(fileName string) bool {
	base := filepath.Base(fileName)
	return base == "packages.config" || strings.HasSuffix(base, ".csproj")
}

// Parse parses a NuGet manifest file or directory.
// If filePath is a directory, it auto-discovers all *.csproj and packages.config files.
// If filePath is a file, it parses that file directly.
func (p *NuGetParser) Parse(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("cannot access %s: %w", filePath, err)
	}

	if info.IsDir() {
		return p.parseDirectory(ctx, filePath)
	}
	return p.parseFile(ctx, filePath)
}

// parseDirectory walks a directory tree and parses all discovered NuGet manifests.
func (p *NuGetParser) parseDirectory(ctx context.Context, dir string) ([]common.PackageInfo, error) {
	seen := make(map[string]common.PackageInfo)

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := info.Name()
			if name == "bin" || name == "obj" || name == ".git" || name == "node_modules" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !p.CanHandle(info.Name()) {
			return nil
		}

		pkgs, err := p.parseFile(ctx, path)
		if err != nil {
			p.logger.WarnContext(ctx, "skipping file due to parse error",
				slog.String("file", path), slog.Any("error", err))
			return nil
		}
		for _, pkg := range pkgs {
			pkg.Location = path
			seen[pkg.Name] = pkg
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("directory walk failed: %w", err)
	}

	packages := make([]common.PackageInfo, 0, len(seen))
	for _, pkg := range seen {
		packages = append(packages, pkg)
	}
	return packages, nil
}

// parseFile dispatches to the appropriate format parser based on filename.
func (p *NuGetParser) parseFile(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
	base := filepath.Base(filePath)
	if base == "packages.config" {
		return p.parsePackagesConfig(filePath)
	}
	if strings.HasSuffix(base, ".csproj") {
		return p.parseCsproj(filePath)
	}
	return nil, fmt.Errorf("unsupported file type: %s", base)
}

// --- XML structures for packages.config ---

type packagesConfig struct {
	XMLName  xml.Name       `xml:"packages"`
	Packages []packageEntry `xml:"package"`
}

type packageEntry struct {
	ID      string `xml:"id,attr"`
	Version string `xml:"version,attr"`
}

// parsePackagesConfig parses a packages.config file.
func (p *NuGetParser) parsePackagesConfig(filePath string) ([]common.PackageInfo, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", filePath, err)
	}

	var cfg packagesConfig
	if err := xml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", filePath, err)
	}

	var packages []common.PackageInfo
	for _, pkg := range cfg.Packages {
		if pkg.ID == "" || pkg.Version == "" {
			continue
		}
		packages = append(packages, common.PackageInfo{
			Name:              pkg.ID,
			Version:           pkg.Version,
			VersionConstraint: pkg.Version,
			Ecosystem:         common.EcosystemNuGet,
			Direct:            true,
			Location:          filePath,
		})
	}
	return packages, nil
}

// --- XML structures for .csproj ---

type csprojProject struct {
	XMLName    xml.Name    `xml:"Project"`
	ItemGroups []itemGroup `xml:"ItemGroup"`
}

type itemGroup struct {
	PackageReferences []packageReference `xml:"PackageReference"`
}

// packageReference handles both attribute-style and child-element-style versions:
//
//	<PackageReference Include="Foo" Version="1.0" />
//	<PackageReference Include="Foo"><Version>1.0</Version></PackageReference>
type packageReference struct {
	Include        string `xml:"Include,attr"`
	VersionAttr    string `xml:"Version,attr"`
	VersionElement string `xml:"Version"`
}

func (r packageReference) version() string {
	if r.VersionAttr != "" {
		return r.VersionAttr
	}
	return r.VersionElement
}

// parseCsproj parses a .csproj file and returns all PackageReference entries.
func (p *NuGetParser) parseCsproj(filePath string) ([]common.PackageInfo, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", filePath, err)
	}

	var project csprojProject
	if err := xml.Unmarshal(data, &project); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", filePath, err)
	}

	var packages []common.PackageInfo
	for _, ig := range project.ItemGroups {
		for _, ref := range ig.PackageReferences {
			if ref.Include == "" {
				continue
			}
			ver := ref.version()
			if ver == "" {
				p.logger.Debug("skipping PackageReference with no version", slog.String("package", ref.Include))
				continue
			}
			packages = append(packages, common.PackageInfo{
				Name:              ref.Include,
				Version:           ver,
				VersionConstraint: ver,
				Ecosystem:         common.EcosystemNuGet,
				Direct:            true,
				Location:          filePath,
			})
		}
	}
	return packages, nil
}

// Update rewrites package references in a NuGet manifest file.
// The updates map is: original package name -> "aliasName:aliasVersion".
// Both the package name (Include/id attribute) and version are rewritten.
// Returns the updated file content as a string.
func (p *NuGetParser) Update(ctx context.Context, filePath string, updates map[string]string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", filePath, err)
	}

	base := filepath.Base(filePath)
	content := string(data)

	if base == "packages.config" {
		return p.updatePackagesConfig(ctx, content, updates), nil
	}
	if strings.HasSuffix(base, ".csproj") {
		return p.updateCsproj(ctx, content, updates), nil
	}
	return "", fmt.Errorf("unsupported file type: %s", base)
}

// parseUpdateValue splits "aliasName:aliasVersion" into its parts.
// If the value has no colon, the original name is kept and the whole value is the version.
func parseUpdateValue(originalName, updateValue string) (aliasName, aliasVersion string) {
	if idx := strings.LastIndex(updateValue, ":"); idx >= 0 {
		return updateValue[:idx], updateValue[idx+1:]
	}
	return originalName, updateValue
}

// updateCsproj replaces PackageReference Include name and Version in a .csproj file.
// Handles both attribute style and child element style.
func (p *NuGetParser) updateCsproj(ctx context.Context, content string, updates map[string]string) string {
	for pkgName, updateValue := range updates {
		aliasName, aliasVersion := parseUpdateValue(pkgName, updateValue)

		// Attribute style: <PackageReference Include="PkgName" ... Version="old" ... />
		attrPattern := regexp.MustCompile(
			`(?i)(<PackageReference\s[^>]*Include\s*=\s*")` + regexp.QuoteMeta(pkgName) + `("[^>]*\s)Version\s*=\s*"[^"]*"`,
		)
		if attrPattern.MatchString(content) {
			content = attrPattern.ReplaceAllString(content, `${1}`+aliasName+`${2}Version="`+aliasVersion+`"`)
			p.logger.DebugContext(ctx, "updated csproj attribute reference",
				slog.String("package", pkgName), slog.String("alias", aliasName), slog.String("version", aliasVersion))
			continue
		}

		// Child element style: <PackageReference Include="PkgName"><Version>old</Version></PackageReference>
		elemPattern := regexp.MustCompile(
			`(?is)(<PackageReference\s[^>]*Include\s*=\s*")` + regexp.QuoteMeta(pkgName) + `("[^>]*>.*?<Version>)[^<]*(</Version>)`,
		)
		if elemPattern.MatchString(content) {
			content = elemPattern.ReplaceAllString(content, `${1}`+aliasName+`${2}`+aliasVersion+`${3}`)
			p.logger.DebugContext(ctx, "updated csproj element reference",
				slog.String("package", pkgName), slog.String("alias", aliasName), slog.String("version", aliasVersion))
			continue
		}

		p.logger.WarnContext(ctx, "package not found in csproj, skipping", slog.String("package", pkgName))
	}
	return content
}

// updatePackagesConfig replaces package id and version in a packages.config file.
func (p *NuGetParser) updatePackagesConfig(ctx context.Context, content string, updates map[string]string) string {
	for pkgName, updateValue := range updates {
		aliasName, aliasVersion := parseUpdateValue(pkgName, updateValue)

		pattern := regexp.MustCompile(
			`(?i)(<package\s[^>]*id\s*=\s*")` + regexp.QuoteMeta(pkgName) + `("[^>]*\s)version\s*=\s*"[^"]*"`,
		)
		if pattern.MatchString(content) {
			content = pattern.ReplaceAllString(content, `${1}`+aliasName+`${2}version="`+aliasVersion+`"`)
			p.logger.DebugContext(ctx, "updated packages.config reference",
				slog.String("package", pkgName), slog.String("alias", aliasName), slog.String("version", aliasVersion))
		} else {
			p.logger.WarnContext(ctx, "package not found in packages.config, skipping", slog.String("package", pkgName))
		}
	}
	return content
}

// Validate checks that content is valid XML.
func (p *NuGetParser) Validate(content string) bool {
	return xml.Unmarshal([]byte(content), new(interface{})) == nil
}
