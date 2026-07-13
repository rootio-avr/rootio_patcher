package npm

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"rootio_patcher/cmd/rootio_patcher/common"
)

func TestNpmParser_Ecosystem(t *testing.T) {
	parser := NewParser()
	if parser.Ecosystem() != common.EcosystemNpm {
		t.Errorf("Expected ecosystem 'npm', got '%s'", parser.Ecosystem())
	}
}

func TestNpmParser_FilePatterns(t *testing.T) {
	parser := NewParser()
	patterns := parser.FilePatterns()

	if len(patterns) != 1 {
		t.Fatalf("Expected 1 pattern, got %d", len(patterns))
	}

	if patterns[0] != "package-lock.json" {
		t.Errorf("Expected pattern 'package-lock.json', got '%s'", patterns[0])
	}
}

func TestNpmParser_CanHandle(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		fileName string
		expected bool
	}{
		{"package-lock.json", true},
		{"package.json", false},
		{"pom.xml", false},
		{"requirements.txt", false},
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

func TestNpmParser_Parse(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()

	// Create a temporary package-lock.json file
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "package-lock.json")

	content := `{
  "name": "test-project",
  "version": "1.0.0",
  "lockfileVersion": 3,
  "packages": {
    "": {
      "name": "test-project",
      "version": "1.0.0",
      "dependencies": {
        "lodash": "^4.17.21"
      },
      "devDependencies": {
        "jest": "^29.0.0"
      }
    },
    "node_modules/lodash": {
      "version": "4.17.21",
      "resolved": "https://registry.npmjs.org/lodash/-/lodash-4.17.21.tgz"
    },
    "node_modules/jest": {
      "version": "29.0.0",
      "resolved": "https://registry.npmjs.org/jest/-/jest-29.0.0.tgz",
      "dev": true
    }
  }
}`

	if err := os.WriteFile(lockFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	packages, err := parser.Parse(ctx, lockFile)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(packages) != 2 {
		t.Fatalf("Expected 2 packages, got %d", len(packages))
	}

	byName := make(map[string]common.PackageInfo, len(packages))
	for _, pkg := range packages {
		byName[pkg.Name] = pkg
	}

	// Check lodash package
	lodash, ok := byName["lodash"]
	if !ok {
		t.Fatalf("Expected package 'lodash' in results, got %v", packages)
	}
	if lodash.Version != "4.17.21" {
		t.Errorf("Expected version '4.17.21', got '%s'", lodash.Version)
	}
	if !lodash.Direct {
		t.Error("Expected lodash to be marked as direct dependency")
	}
	if lodash.Dev {
		t.Error("Expected lodash to NOT be marked as dev dependency")
	}
	if lodash.Ecosystem != common.EcosystemNpm {
		t.Errorf("Expected ecosystem 'npm', got '%s'", lodash.Ecosystem)
	}

	// Check jest package
	jest, ok := byName["jest"]
	if !ok {
		t.Fatalf("Expected package 'jest' in results, got %v", packages)
	}
	if jest.Version != "29.0.0" {
		t.Errorf("Expected version '29.0.0', got '%s'", jest.Version)
	}
	if !jest.Direct {
		t.Error("Expected jest to be marked as direct dependency")
	}
	if !jest.Dev {
		t.Error("Expected jest to be marked as dev dependency")
	}
}

func TestNpmParser_Parse_FileNotFound(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()

	_, err := parser.Parse(ctx, "/nonexistent/package-lock.json")
	if err == nil {
		t.Fatal("Expected error for nonexistent file, got nil")
	}
}

func TestNpmParser_Parse_InvalidJSON(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()

	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "package-lock.json")

	content := `{invalid json`
	if err := os.WriteFile(lockFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	_, err := parser.Parse(ctx, lockFile)
	if err == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}
}

func TestNpmParser_Update(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()

	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "package-lock.json")

	content := `{
  "name": "test-project",
  "version": "1.0.0",
  "lockfileVersion": 3,
  "packages": {
    "": {
      "name": "test-project"
    },
    "node_modules/lodash": {
      "version": "4.17.20",
      "resolved": "https://registry.npmjs.org/lodash/-/lodash-4.17.20.tgz"
    }
  }
}`

	if err := os.WriteFile(lockFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	updates := map[string]string{
		"lodash": "4.17.21",
	}

	updated, err := parser.Update(ctx, lockFile, updates)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify the update contains the new version
	if !contains(updated, "4.17.21") {
		t.Error("Expected updated content to contain new version '4.17.21'")
	}

	// Verify the resolved URL was updated
	if !contains(updated, "lodash-4.17.21.tgz") {
		t.Error("Expected resolved URL to be updated")
	}
}

