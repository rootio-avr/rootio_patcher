package maven

import (
	"context"
	"encoding/xml"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"rootio_patcher/cmd/rootio_patcher/common"
)

// Parser handles parsing of Maven pom.xml files
type MavenParser struct {
	logger *slog.Logger
}

// NewParser creates a new Maven parser
func NewParser(logger *slog.Logger) *MavenParser {
	return &MavenParser{
		logger: logger,
	}
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

	p.logger.InfoContext(ctx, "Scanning Maven project", slog.String("source_dir", sourceDir))

	// Track unique dependencies by groupId:artifactId
	dependenciesMap := make(map[string]common.PackageInfo)

	// Step 1: Find all pom.xml files in the project
	pomFiles, err := p.findAllPomFiles(sourceDir)
	if err != nil || len(pomFiles) == 0 {
		return nil, fmt.Errorf("no Maven POM files found. Expected pom.xml files")
	}

	p.logger.InfoContext(ctx, "Found POM files", slog.Int("count", len(pomFiles)))

	// Step 1.5: Build module registry to filter out inter-module dependencies
	projectGroupIDs, projectModules := p.buildModuleRegistry(pomFiles)
	if len(projectGroupIDs) > 0 {
		p.logger.DebugContext(ctx, "Found project groupIds", slog.Any("group_ids", projectGroupIDs))
	}
	if len(projectModules) > 0 {
		p.logger.DebugContext(ctx, "Found project modules", slog.Int("count", len(projectModules)))
	}

	// Step 2: Find the root/aggregator POM using smart detection
	rootPom := p.findRootPom(pomFiles, sourceDir)

	if rootPom != "" {
		// We found a root POM - generate single effective POM from it
		relPath, _ := filepath.Rel(sourceDir, rootPom)
		p.logger.InfoContext(ctx, "Using root POM", slog.String("path", relPath))

		effectivePom := p.tryGenerateEffectivePom(sourceDir, rootPom)
		if effectivePom != "" {
			deps, err := p.parseDirectDependencies(effectivePom)
			if err != nil {
				return nil, fmt.Errorf("failed to parse root POM %s: %w", rootPom, err)
			}
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
		p.logger.WarnContext(ctx, "Could not determine root POM, processing all POMs individually", slog.Int("count", len(pomFiles)))

		for _, pomFile := range pomFiles {
			relPath, _ := filepath.Rel(sourceDir, pomFile)
			p.logger.DebugContext(ctx, "Processing POM", slog.String("path", relPath))

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

	p.logger.InfoContext(ctx, "Parsed direct dependencies", slog.Int("count", len(dependenciesMap)))

	// Step 3: Parse transitive dependencies using mvn dependency:tree
	pomToUse := rootPom
	if pomToUse == "" {
		// Use the originally provided file if no root found
		pomToUse = filePath
	}

	transitiveDeps, treeSuccess := p.parseTransitiveDependencies(ctx, pomToUse)
	if !treeSuccess {
		p.logger.WarnContext(ctx, "mvn dependency:tree failed, results may be incomplete")
	}

	for _, dep := range transitiveDeps {
		key := dep.Name
		// Only add if not already present (direct deps take precedence)
		if _, exists := dependenciesMap[key]; !exists {
			dep.Direct = false
			dependenciesMap[key] = dep
		}
	}

	p.logger.InfoContext(ctx, "Total unique dependencies", slog.Int("count", len(dependenciesMap)))

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
		p.logger.InfoContext(ctx, "Filtered out inter-module dependencies", slog.Int("count", filteredCount))
	}

	sort.Slice(packages, func(i, j int) bool {
		return packages[i].Name < packages[j].Name
	})

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
		p.logger.ErrorContext(ctx, "mvn dependency:tree failed", slog.Any("error", err))
		if len(output) > 0 {
			// Show first 500 chars of error output
			errMsg := string(output)
			if len(errMsg) > 500 {
				errMsg = errMsg[:500] + "..."
			}
			p.logger.DebugContext(ctx, "Maven output", slog.String("output", errMsg))
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

	p.logger.InfoContext(ctx, "Found transitive dependencies", slog.Int("count", len(dependencies)))
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

// propertyRefName returns the property name and true if value is a Maven
// property reference like ${log4j.version}.
func propertyRefName(value string) (string, bool) {
	if !strings.HasPrefix(value, "${") || !strings.HasSuffix(value, "}") {
		return "", false
	}
	return strings.TrimSuffix(strings.TrimPrefix(value, "${"), "}"), true
}

// replacePropertyValue replaces a <properties> entry's value, e.g.
// <log4j.version>2.17.0</log4j.version> -> <log4j.version>2.17.1</log4j.version>
func (p *MavenParser) replacePropertyValue(content, propName, newValue string) string {
	pattern := fmt.Sprintf(`(<%s>)([^<]*)(</%s>)`, regexp.QuoteMeta(propName), regexp.QuoteMeta(propName))
	re := regexp.MustCompile(pattern)
	return re.ReplaceAllString(content, fmt.Sprintf("${1}%s${3}", newValue))
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

	// Build a list of all packages being patched (original groupId:artifactId)
	// These will be used for exclusions
	patchedPackages := make(map[string]struct {
		originalGroupID string
		artifactID      string
	})
	for originalName := range updates {
		parts := strings.SplitN(originalName, ":", 2)
		if len(parts) == 2 {
			patchedPackages[originalName] = struct {
				originalGroupID string
				artifactID      string
			}{
				originalGroupID: parts[0],
				artifactID:      parts[1],
			}
		}
	}

	// Track which dependencies are already in the POM
	existingDeps := make(map[string]bool)
	for _, dep := range project.Dependencies.Dependency {
		if dep.GroupID != "" && dep.ArtifactID != "" {
			existingDeps[fmt.Sprintf("%s:%s", dep.GroupID, dep.ArtifactID)] = true
		}
	}

	exclusionsAdded := 0
	dependenciesUpdated := 0

	// Step 1: Update existing direct dependencies and add exclusions
	// For each dependency: either update it (if being patched) or add exclusions (if not)
	for _, dep := range project.Dependencies.Dependency {
		if dep.GroupID == "" || dep.ArtifactID == "" {
			continue
		}

		name := fmt.Sprintf("%s:%s", dep.GroupID, dep.ArtifactID)

		if updateValue, ok := updates[name]; ok {
			// This dependency is being patched - update groupId and version
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

			if propName, isPropertyRef := propertyRefName(oldVersion); isPropertyRef && newGroupID == dep.GroupID {
				// Version is a property reference (e.g. ${log4j.version}) and groupId is
				// unchanged - update the property itself so all dependents stay in sync,
				// rather than replacing the tag content with a hardcoded version.
				updatedContent = p.replacePropertyValue(updatedContent, propName, newVersion)
			} else if newGroupID != dep.GroupID {
				updatedContent = p.replaceDependencyGroupIDAndVersion(updatedContent, dep.GroupID, dep.ArtifactID, oldVersion, newGroupID, newVersion)
			} else {
				updatedContent = p.replaceDependencyVersion(updatedContent, dep.GroupID, dep.ArtifactID, oldVersion, newVersion)
			}
			dependenciesUpdated++
		} else {
			// This dependency is NOT being patched - add exclusions for all patched packages
			// This prevents this dependency from transitively pulling in vulnerable versions
			count := p.addExclusionsToDependency(&updatedContent, dep.GroupID, dep.ArtifactID, patchedPackages)
			exclusionsAdded += count
		}
	}

	p.logger.InfoContext(ctx, "Updated dependencies and added exclusions",
		slog.Int("updated", dependenciesUpdated),
		slog.Int("exclusions_added", exclusionsAdded))

	// Step 2: Add explicit Root.io dependencies for transitive ones (not already in POM)
	// This matches Python behavior: maven_dependency_updater.py lines 350-367
	for originalName, updateValue := range updates {
		// Parse the original name and update value
		originalParts := strings.SplitN(originalName, ":", 2)
		if len(originalParts) != 2 {
			continue
		}
		originalGroupID := originalParts[0]
		artifactID := originalParts[1]

		// Parse update value: "newGroupId:artifactId:newVersion"
		updateParts := strings.Split(updateValue, ":")
		if len(updateParts) < 3 {
			continue // Skip if not in the expected format
		}
		newGroupID := strings.Join(updateParts[:len(updateParts)-2], ":")
		newVersion := updateParts[len(updateParts)-1]

		// Check if this dependency is already in the POM (either original or updated form)
		originalKey := fmt.Sprintf("%s:%s", originalGroupID, artifactID)
		newKey := fmt.Sprintf("%s:%s", newGroupID, artifactID)

		if !existingDeps[originalKey] && !existingDeps[newKey] {
			// This is a transitive dependency - add it explicitly to the POM
			// Insert after <dependencies> tag
			updatedContent = p.addDependency(updatedContent, newGroupID, artifactID, newVersion)
			// Track that we've added it
			existingDeps[newKey] = true
		}
	}

	return updatedContent, nil
}

// addExclusionsToDependency adds exclusions to a specific dependency
// This prevents the dependency from transitively pulling in vulnerable versions
// Returns the number of exclusions added
func (p *MavenParser) addExclusionsToDependency(content *string, depGroupID, depArtifactID string, patchedPackages map[string]struct {
	originalGroupID string
	artifactID      string
}) int {
	exclusionsAdded := 0

	// Find the dependency block for this specific groupId:artifactId
	// Pattern matches: <dependency>...groupId...artifactId...</dependency>
	depPattern := fmt.Sprintf(
		`(?s)(<dependency>\s*)(<groupId>%s</groupId>\s*<artifactId>%s</artifactId>)(.*?)(</dependency>)`,
		regexp.QuoteMeta(depGroupID),
		regexp.QuoteMeta(depArtifactID),
	)

	depRe := regexp.MustCompile(depPattern)
	depMatch := depRe.FindStringSubmatch(*content)
	if depMatch == nil {
		// Dependency not found, skip
		return 0
	}

	depBlockContent := depMatch[3] // The content between artifactId and </dependency>

	// Check if this dependency already has an <exclusions> section
	hasExclusions := strings.Contains(depBlockContent, "<exclusions>")

	var exclusionsContent string
	if hasExclusions {
		// Extract existing exclusions content
		exclusionsPattern := regexp.MustCompile(`(?s)<exclusions>(.*?)</exclusions>`)
		exclusionsMatch := exclusionsPattern.FindStringSubmatch(depBlockContent)
		if exclusionsMatch != nil {
			exclusionsContent = exclusionsMatch[1]
		}
	}

	// For each patched package, add an exclusion if not already present
	newExclusions := ""
	for _, pkg := range patchedPackages {
		// Check if exclusion already exists
		exclusionPattern := fmt.Sprintf(
			`<exclusion>\s*<groupId>%s</groupId>\s*<artifactId>%s</artifactId>\s*</exclusion>`,
			regexp.QuoteMeta(pkg.originalGroupID),
			regexp.QuoteMeta(pkg.artifactID),
		)
		exclusionRe := regexp.MustCompile(exclusionPattern)
		if exclusionRe.MatchString(exclusionsContent) {
			// Exclusion already exists, skip
			continue
		}

		// Add new exclusion
		newExclusions += fmt.Sprintf(`
            <exclusion>
                <groupId>%s</groupId>
                <artifactId>%s</artifactId>
            </exclusion>`, pkg.originalGroupID, pkg.artifactID)
		exclusionsAdded++
	}

	if exclusionsAdded > 0 {
		if hasExclusions {
			// Append to existing exclusions section
			*content = strings.Replace(*content, depMatch[0],
				depMatch[1]+depMatch[2]+strings.Replace(depBlockContent,
					"</exclusions>",
					newExclusions+"\n        </exclusions>",
					1)+depMatch[4],
				1)
		} else {
			// Create new exclusions section after <version> (or after artifactId if no version)
			exclusionsBlock := fmt.Sprintf(`
        <exclusions>%s
        </exclusions>`, newExclusions)

			// Insert exclusions after version tag (if exists) or after artifactId
			versionPattern := regexp.MustCompile(`(<version>[^<]+</version>)`)
			if versionPattern.MatchString(depBlockContent) {
				newDepBlockContent := versionPattern.ReplaceAllString(depBlockContent, "${1}"+exclusionsBlock)
				*content = strings.Replace(*content, depMatch[0],
					depMatch[1]+depMatch[2]+newDepBlockContent+depMatch[4],
					1)
			} else {
				// No version tag, insert right after artifactId
				*content = strings.Replace(*content, depMatch[0],
					depMatch[1]+depMatch[2]+exclusionsBlock+depBlockContent+depMatch[4],
					1)
			}
		}
	}

	return exclusionsAdded
}

// addDependency adds a new dependency to the <dependencies> section
// This is used for transitive dependencies that need to be made explicit
func (p *MavenParser) addDependency(content, groupID, artifactID, version string) string {
	// Create the new dependency block with proper indentation
	newDep := fmt.Sprintf(`
        <dependency>
            <groupId>%s</groupId>
            <artifactId>%s</artifactId>
            <version>%s</version>
        </dependency>`, groupID, artifactID, version)

	// Find the <dependencies> section and insert after the opening tag
	// Pattern: <dependencies> with optional whitespace
	pattern := regexp.MustCompile(`(<dependencies>\s*)`)
	if pattern.MatchString(content) {
		content = pattern.ReplaceAllString(content, "${1}"+newDep+"\n        ")
	}

	return content
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
	return re.ReplaceAllString(content, fmt.Sprintf("${1}%s${3}", newVersion))
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
