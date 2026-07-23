package npm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestYarn2Parser_Ecosystem(t *testing.T) {
	parser := NewYarn2Parser()
	if parser.Ecosystem() != "npm" {
		t.Errorf("Expected ecosystem 'npm', got '%s'", parser.Ecosystem())
	}
}

func TestYarn2Parser_FilePatterns(t *testing.T) {
	parser := NewYarn2Parser()
	patterns := parser.FilePatterns()
	if len(patterns) != 1 || patterns[0] != "yarn.lock" {
		t.Errorf("Expected patterns [yarn.lock], got %v", patterns)
	}
}

func TestYarn2Parser_CanHandle(t *testing.T) {
	parser := NewYarn2Parser()

	tests := []struct {
		name     string
		fileName string
		expected bool
	}{
		{"yarn.lock", "yarn.lock", true},
		{"package-lock.json", "package-lock.json", false},
		{"pnpm-lock.yaml", "pnpm-lock.yaml", false},
		{"pom.xml", "pom.xml", false},
		{"path/yarn.lock", "some/path/yarn.lock", true},
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

func TestYarn2Parser_Parse_Basic(t *testing.T) {
	ctx := context.Background()
	parser := NewYarn2Parser()

	// Create temp directory
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "yarn.lock")

	// Create basic Yarn 2+ lock file
	content := `__metadata:
  version: 8
  cacheKey: 10c0

"lodash@npm:^4.17.21":
  version: 4.17.21
  resolution: "lodash@npm:4.17.21"
  checksum: 10c0/abcd1234
  languageName: node
  linkType: hard

"express@npm:^4.18.0":
  version: 4.18.2
  resolution: "express@npm:4.18.2"
  checksum: 10c0/efgh5678
  languageName: node
  linkType: hard
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
	foundExpress := false

	for _, pkg := range packages {
		switch pkg.Name {
		case "lodash":
			foundLodash = true
			if pkg.Version != "4.17.21" {
				t.Errorf("Expected lodash version 4.17.21, got %s", pkg.Version)
			}
		case "express":
			foundExpress = true
			if pkg.Version != "4.18.2" {
				t.Errorf("Expected express version 4.18.2, got %s", pkg.Version)
			}
		}
	}

	if !foundLodash {
		t.Error("Expected to find lodash in parsed packages")
	}
	if !foundExpress {
		t.Error("Expected to find express in parsed packages")
	}
}

func TestYarn2Parser_Parse_ScopedPackages(t *testing.T) {
	ctx := context.Background()
	parser := NewYarn2Parser()

	// Create temp directory
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "yarn.lock")

	// Create Yarn 2+ lock file with scoped packages
	content := `__metadata:
  version: 8
  cacheKey: 10c0

"@babel/runtime@npm:^7.25.0":
  version: 7.25.0
  resolution: "@babel/runtime@npm:7.25.0"
  dependencies:
    regenerator-runtime: "npm:^0.14.0"
  checksum: 10c0/abcd1234
  languageName: node
  linkType: hard

"@types/node@npm:^18.0.0":
  version: 18.11.0
  resolution: "@types/node@npm:18.11.0"
  checksum: 10c0/efgh5678
  languageName: node
  linkType: hard
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

func TestYarn2Parser_Parse_MultipleSpecs(t *testing.T) {
	ctx := context.Background()
	parser := NewYarn2Parser()

	// Create temp directory
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "yarn.lock")

	// Create Yarn 2+ lock file with multiple specs for same package
	content := `__metadata:
  version: 8
  cacheKey: 10c0

"lodash@npm:^4.17.0, lodash@npm:^4.17.21":
  version: 4.17.21
  resolution: "lodash@npm:4.17.21"
  checksum: 10c0/abcd1234
  languageName: node
  linkType: hard
`
	if err := os.WriteFile(lockFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create lock file: %v", err)
	}

	packages, err := parser.Parse(ctx, lockFile)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Should only get one package even though there are multiple specs
	if len(packages) != 1 {
		t.Fatalf("Expected 1 package, got %d", len(packages))
	}

	pkg := packages[0]
	if pkg.Name != "lodash" {
		t.Errorf("Expected package name 'lodash', got %s", pkg.Name)
	}
	if pkg.Version != "4.17.21" {
		t.Errorf("Expected version 4.17.21, got %s", pkg.Version)
	}
}

func TestYarn2Parser_Parse_InvalidYAML(t *testing.T) {
	ctx := context.Background()
	parser := NewYarn2Parser()

	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "yarn.lock")

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

func TestYarn2Parser_Parse_MissingMetadata(t *testing.T) {
	ctx := context.Background()
	parser := NewYarn2Parser()

	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "yarn.lock")

	// Missing __metadata
	content := `"lodash@npm:^4.17.21":
  version: 4.17.21
  resolution: "lodash@npm:4.17.21"
`
	if err := os.WriteFile(lockFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create lock file: %v", err)
	}

	_, err := parser.Parse(ctx, lockFile)
	if err == nil {
		t.Error("Expected error for missing __metadata, got nil")
	}
}

