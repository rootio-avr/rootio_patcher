package npm

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPnpmParser_Ecosystem(t *testing.T) {
	parser := NewPnpmParser()
	if parser.Ecosystem() != "npm" {
		t.Errorf("Expected ecosystem 'npm', got '%s'", parser.Ecosystem())
	}
}

func TestPnpmParser_FilePatterns(t *testing.T) {
	parser := NewPnpmParser()
	patterns := parser.FilePatterns()
	if len(patterns) != 1 || patterns[0] != "pnpm-lock.yaml" {
		t.Errorf("Expected patterns [pnpm-lock.yaml], got %v", patterns)
	}
}

func TestPnpmParser_CanHandle(t *testing.T) {
	parser := NewPnpmParser()

	tests := []struct {
		name     string
		fileName string
		expected bool
	}{
		{"pnpm-lock.yaml", "pnpm-lock.yaml", true},
		{"package-lock.json", "package-lock.json", false},
		{"yarn.lock", "yarn.lock", false},
		{"pom.xml", "pom.xml", false},
		{"path/pnpm-lock.yaml", "some/path/pnpm-lock.yaml", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parser.CanHandle(tt.fileName)
			if result != tt.expected {
				t.Errorf("CanHandle(%s) = %v, want %v", tt.fileName, result, tt.expected)
			}
		})
	}
}

func TestPnpmParser_Parse_Basic(t *testing.T) {
	ctx := context.Background()
	parser := NewPnpmParser()

	// Create temp directory
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "pnpm-lock.yaml")

	// Create basic pnpm-lock.yaml
	content := `lockfileVersion: '9.0'

importers:
  .:
    dependencies:
      lodash:
        specifier: ^4.17.21
        version: 4.17.23
    devDependencies:
      jest:
        specifier: ^29.0.0
        version: 29.0.0

packages:
  lodash@4.17.23:
    resolution: {integrity: sha512-xxx}

  jest@29.0.0:
    resolution: {integrity: sha512-yyy}
`
	if err := os.WriteFile(lockFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create lock file: %v", err)
	}

	packages, err := parser.Parse(ctx, lockFile)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(packages) != 2 {
		t.Fatalf("Expected 2 packages, got %d", len(packages))
	}

	// Check lodash
	foundLodash := false
	foundJest := false

	for _, pkg := range packages {
		switch pkg.Name {
		case "lodash":
			foundLodash = true
			if pkg.Version != "4.17.23" {
				t.Errorf("Expected lodash version 4.17.23, got %s", pkg.Version)
			}
			if pkg.Dev {
				t.Error("Expected lodash to NOT be a dev dependency")
			}
		case "jest":
			foundJest = true
			if pkg.Version != "29.0.0" {
				t.Errorf("Expected jest version 29.0.0, got %s", pkg.Version)
			}
			if !pkg.Dev {
				t.Error("Expected jest to be a dev dependency")
			}
		}
	}

	if !foundLodash {
		t.Error("Expected to find lodash in parsed packages")
	}
	if !foundJest {
		t.Error("Expected to find jest in parsed packages")
	}
}

