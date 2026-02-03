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
	Modules      Modules      `xml:"modules"`
}

// Modules represents the modules section
type Modules struct {
	Module []string `xml:"module"`
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
// Implements the full multi-module logic from dependency-updater-go
func (p *MavenParser) Parse(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
	// Get source directory (parent of pom.xml or project root)
	sourceDir := p.findProjectRoot(filePath)

	fmt.Printf("Scanning Maven project from: %s\n", sourceDir)

	// Track unique dependencies by groupId:artifactId
	dependenciesMap := make(map[string]common.PackageInfo)

	// Step 1: Find all pom.xml files in the project
	pomFiles, err := p.findAllPomFiles(sourceDir)
	if err != nil || len(pomFiles) == 0 {
		return nil, fmt.Errorf("no Maven POM files found. Expected pom.xml files")
	}

	fmt.Printf("Found %d POM file(s)\n", len(pomFiles))

	// Step 1.5: Build module registry to filter out inter-module dependencies
	projectGroupIDs, projectModules := p.buildModuleRegistry(pomFiles)
	if len(projectGroupIDs) > 0 {
		fmt.Printf("Project groupId(s): %v\n", projectGroupIDs)
	}
	if len(projectModules) > 0 {
		fmt.Printf("Project modules: %d artifact(s)\n", len(projectModules))
	}

	// Step 2: Find the root/aggregator POM using smart detection
	rootPom := p.findRootPom(pomFiles, sourceDir)

	if rootPom != "" {
		// We found a root POM - generate single effective POM from it
		relPath, _ := filepath.Rel(sourceDir, rootPom)
		fmt.Printf("Using root POM: %s\n", relPath)

		effectivePom := p.tryGenerateEffectivePom(sourceDir, rootPom)
		if effectivePom != "" {
			deps, _ := p.parseDirectDependencies(effectivePom)
			for _, dep := range deps {
				key := dep.Name
				dep.Direct = true
				dependenciesMap[key] = dep
			}

			// Clean up temporary effective POM (only if it's not the original)
			if effectivePom != rootPom {
				os.Remove(effectivePom)
			}
		}
	} else {
		// No clear root found - process all POMs individually
		fmt.Printf("⚠ Could not determine root POM, processing all %d POM file(s) individually\n", len(pomFiles))

		for _, pomFile := range pomFiles {
			relPath, _ := filepath.Rel(sourceDir, pomFile)
			fmt.Printf("  Processing: %s\n", relPath)

			effectivePom := p.tryGenerateEffectivePom(sourceDir, pomFile)
			if effectivePom != "" {
				deps, _ := p.parseDirectDependencies(effectivePom)
				for _, dep := range deps {
					key := dep.Name
					dep.Direct = true
					dependenciesMap[key] = dep
				}

				// Clean up temporary effective POM (only if it's not the original)
				if effectivePom != pomFile {
					os.Remove(effectivePom)
				}
			}
		}
	}

	fmt.Printf("Parsed %d unique direct dependencies\n", len(dependenciesMap))

	// Step 3: Parse transitive dependencies using mvn dependency:tree
	pomToUse := rootPom
	if pomToUse == "" {
		// Use the originally provided file if no root found
		pomToUse = filePath
	}

	transitiveDeps, treeSuccess := p.parseTransitiveDependencies(ctx, pomToUse)
	if !treeSuccess {
		fmt.Printf("Warning: mvn dependency:tree failed, results may be incomplete\n")
	}

	for _, dep := range transitiveDeps {
		key := dep.Name
		// Only add if not already present (direct deps take precedence)
		if _, exists := dependenciesMap[key]; !exists {
			dep.Direct = false
			dependenciesMap[key] = dep
		}
	}

	fmt.Printf("Total unique dependencies (direct + transitive): %d\n", len(dependenciesMap))

	// Convert map to slice, filtering out inter-module dependencies
	var packages []common.PackageInfo
	filteredCount := 0
	for _, pkg := range dependenciesMap {
		// Only include packages with resolved versions
		if pkg.Version == "" {
			continue
		}

		// Filter out inter-module dependencies
		// Format is "groupId:artifactId"
		parts := strings.SplitN(pkg.Name, ":", 2)
		if len(parts) == 2 {
			groupID := parts[0]
			artifactID := parts[1]

			// Check if this is an inter-module dependency
			isProjectGroupID := false
			for _, projectGroupID := range projectGroupIDs {
				if groupID == projectGroupID {
					isProjectGroupID = true
					break
				}
			}

			if isProjectGroupID && projectModules[artifactID] {
				// This is an inter-module dependency, skip it
				filteredCount++
				continue
			}
		}

		packages = append(packages, pkg)
	}

	if filteredCount > 0 {
		fmt.Printf("Filtered out %d inter-module dependencies\n", filteredCount)
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

// findProjectRoot determines the project root directory from a given pom.xml path
// Walks up the directory tree to find the topmost directory containing a pom.xml
func (p *MavenParser) findProjectRoot(filePath string) string {
	// Convert to absolute path if relative
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		// Fall back to original if abs() fails
		absPath = filePath
	}

	dir := filepath.Dir(absPath)
	lastDirWithPom := dir

	// Walk up to find the topmost directory with a pom.xml
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			break
		}

		parentPom := filepath.Join(parent, "pom.xml")
		if _, err := os.Stat(parentPom); err == nil {
			lastDirWithPom = parent
			dir = parent
		} else {
			break
		}
	}

	return lastDirWithPom
}

