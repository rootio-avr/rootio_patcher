package maven

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
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

// Parse parses pom.xml and returns all dependencies (direct and transitive)
func (p *MavenParser) Parse(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
	// Get source directory (parent of pom.xml)
	sourceDir := filepath.Dir(filePath)

	// Track unique dependencies by groupId:artifactId
	dependenciesMap := make(map[string]common.PackageInfo)

	// Step 1: Parse direct dependencies from pom.xml
	directDeps, err := p.parseDirectDependencies(filePath)
	if err != nil {
		return nil, err
	}

	// Add direct dependencies to map
	for _, dep := range directDeps {
		key := dep.Name
		dep.Direct = true
		dependenciesMap[key] = dep
	}

	// Step 2: Parse transitive dependencies using mvn dependency:tree
	transitiveDeps := p.parseTransitiveDependencies(ctx, filePath)
	for _, dep := range transitiveDeps {
		key := dep.Name
		// Only add if not already present (direct deps take precedence)
		if _, exists := dependenciesMap[key]; !exists {
			dep.Direct = false
			dependenciesMap[key] = dep
		}
	}

	// Step 3: Try to use effective POM for better version resolution
	effectivePom := p.tryGenerateEffectivePom(sourceDir, filePath)
	if effectivePom != "" && effectivePom != filePath {
		// Parse effective POM to get resolved versions
		effectiveDeps, err := p.parseDirectDependencies(effectivePom)
		if err == nil {
			// Update versions from effective POM
			for _, dep := range effectiveDeps {
				if existing, exists := dependenciesMap[dep.Name]; exists && dep.Version != "" {
					existing.Version = dep.Version
					existing.VersionConstraint = dep.Version
					dependenciesMap[dep.Name] = existing
				}
			}
		}
		// Clean up effective POM
		os.Remove(effectivePom)
	}

	// Convert map to slice
	var packages []common.PackageInfo
	for _, pkg := range dependenciesMap {
		// Only include packages with resolved versions
		if pkg.Version != "" {
			packages = append(packages, pkg)
		}
	}

	return packages, nil
}

// parseDirectDependencies parses direct dependencies from a pom.xml file
func (p *MavenParser) parseDirectDependencies(filePath string) ([]common.PackageInfo, error) {
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

		// Skip dependencies without version (will be resolved from effective POM or transitive tree)
		if version == "" {
			version = dep.Version // Keep the property reference for now
		}

		// Maven package name format: groupId:artifactId
		name := fmt.Sprintf("%s:%s", dep.GroupID, dep.ArtifactID)

		isDev := dep.Scope == "test"

		packages = append(packages, common.PackageInfo{
			Name:              name,
			Version:           version,
			VersionConstraint: version,
			Ecosystem:         common.EcosystemMaven,
			Direct:            true,
			Dev:               isDev,
		})
	}

	return packages, nil
}

// tryGenerateEffectivePom attempts to generate an effective POM with all properties resolved
func (p *MavenParser) tryGenerateEffectivePom(sourceDir, pomFile string) string {
	dir := filepath.Dir(pomFile)
	base := filepath.Base(pomFile)
	effectivePomPath := filepath.Join(dir, ".effective-pom-"+base)

	args := []string{
		"help:effective-pom",
		"-f", pomFile,
		"-Doutput=" + effectivePomPath,
		"-q",
	}

	cmd := exec.Command("mvn", args...)
	cmd.Dir = sourceDir

	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()

	select {
	case err := <-done:
		if err == nil {
			if _, statErr := os.Stat(effectivePomPath); statErr == nil {
				return effectivePomPath
			}
		}
		return pomFile
	case <-time.After(30 * time.Second):
		cmd.Process.Kill()
		return pomFile
	}
}