func TestPnpmParser_Parse_ScopedPackages(t *testing.T) {
	ctx := context.Background()
	parser := NewPnpmParser()

	// Create temp directory
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "pnpm-lock.yaml")

	// Create pnpm-lock.yaml with scoped packages
	content := `lockfileVersion: '9.0'

importers:
  .:
    dependencies:
      '@babel/runtime':
        specifier: ^7.25.0
        version: 7.25.0
      '@types/node':
        specifier: ^18.0.0
        version: 18.11.0

packages:
  '@babel/runtime@7.25.0':
    resolution: {integrity: sha512-xxx}

  '@types/node@18.11.0':
    resolution: {integrity: sha512-yyy}
`
	if err := os.WriteFile(lockFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create lock file: %v", err)
	}

	packages, err := parser.Parse(ctx, lockFile)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(packages) != 2 {
		t.Fatalf("Expected 2 packages, got %d", len(packages))
	}

	// Check scoped packages
	foundBabel := false
	foundTypes := false

	for _, pkg := range packages {
		switch pkg.Name {
		case "@babel/runtime":
			foundBabel = true
			if pkg.Version != "7.25.0" {
				t.Errorf("Expected @babel/runtime version 7.25.0, got %s", pkg.Version)
			}
		case "@types/node":
			foundTypes = true
			if pkg.Version != "18.11.0" {
				t.Errorf("Expected @types/node version 18.11.0, got %s", pkg.Version)
			}
		}
	}

	if !foundBabel {
		t.Error("Expected to find @babel/runtime in parsed packages")
	}
	if !foundTypes {
		t.Error("Expected to find @types/node in parsed packages")
	}
}

func TestPnpmParser_Parse_InvalidYAML(t *testing.T) {
	ctx := context.Background()
	parser := NewPnpmParser()

	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "pnpm-lock.yaml")

	// Invalid YAML
	content := `this is not valid yaml: {{{`
	if err := os.WriteFile(lockFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create lock file: %v", err)
	}

	_, err := parser.Parse(ctx, lockFile)
	if err == nil {
		t.Error("Expected error for invalid YAML, got nil")
	}
}

func TestPnpmParser_Parse_FileNotFound(t *testing.T) {
	ctx := context.Background()
	parser := NewPnpmParser()

	_, err := parser.Parse(ctx, "/nonexistent/pnpm-lock.yaml")
	if err == nil {
		t.Error("Expected error for nonexistent file, got nil")
	}
}

func TestParsePnpmPackageKey(t *testing.T) {
	tests := []struct {
		name            string
		key             string
		expectedName    string
		expectedVersion string
	}{
		{
			name:            "simple package",
			key:             "lodash@4.17.23",
			expectedName:    "lodash",
			expectedVersion: "4.17.23",
		},
		{
			name:            "scoped package",
			key:             "@babel/runtime@7.25.0",
			expectedName:    "@babel/runtime",
			expectedVersion: "7.25.0",
		},
		{
			name:            "deeply scoped package",
			key:             "@scope/sub-scope/package@1.2.3",
			expectedName:    "@scope/sub-scope/package",
			expectedVersion: "1.2.3",
		},
		{
			name:            "version with prerelease",
			key:             "package@1.0.0-beta.1",
			expectedName:    "package",
			expectedVersion: "1.0.0-beta.1",
		},
		{
			name:            "invalid: no version",
			key:             "package",
			expectedName:    "",
			expectedVersion: "",
		},
		{
			name:            "invalid: no name",
			key:             "@1.0.0",
			expectedName:    "",
			expectedVersion: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, version := parsePnpmPackageKey(tt.key)
			if name != tt.expectedName {
				t.Errorf("parsePnpmPackageKey(%s) name = %s, want %s", tt.key, name, tt.expectedName)
			}
			if version != tt.expectedVersion {
				t.Errorf("parsePnpmPackageKey(%s) version = %s, want %s", tt.key, version, tt.expectedVersion)
			}
		})
	}
}

func TestPnpmParser_Validate(t *testing.T) {
	parser := NewPnpmParser()

	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name: "valid pnpm-lock.yaml",
			content: `lockfileVersion: '9.0'
packages:
  lodash@4.17.23:
    resolution: {integrity: sha512-xxx}`,
			expected: true,
		},
		{
			name:     "empty string",
			content:  "",
			expected: false,
		},
		{
			name:     "invalid YAML",
			content:  `{{{invalid`,
			expected: false,
		},
		{
			name:     "no lockfileVersion",
			content:  `packages: {}`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parser.Validate(tt.content)
			if result != tt.expected {
				t.Errorf("Validate() = %v, want %v", result, tt.expected)
			}
		})
	}
}