// buildModuleRegistry builds a registry of all module artifactIds and groupIds in the project
// This is used to filter out inter-module dependencies (internal project dependencies)
// Returns: (projectGroupIDs, projectModules)
func (p *MavenParser) buildModuleRegistry(pomFiles []string) ([]string, map[string]bool) {
	groupIDSet := make(map[string]bool)
	moduleSet := make(map[string]bool)

	// Extract all groupIds and artifactIds from all POMs
	for _, pomFile := range pomFiles {
		data, err := os.ReadFile(pomFile)
		if err != nil {
			continue
		}

		var project PomProjectInfo
		if err := xml.Unmarshal(data, &project); err != nil {
			continue
		}

		// Extract groupId (may be in <groupId> or inherited from <parent>)
		if project.GroupID != "" {
			groupIDSet[project.GroupID] = true
		}
		if project.Parent != nil && project.Parent.GroupID != "" {
			groupIDSet[project.Parent.GroupID] = true
		}

		// Extract artifactId (for module registry)
		if project.ArtifactID != "" {
			moduleSet[project.ArtifactID] = true
		}
	}

	// Convert groupID set to slice
	groupIDs := make([]string, 0, len(groupIDSet))
	for groupID := range groupIDSet {
		groupIDs = append(groupIDs, groupID)
	}

	return groupIDs, moduleSet
}

// findAllPomFiles scans directory tree for all pom.xml files (excluding target directories)
func (p *MavenParser) findAllPomFiles(sourceDir string) ([]string, error) {
	var pomFiles []string
	err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip target directories, build directories, and hidden directories
		if info.IsDir() {
			name := info.Name()
			if name == "target" || name == "build" || name == ".git" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}

		// Collect pom.xml files
		if info.Name() == "pom.xml" {
			pomFiles = append(pomFiles, path)
		}

		return nil
	})
	return pomFiles, err
}

// PomProjectInfo holds parsed POM metadata for root detection and module registry
type PomProjectInfo struct {
	XMLName    xml.Name `xml:"project"`
	GroupID    string   `xml:"groupId"`
	ArtifactID string   `xml:"artifactId"`
	Modules    struct {
		Module []string `xml:"module"`
	} `xml:"modules"`
	Parent *struct {
		GroupID string `xml:"groupId"`
	} `xml:"parent"`
}