func TestYarn2Parser_Parse_FileNotFound(t *testing.T) {
	ctx := context.Background()
	parser := NewYarn2Parser()

	_, err := parser.Parse(ctx, "/nonexistent/yarn.lock")
	if err == nil {
		t.Error("Expected error for nonexistent file, got nil")
	}
}

func TestParseYarn2PackageName(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		expectedName string
	}{
		{
			name:         "simple package",
			key:          "lodash@npm:4.17.21",
			expectedName: "lodash",
		},
		{
			name:         "scoped package",
			key:          "@babel/runtime@npm:7.25.0",
			expectedName: "@babel/runtime",
		},
		{
			name:         "multiple specs",
			key:          "lodash@npm:^4.17.0, lodash@npm:^4.17.21",
			expectedName: "lodash",
		},
		{
			name:         "scoped with multiple specs",
			key:          "@babel/runtime@npm:^7.0.0, @babel/runtime@npm:^7.25.0",
			expectedName: "@babel/runtime",
		},
		{
			name:         "workspace protocol",
			key:          "my-package@workspace:*",
			expectedName: "my-package",
		},
		{
			name:         "invalid: no @ separator",
			key:          "package",
			expectedName: "",
		},
		{
			name:         "invalid: only scope",
			key:          "@scope",
			expectedName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := parseYarn2PackageName(tt.key)
			if name != tt.expectedName {
				t.Errorf("parseYarn2PackageName(%s) = %s, want %s", tt.key, name, tt.expectedName)
			}
		})
	}
}

func TestYarn2Parser_Validate(t *testing.T) {
	parser := NewYarn2Parser()

	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name: "valid yarn 2+ lock file",
			content: `__metadata:
  version: 8
  cacheKey: 10c0

"lodash@npm:4.17.21":
  version: 4.17.21
  resolution: "lodash@npm:4.17.21"`,
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
			name: "no __metadata",
			content: `"lodash@npm:4.17.21":
  version: 4.17.21`,
			expected: false,
		},
		{
			name: "metadata without version",
			content: `__metadata:
  cacheKey: 10c0

"lodash@npm:4.17.21":
  version: 4.17.21`,
			expected: false,
		},
		{
			name: "yarn 1 format",
			content: `# yarn lockfile v1

lodash@^4.17.21:
  version "4.17.21"`,
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

func TestYarn2Parser_Parse_RealLockFile(t *testing.T) {
	ctx := context.Background()
	parser := NewYarn2Parser()

	// Test with the actual yarn.lock from test_cases/7 (Yarn 2+)
	lockFile := "../../../test_cases/7/yarn.lock"

	// Check if file exists (skip test if not)
	if _, err := os.Stat(lockFile); os.IsNotExist(err) {
		t.Skip("Real yarn.lock file not found")
	}

	packages, err := parser.Parse(ctx, lockFile)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Should have many packages
	if len(packages) == 0 {
		t.Error("Expected to find packages in real lockfile")
	}

	// Check for some expected packages
	foundLodash := false
	foundMocha := false

	for _, pkg := range packages {
		if pkg.Name == "lodash" {
			foundLodash = true
		}
		if pkg.Name == "mocha" {
			foundMocha = true
		}
	}

	if !foundLodash {
		t.Error("Expected to find lodash in real lockfile")
	}
	if !foundMocha {
		t.Error("Expected to find mocha in real lockfile")
	}
}

func TestYarn2Parser_Parse_CacheKeyV9(t *testing.T) {
	ctx := context.Background()
	parser := NewYarn2Parser()

	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "yarn.lock")

	content := `__metadata:
  version: 9
  cacheKey: 10c0

"lodash@npm:4.17.21":
  version: 4.17.21
  resolution: "lodash@npm:4.17.21"
  checksum: 10c0/abcd1234
  languageName: node
  linkType: hard
`
	if err := os.WriteFile(lockFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create lock file: %v", err)
	}

	packages, err := parser.Parse(ctx, lockFile)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(packages) != 1 || packages[0].Name != "lodash" || packages[0].Version != "4.17.21" {
		t.Errorf("Expected single lodash@4.17.21 entry, got %+v", packages)
	}
}