// parseTransitiveDependencies parses transitive dependencies using mvn dependency:tree
func (p *MavenParser) parseTransitiveDependencies(ctx context.Context, pomFile string) []common.PackageInfo {
	args := []string{
		"dependency:tree",
		"-f", pomFile,
		"-DoutputType=text",
		"-DincludeScope=compile",
	}

	cmd := exec.Command("mvn", args...)
	cmd.Dir = filepath.Dir(pomFile)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// If dependency:tree fails, return empty list (not fatal)
		return []common.PackageInfo{}
	}

	var dependencies []common.PackageInfo
	outputStr := string(output)

	// Parse output looking for lines like:
	// [INFO] |  +- io.netty:netty-common:jar:4.1.118.Final:compile
	// [INFO] |  \- io.netty:netty-codec:jar:4.1.118.Final:compile
	pattern := regexp.MustCompile(`^\[INFO\]\s+[\|\\+\s\-]+\s+([^:]+):([^:]+):([^:]+):([^:]+):(.+)$`)

	for _, line := range strings.Split(outputStr, "\n") {
		matches := pattern.FindStringSubmatch(line)
		if len(matches) == 6 {
			groupID := matches[1]
			artifactID := matches[2]
			// matches[3] is packaging (e.g., jar, war) - not needed
			version := matches[4]
			scope := matches[5]

			if groupID != "" && artifactID != "" && version != "" {
				name := fmt.Sprintf("%s:%s", groupID, artifactID)
				isDev := scope == "test"

				dependencies = append(dependencies, common.PackageInfo{
					Name:              name,
					Version:           version,
					VersionConstraint: version,
					Ecosystem:         common.EcosystemMaven,
					Direct:            false,
					Dev:               isDev,
				})
			}
		}
	}

	return dependencies
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
// The updates map format is: "groupId:artifactId" -> "newGroupId:artifactId:newVersion"
// This supports changing both groupId and version for Root.io patched packages
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

		if updateValue, ok := updates[name]; ok {
			// Parse update value: can be either "newVersion" or "newGroupId:artifactId:newVersion"
			var newGroupID, newVersion string
			updateParts := strings.Split(updateValue, ":")

			if len(updateParts) >= 3 {
				// Format: "newGroupId:artifactId:newVersion"
				newGroupID = strings.Join(updateParts[:len(updateParts)-2], ":")
				newVersion = updateParts[len(updateParts)-1]
			} else {
				// Format: "newVersion" (groupId stays the same)
				newGroupID = dep.GroupID
				newVersion = updateValue
			}

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
				// Direct version - replace both groupId and version if needed
				if newGroupID != dep.GroupID {
					updatedContent = p.replaceDependencyGroupIDAndVersion(updatedContent, dep.GroupID, dep.ArtifactID, oldVersion, newGroupID, newVersion)
				} else {
					updatedContent = p.replaceDependencyVersion(updatedContent, dep.GroupID, dep.ArtifactID, oldVersion, newVersion)
				}
			}
		}
	}

	return updatedContent, nil
}

// replaceDependencyGroupIDAndVersion replaces both groupId and version for a specific dependency
func (p *MavenParser) replaceDependencyGroupIDAndVersion(content, oldGroupID, artifactID, oldVersion, newGroupID, newVersion string) string {
	// Pattern to match a dependency block and replace both groupId and version
	pattern := fmt.Sprintf(
		`(<dependency>\s*)<groupId>%s</groupId>(\s*<artifactId>%s</artifactId>\s*<version>)%s(</version>)`,
		regexp.QuoteMeta(oldGroupID),
		regexp.QuoteMeta(artifactID),
		regexp.QuoteMeta(oldVersion),
	)

	re := regexp.MustCompile(pattern)
	replacement := fmt.Sprintf("${1}<groupId>%s</groupId>${2}%s${3}", newGroupID, newVersion)
	return re.ReplaceAllString(content, replacement)
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

// UpdatePackageJSON is not applicable for Maven (Maven uses pom.xml, not package.json)
func (p *MavenParser) UpdatePackageJSON(ctx context.Context, overrides map[string]string) error {
	return fmt.Errorf("UpdatePackageJSON not supported for Maven - use Update() to modify pom.xml")
}