func TestNpmParser_Validate(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "valid JSON",
			content:  `{"name": "test", "version": "1.0.0"}`,
			expected: true,
		},
		{
			name:     "invalid JSON",
			content:  `{invalid}`,
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

func TestExtractPackageName(t *testing.T) {
	tests := []struct {
		pkgPath  string
		expected string
	}{
		{"node_modules/lodash", "lodash"},
		{"node_modules/@types/node", "@types/node"},
		{"node_modules/foo/node_modules/bar", "bar"},
		{"node_modules/a/node_modules/b/node_modules/c", "c"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.pkgPath, func(t *testing.T) {
			result := extractPackageName(tt.pkgPath)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr) && (s[:len(substr)] == substr || contains(s[1:], substr)))
}

func TestGetParserForFile(t *testing.T) {
	tests := []struct {
		name         string
		filePath     string
		expectedType string
		shouldError  bool
	}{
		{
			name:         "npm package-lock.json",
			filePath:     "package-lock.json",
			expectedType: "*npm.NpmParser",
			shouldError:  false,
		},
		{
			name:         "pnpm lock file",
			filePath:     "pnpm-lock.yaml",
			expectedType: "*npm.PnpmParser",
			shouldError:  false,
		},
		{
			name:         "unsupported file",
			filePath:     "pom.xml",
			expectedType: "",
			shouldError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, err := GetParserForFile(tt.filePath)

			if tt.shouldError {
				if err == nil {
					t.Errorf("Expected error for file %s, got nil", tt.filePath)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if parser == nil {
				t.Error("Expected parser, got nil")
			}
		})
	}
}

func TestDetectYarnVersion(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name         string
		content      string
		expectedType string
	}{
		{
			name: "Yarn 1 format",
			content: `# THIS IS AN AUTOGENERATED FILE. DO NOT EDIT THIS FILE DIRECTLY.
# yarn lockfile v1

lodash@^4.17.21:
  version "4.17.21"
  resolved "https://registry.npmjs.org/lodash/-/lodash-4.17.21.tgz"
  integrity sha512-xxx`,
			expectedType: "*npm.YarnParser",
		},
		{
			name: "Yarn 2+ format",
			content: `__metadata:
  version: 8
  cacheKey: 10c0

"lodash@npm:^4.17.21":
  version: 4.17.21
  resolution: "lodash@npm:4.17.21"
  checksum: 10c0/xxx
  languageName: node
  linkType: hard`,
			expectedType: "*npm.Yarn2Parser",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			lockFile := filepath.Join(tmpDir, "yarn.lock")

			if err := os.WriteFile(lockFile, []byte(tt.content), 0644); err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}

			parser, err := detectYarnVersion(lockFile)
			if err != nil {
				t.Fatalf("detectYarnVersion failed: %v", err)
			}

			if parser == nil {
				t.Fatal("Expected parser, got nil")
			}

			// Verify we can parse with the detected parser
			packages, err := parser.Parse(ctx, lockFile)
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}

			if len(packages) == 0 {
				t.Error("Expected to parse at least one package")
			}

			// Check that we found lodash
			foundLodash := false
			for _, pkg := range packages {
				if pkg.Name == "lodash" {
					foundLodash = true
					break
				}
			}
			if !foundLodash {
				t.Error("Expected to find lodash package")
			}
		})
	}
}

func TestGetParserForPackageManager(t *testing.T) {
	tests := []struct {
		name           string
		packageManager string
		expectedType   string
		shouldError    bool
	}{
		{
			name:           "npm",
			packageManager: "npm",
			expectedType:   "*npm.NpmParser",
			shouldError:    false,
		},
		{
			name:           "yarn - auto-detects version",
			packageManager: "yarn",
			expectedType:   "*npm.YarnParser or *npm.Yarn2Parser",
			shouldError:    false,
		},
		{
			name:           "pnpm",
			packageManager: "pnpm",
			expectedType:   "*npm.PnpmParser",
			shouldError:    false,
		},
		{
			name:           "unsupported",
			packageManager: "bun",
			expectedType:   "",
			shouldError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, err := GetParserForPackageManager(tt.packageManager)

			if tt.shouldError {
				if err == nil {
					t.Errorf("Expected error for package manager %s, got nil", tt.packageManager)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if parser == nil {
				t.Error("Expected parser, got nil")
			}
		})
	}
}

func TestGetParserForPackageManager_YarnAutoDetect(t *testing.T) {
	// Test that yarn auto-detects from disk
	ctx := context.Background()

	tests := []struct {
		name         string
		lockContent  string
		expectYarn1  bool
		expectYarn2  bool
	}{
		{
			name: "Yarn 1 lockfile",
			lockContent: `# THIS IS AN AUTOGENERATED FILE. DO NOT EDIT THIS FILE DIRECTLY.
# yarn lockfile v1

lodash@^4.17.21:
  version "4.17.21"`,
			expectYarn1: true,
			expectYarn2: false,
		},
		{
			name: "Yarn 2+ lockfile",
			lockContent: `__metadata:
  version: 8

"lodash@npm:^4.17.21":
  version: 4.17.21`,
			expectYarn1: false,
			expectYarn2: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			oldWd, _ := os.Getwd()
			defer os.Chdir(oldWd)

			// Change to temp directory
			os.Chdir(tmpDir)

			// Create yarn.lock
			lockFile := filepath.Join(tmpDir, "yarn.lock")
			if err := os.WriteFile(lockFile, []byte(tt.lockContent), 0644); err != nil {
				t.Fatalf("Failed to create lock file: %v", err)
			}

			// Get parser - should auto-detect
			parser, err := GetParserForPackageManager("yarn")
			if err != nil {
				t.Fatalf("GetParserForPackageManager failed: %v", err)
			}

			// Verify we can parse with the detected parser
			packages, err := parser.Parse(ctx, lockFile)
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}

			if len(packages) == 0 {
				t.Error("Expected to parse at least one package")
			}

			// Check parser type
			if tt.expectYarn1 {
				if _, ok := parser.(*YarnParser); !ok {
					t.Errorf("Expected YarnParser, got %T", parser)
				}
			}
			if tt.expectYarn2 {
				if _, ok := parser.(*Yarn2Parser); !ok {
					t.Errorf("Expected Yarn2Parser, got %T", parser)
				}
			}
		})
	}
}
