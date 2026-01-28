package maven

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"regexp"
	"strings"
	"rootio_patcher/cmd/rootio_patcher/common"
)

// Parser handles parsing of Maven pom.xml files
type MavenParser struct{}

// NewParser creates a new Maven parser
func NewParser() *MavenParser {
	return &MavenParser{}
}

// Ecosystem returns the ecosystem name
func (p *MavenParser) Ecosystem() common.Ecosystem {
	return common.EcosystemMaven
}

// FilePatterns returns file patterns this parser handles
func (p *MavenParser) FilePatterns() []string {
	return []string{"pom.xml"}
}

// CanHandle checks if this parser can handle the given file
func (p *MavenParser) CanHandle(fileName string) bool {
	for _, pattern := range p.FilePatterns() {
		if fileName == pattern {
			return true
		}
	}
	return false
}

// Project represents a Maven POM structure
type Project struct {
	XMLName      xml.Name     `xml:"project"`
	GroupID      string       `xml:"groupId"`
	ArtifactID   string       `xml:"artifactId"`
	Version      string       `xml:"version"`
	Properties   Properties   `xml:"properties"`
	Dependencies Dependencies `xml:"dependencies"`
}

// Properties represents Maven properties
type Properties struct {
	Properties map[string]string `xml:",any"`
}

// UnmarshalXML custom unmarshaler for properties
func (p *Properties) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	p.Properties = make(map[string]string)

	for {
		t, err := d.Token()
		if err != nil {
			return err
		}

		switch se := t.(type) {
		case xml.StartElement:
			var value string
			if err := d.DecodeElement(&value, &se); err != nil {
				return err
			}
			p.Properties[se.Name.Local] = value
		case xml.EndElement:
			if se == start.End() {
				return nil
			}
		}
	}
}

// Dependencies represents the dependencies section
type Dependencies struct {
	Dependency []Dependency `xml:"dependency"`
}

// Dependency represents a single Maven dependency
type Dependency struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
	Scope      string `xml:"scope"`
}

// Parse parses pom.xml and returns all dependencies
func (p *MavenParser) Parse(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var project Project
	if err := xml.Unmarshal(content, &project); err != nil {
		return nil, fmt.Errorf("failed to parse XML: %w", err)
	}

	var packages []common.PackageInfo

	for _, dep := range project.Dependencies.Dependency {
		if dep.GroupID == "" || dep.ArtifactID == "" {
			continue
		}

		// Resolve version property references
		version := p.resolveProperty(dep.Version, project.Properties.Properties)

		// Skip dependencies without version (managed by parent/BOM)
		if version == "" {
			continue
		}

		// Maven package name format: groupId:artifactId
		name := fmt.Sprintf("%s:%s", dep.GroupID, dep.ArtifactID)

		isDev := dep.Scope == "test"

		packages = append(packages, common.PackageInfo{
			Name:              name,
			Version:           version,
			VersionConstraint: version,
			Ecosystem:         common.EcosystemMaven,
			Direct:            true, // Maven doesn't have lock files, all declared deps are "direct"
			Dev:               isDev,
		})
	}

	return packages, nil
}

// resolveProperty resolves Maven property references like ${log4j.version}
func (p *MavenParser) resolveProperty(value string, properties map[string]string) string {
	if value == "" || !strings.HasPrefix(value, "${") {
		return value
	}

	// Extract property name: ${foo.bar} -> foo.bar
	propName := strings.TrimSuffix(strings.TrimPrefix(value, "${"), "}")
	if resolved, ok := properties[propName]; ok {
		return resolved
	}

	return value
}

// Update updates dependency versions in pom.xml
func (p *MavenParser) Update(ctx context.Context, filePath string, updates map[string]string) (string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// Parse to get structure
	var project Project
	if err := xml.Unmarshal(content, &project); err != nil {
		return "", fmt.Errorf("failed to parse XML: %w", err)
	}

	// Work with raw content to preserve formatting
	updatedContent := string(content)

	for _, dep := range project.Dependencies.Dependency {
		if dep.GroupID == "" || dep.ArtifactID == "" {
			continue
		}

		name := fmt.Sprintf("%s:%s", dep.GroupID, dep.ArtifactID)

		if newVersion, ok := updates[name]; ok {
			oldVersion := dep.Version

			// If it's a property reference, update the property instead
			if strings.HasPrefix(oldVersion, "${") {
				propName := strings.TrimSuffix(strings.TrimPrefix(oldVersion, "${"), "}")
				if _, exists := project.Properties.Properties[propName]; exists {
					// Replace property value in content
					pattern := fmt.Sprintf(`(<%s>)[^<]*(</[^>]*>)`, regexp.QuoteMeta(propName))
					re := regexp.MustCompile(pattern)
					updatedContent = re.ReplaceAllString(updatedContent, fmt.Sprintf("${1}%s${2}", newVersion))
				}
			} else {
				// Direct version - replace in content
				updatedContent = p.replaceDependencyVersion(updatedContent, dep.GroupID, dep.ArtifactID, oldVersion, newVersion)
			}
		}
	}

	return updatedContent, nil
}

// replaceDependencyVersion replaces version for a specific dependency
func (p *MavenParser) replaceDependencyVersion(content, groupID, artifactID, oldVersion, newVersion string) string {
	// Pattern to match a dependency block and replace its version
	pattern := fmt.Sprintf(
		`(<dependency>\s*<groupId>%s</groupId>\s*<artifactId>%s</artifactId>\s*<version>)%s(</version>)`,
		regexp.QuoteMeta(groupID),
		regexp.QuoteMeta(artifactID),
		regexp.QuoteMeta(oldVersion),
	)

	re := regexp.MustCompile(pattern)
	return re.ReplaceAllString(content, fmt.Sprintf("${1}%s${2}", newVersion))
}

// Validate validates XML syntax
func (p *MavenParser) Validate(content string) bool {
	var project Project
	return xml.Unmarshal([]byte(content), &project) == nil
}
