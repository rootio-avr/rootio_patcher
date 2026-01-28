package maven

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rootio_patcher/cmd/rootio_patcher/common"
)

func TestMavenParser_Ecosystem(t *testing.T) {
	parser := NewParser()
	if parser.Ecosystem() != common.EcosystemMaven {
		t.Errorf("Expected ecosystem 'maven', got '%s'", parser.Ecosystem())
	}
}

func TestMavenParser_FilePatterns(t *testing.T) {
	parser := NewParser()
	patterns := parser.FilePatterns()

	if len(patterns) != 1 {
		t.Fatalf("Expected 1 pattern, got %d", len(patterns))
	}

	if patterns[0] != "pom.xml" {
		t.Errorf("Expected pattern 'pom.xml', got '%s'", patterns[0])
	}
}

func TestMavenParser_CanHandle(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		fileName string
		expected bool
	}{
		{"pom.xml", true},
		{"package-lock.json", false},
		{"requirements.txt", false},
		{"build.gradle", false},
	}

	for _, tt := range tests {
		t.Run(tt.fileName, func(t *testing.T) {
			result := parser.CanHandle(tt.fileName)
			if result != tt.expected {
				t.Errorf("Expected CanHandle('%s') = %v, got %v", tt.fileName, tt.expected, result)
			}
		})
	}
}