// findRootPom finds the root/aggregator POM using smart detection logic
// This is the full algorithm from dependency-updater-go
func (p *MavenParser) findRootPom(pomFiles []string, sourceDir string) string {
	var candidates []string

	for _, pomFile := range pomFiles {
		data, err := os.ReadFile(pomFile)
		if err != nil {
			continue
		}

		var project PomProjectInfo
		if err := xml.Unmarshal(data, &project); err != nil {
			continue
		}

		hasModules := len(project.Modules.Module) > 0
		hasParent := project.Parent != nil

		// Aggregator POM: has modules but no parent (or is topmost parent)
		if hasModules && !hasParent {
			return pomFile
		}

		// Collect POMs with modules as candidates
		if hasModules {
			candidates = append(candidates, pomFile)
		}
	}

	// If we found candidates with modules, return the shallowest one (closest to source_dir)
	if len(candidates) > 0 {
		minDepth := -1
		var shallowest string
		for _, candidate := range candidates {
			relPath, _ := filepath.Rel(sourceDir, candidate)
			depth := len(strings.Split(relPath, string(filepath.Separator)))
			if minDepth == -1 || depth < minDepth {
				minDepth = depth
				shallowest = candidate
			}
		}
		return shallowest
	}

	// No aggregator found, try to find POM at source_dir root
	rootPom := filepath.Join(sourceDir, "pom.xml")
	for _, pom := range pomFiles {
		if pom == rootPom {
			return rootPom
		}
	}

	// No clear root found
	return ""
}

// parseTransitiveDependencies parses transitive dependencies using mvn dependency:tree
// Returns dependencies and a boolean indicating if the command succeeded
func (p *MavenParser) parseTransitiveDependencies(ctx context.Context, pomFile string) ([]common.PackageInfo, bool) {
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
		fmt.Printf("mvn dependency:tree failed: %v\n", err)
		if len(output) > 0 {
			// Show first 500 chars of error output
			errMsg := string(output)
			if len(errMsg) > 500 {
				errMsg = errMsg[:500] + "..."
			}
			fmt.Printf("Maven output: %s\n", errMsg)
		}
		return []common.PackageInfo{}, false
	}

	var dependencies []common.PackageInfo
	outputStr := string(output)

	// Strip ANSI color codes from Maven output
	ansiPattern := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	outputStr = ansiPattern.ReplaceAllString(outputStr, "")

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

	fmt.Printf("Found %d transitive dependencies via mvn dependency:tree\n", len(dependencies))
	return dependencies, true
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

			// Always replace both groupId and version, even if version is a property reference
			// This matches Python behavior: property references like ${netty.version} are
			// replaced with hardcoded Root.io versions like 4.1.118.Final-root.io.3
			if newGroupID != dep.GroupID {
				updatedContent = p.replaceDependencyGroupIDAndVersion(updatedContent, dep.GroupID, dep.ArtifactID, oldVersion, newGroupID, newVersion)
			} else {
				updatedContent = p.replaceDependencyVersion(updatedContent, dep.GroupID, dep.ArtifactID, oldVersion, newVersion)
			}
		}
	}

	return updatedContent, nil
}

// replaceDependencyGroupIDAndVersion replaces both groupId and version for a specific dependency
func (p *MavenParser) replaceDependencyGroupIDAndVersion(content, oldGroupID, artifactID, oldVersion, newGroupID, newVersion string) string {
	// Pattern to match a dependency block and replace both groupId and version
	// Use [^<]+ to match ANY version content (literal or property reference like ${netty.version})
	// This is copied from dependency-updater-go/maven.go line 383
	pattern := fmt.Sprintf(
		`(<dependency>\s*)<groupId>%s</groupId>(\s*<artifactId>%s</artifactId>\s*<version>)([^<]+)(</version>)`,
		regexp.QuoteMeta(oldGroupID),
		regexp.QuoteMeta(artifactID),
	)

	re := regexp.MustCompile(pattern)
	replacement := fmt.Sprintf("${1}<groupId>%s</groupId>${2}%s${4}", newGroupID, newVersion)
	return re.ReplaceAllString(content, replacement)
}

// replaceDependencyVersion replaces version for a specific dependency
func (p *MavenParser) replaceDependencyVersion(content, groupID, artifactID, oldVersion, newVersion string) string {
	// Pattern to match a dependency block and replace its version
	// Use [^<]+ to match ANY version content (literal or property reference)
	pattern := fmt.Sprintf(
		`(<dependency>\s*<groupId>%s</groupId>\s*<artifactId>%s</artifactId>\s*<version>)([^<]+)(</version>)`,
		regexp.QuoteMeta(groupID),
		regexp.QuoteMeta(artifactID),
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