func TestYarn2Parser_Parse_MetadataVersion(t *testing.T) {
	ctx := context.Background()
	parser := NewYarn2Parser()

	tests := []struct {
		name      string
		version   string
		wantError bool
	}{
		{name: "v2 accepted", version: "2", wantError: false},
		{name: "v8 accepted", version: "8", wantError: false},
		{name: "v9 accepted", version: "9", wantError: false},
		{name: "v10 accepted (regression: two-digit)", version: "10", wantError: false},
		{name: "v99 accepted", version: "99", wantError: false},
		{name: "v1 rejected (yarn classic)", version: "1", wantError: true},
		{name: "v0 rejected", version: "0", wantError: true},
		{name: "non-numeric rejected", version: "berry", wantError: true},
	}

	body := `
"lodash@npm:4.17.21":
  version: 4.17.21
  resolution: "lodash@npm:4.17.21"
  checksum: 10c0/abcd1234
  languageName: node
  linkType: hard
`

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lockFile := filepath.Join(t.TempDir(), "yarn.lock")
			content := fmt.Sprintf("__metadata:\n  version: %s\n  cacheKey: 10c0\n%s", tt.version, body)
			require.NoError(t, os.WriteFile(lockFile, []byte(content), 0644))

			_, err := parser.Parse(ctx, lockFile)
			if tt.wantError {
				require.Error(t, err, "version %q should be rejected", tt.version)
			} else {
				require.NoError(t, err, "version %q should be accepted", tt.version)
			}
		})
	}
}

// TestYarn2Parser_FindParents_AliasedParent is the regression test for the
// invalid/inert-resolutions-key bug: once a parent (express) has already been
// aliased to npm:@rootio/... in an earlier remediation round, its lock file
// spec key embeds the alias descriptor (e.g.
// "express@npm:@rootio/express@4.18.2-root.io.3") while its "resolution:"
// field carries the true resolved identity, alias-name-first (e.g.
// "@rootio/express@npm:4.18.2-root.io.3" — this is the real shape Yarn Berry
// writes, verified against a live `yarn install`).
//
// FindParents must return the parent's RESOLVED identity ("@rootio/express"),
// not its pre-alias spec-key name ("express") and not the raw spec-key
// descriptor. Two distinct failures if it doesn't:
//   - Using the spec-key descriptor verbatim produces unparseable syntax
//     ("express@npm:@rootio/express/body-parser", Yarn error YN0057).
//   - Using the pre-alias bare name ("express/body-parser") is syntactically
//     valid but Yarn Berry silently fails to match it against the tree (it
//     matches path-scoped resolutions against the resolved identity), so the
//     override is silently never applied — confirmed via a live repro
//     against the real registry.
func TestYarn2Parser_FindParents_AliasedParent(t *testing.T) {
	ctx := context.Background()
	parser := NewYarn2Parser()
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "yarn.lock")

	content := `__metadata:
  version: 8
  cacheKey: 10c0

"express@npm:@rootio/express@4.18.2-root.io.3":
  version: 4.18.2-root.io.3
  resolution: "@rootio/express@npm:4.18.2-root.io.3"
  dependencies:
    body-parser: "npm:@rootio/body-parser@1.20.1-root.io.1"
  checksum: 10c0/abcd1234
  languageName: node
  linkType: hard

"body-parser@npm:@rootio/body-parser@1.20.1-root.io.1":
  version: 1.20.1-root.io.1
  resolution: "@rootio/body-parser@npm:1.20.1-root.io.1"
  checksum: 10c0/deadbeef
  languageName: node
  linkType: hard
`
	if err := os.WriteFile(lockFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create lock file: %v", err)
	}

	got, err := parser.FindParents(ctx, lockFile, "body-parser", "1.20.1-root.io.1")
	if err != nil {
		t.Fatalf("FindParents failed: %v", err)
	}
	if len(got) != 1 || got[0] != "@rootio/express" {
		t.Errorf("expected parent [@rootio/express] (resolved identity), got %v", got)
	}
}