func TestMavenParser_Parse(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()

	tmpDir := t.TempDir()
	pomFile := filepath.Join(tmpDir, "pom.xml")

	content := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <groupId>com.example</groupId>
    <artifactId>test-project</artifactId>
    <version>1.0.0</version>

    <properties>
        <log4j.version>2.17.0</log4j.version>
    </properties>

    <dependencies>
        <dependency>
            <groupId>org.apache.logging.log4j</groupId>
            <artifactId>log4j-core</artifactId>
            <version>${log4j.version}</version>
        </dependency>
        <dependency>
            <groupId>junit</groupId>
            <artifactId>junit</artifactId>
            <version>4.13.2</version>
            <scope>test</scope>
        </dependency>
    </dependencies>
</project>`

	if err := os.WriteFile(pomFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	packages, err := parser.Parse(ctx, pomFile)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(packages) != 2 {
		t.Fatalf("Expected 2 packages, got %d", len(packages))
	}

	// Check log4j package
	log4j := packages[0]
	if log4j.Name != "org.apache.logging.log4j:log4j-core" {
		t.Errorf("Expected package name 'org.apache.logging.log4j:log4j-core', got '%s'", log4j.Name)
	}
	if log4j.Version != "2.17.0" {
		t.Errorf("Expected version '2.17.0', got '%s'", log4j.Version)
	}
	if !log4j.Direct {
		t.Error("Expected log4j to be marked as direct dependency")
	}
	if log4j.Dev {
		t.Error("Expected log4j to NOT be marked as dev dependency")
	}
	if log4j.Ecosystem != common.EcosystemMaven {
		t.Errorf("Expected ecosystem 'maven', got '%s'", log4j.Ecosystem)
	}

	// Check junit package
	junit := packages[1]
	if junit.Name != "junit:junit" {
		t.Errorf("Expected package name 'junit:junit', got '%s'", junit.Name)
	}
	if junit.Version != "4.13.2" {
		t.Errorf("Expected version '4.13.2', got '%s'", junit.Version)
	}
	if !junit.Direct {
		t.Error("Expected junit to be marked as direct dependency")
	}
	if !junit.Dev {
		t.Error("Expected junit to be marked as dev dependency (test scope)")
	}
}

func TestMavenParser_Parse_WithoutNamespace(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()

	tmpDir := t.TempDir()
	pomFile := filepath.Join(tmpDir, "pom.xml")

	// POM without namespace declaration
	content := `<?xml version="1.0" encoding="UTF-8"?>
<project>
    <modelVersion>4.0.0</modelVersion>
    <groupId>com.example</groupId>
    <artifactId>test-project</artifactId>
    <version>1.0.0</version>

    <dependencies>
        <dependency>
            <groupId>commons-lang</groupId>
            <artifactId>commons-lang</artifactId>
            <version>2.6</version>
        </dependency>
    </dependencies>
</project>`

	if err := os.WriteFile(pomFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	packages, err := parser.Parse(ctx, pomFile)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(packages) != 1 {
		t.Fatalf("Expected 1 package, got %d", len(packages))
	}

	if packages[0].Name != "commons-lang:commons-lang" {
		t.Errorf("Expected package name 'commons-lang:commons-lang', got '%s'", packages[0].Name)
	}
}

func TestMavenParser_Parse_FileNotFound(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()

	_, err := parser.Parse(ctx, "/nonexistent/pom.xml")
	if err == nil {
		t.Fatal("Expected error for nonexistent file, got nil")
	}
}

func TestMavenParser_Parse_InvalidXML(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()

	tmpDir := t.TempDir()
	pomFile := filepath.Join(tmpDir, "pom.xml")

	content := `<project>invalid xml`
	if err := os.WriteFile(pomFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	_, err := parser.Parse(ctx, pomFile)
	if err == nil {
		t.Fatal("Expected error for invalid XML, got nil")
	}
}

func TestMavenParser_Update_DirectVersion(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()

	tmpDir := t.TempDir()
	pomFile := filepath.Join(tmpDir, "pom.xml")

	content := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <dependencies>
        <dependency>
            <groupId>junit</groupId>
            <artifactId>junit</artifactId>
            <version>4.12</version>
        </dependency>
    </dependencies>
</project>`

	if err := os.WriteFile(pomFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	updates := map[string]string{
		"junit:junit": "4.13.2",
	}

	updated, err := parser.Update(ctx, pomFile, updates)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify the update contains the new version
	if !strings.Contains(updated, "4.13.2") {
		t.Error("Expected updated content to contain new version '4.13.2'")
	}

	// Verify old version is not present
	if strings.Contains(updated, "<version>4.12</version>") {
		t.Error("Expected old version '4.12' to be replaced")
	}
}

func TestMavenParser_Update_PropertyVersion(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()

	tmpDir := t.TempDir()
	pomFile := filepath.Join(tmpDir, "pom.xml")

	content := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <properties>
        <log4j.version>2.17.0</log4j.version>
    </properties>
    <dependencies>
        <dependency>
            <groupId>org.apache.logging.log4j</groupId>
            <artifactId>log4j-core</artifactId>
            <version>${log4j.version}</version>
        </dependency>
    </dependencies>
</project>`

	if err := os.WriteFile(pomFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	updates := map[string]string{
		"org.apache.logging.log4j:log4j-core": "2.17.1",
	}

	updated, err := parser.Update(ctx, pomFile, updates)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify the property was updated
	if !strings.Contains(updated, "<log4j.version>2.17.1</log4j.version>") {
		t.Error("Expected property to be updated to '2.17.1'")
	}
}

func TestMavenParser_Validate(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "valid XML",
			content:  `<?xml version="1.0"?><project></project>`,
			expected: true,
		},
		{
			name:     "invalid XML",
			content:  `<project>unclosed`,
			expected: false,
		},
		{
			name:     "empty string",
			content:  ``,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parser.Validate(tt.content)
			if result != tt.expected {
				t.Errorf("Expected Validate() = %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestMavenParser_ResolveProperty(t *testing.T) {
	parser := NewParser()

	properties := map[string]string{
		"foo.version": "1.2.3",
		"bar.version": "4.5.6",
	}

	tests := []struct {
		name     string
		value    string
		expected string
	}{
		{"property reference", "${foo.version}", "1.2.3"},
		{"different property", "${bar.version}", "4.5.6"},
		{"direct version", "1.0.0", "1.0.0"},
		{"empty string", "", ""},
		{"unknown property", "${unknown}", "${unknown}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parser.resolveProperty(tt.value, properties)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}
