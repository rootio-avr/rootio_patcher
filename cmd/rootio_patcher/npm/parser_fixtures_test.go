package npm

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// TestNpmParser_ParseRealLockFile tests parsing a real npm-generated package-lock.json
func TestNpmParser_ParseRealLockFile(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()

	lockFile := filepath.Join("testdata", "npm", "package-lock.json")

	packages, err := parser.Parse(ctx, lockFile)
	if err != nil {
		t.Fatalf("Failed to parse real lock file: %v", err)
	}

	// Verify we parsed packages
	if len(packages) == 0 {
		t.Fatal("Expected packages to be parsed, got 0")
	}

	t.Logf("Parsed %d packages from real npm lock file", len(packages))

	// Check for expected top-level dependencies
	foundLodash := false
	foundExpress := false
	foundJest := false

	for _, pkg := range packages {
		switch pkg.Name {
		case "lodash":
			foundLodash = true
			if pkg.Version != "4.17.21" {
				t.Errorf("Expected lodash version 4.17.21, got %s", pkg.Version)
			}
			if pkg.Dev {
				t.Error("Expected lodash to NOT be a dev dependency")
			}
		case "express":
			foundExpress = true
			if pkg.Version != "4.18.2" {
				t.Errorf("Expected express version 4.18.2, got %s", pkg.Version)
			}
			if pkg.Dev {
				t.Error("Expected express to NOT be a dev dependency")
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
	if !foundExpress {
		t.Error("Expected to find express in parsed packages")
	}
	if !foundJest {
		t.Error("Expected to find jest in parsed packages")
	}

	// Verify transitive dependencies are also parsed
	foundTransitive := false
	for _, pkg := range packages {
		// Check for some known transitive dependencies
		if pkg.Name == "ms" || pkg.Name == "debug" || pkg.Name == "accepts" {
			foundTransitive = true
			break
		}
	}

	if !foundTransitive {
		t.Error("Expected to find transitive dependencies in parsed packages")
	}
}

// TestNpmParser_ParseRealLockFile_Structure tests the structure of parsed packages
func TestNpmParser_ParseRealLockFile_Structure(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()

	lockFile := filepath.Join("testdata", "npm", "package-lock.json")

	packages, err := parser.Parse(ctx, lockFile)
	if err != nil {
		t.Fatalf("Failed to parse real lock file: %v", err)
	}

	// Verify all packages have required fields
	for _, pkg := range packages {
		if pkg.Name == "" {
			t.Error("Found package with empty name")
		}
		if pkg.Version == "" {
			t.Errorf("Package %s has empty version", pkg.Name)
		}

		// Version should be a valid semver-like string
		if len(pkg.Version) < 3 {
			t.Errorf("Package %s has suspiciously short version: %s", pkg.Name, pkg.Version)
		}
	}
}

// TestNpmParser_ParseRealLockFile_DevDependencies tests dev dependency detection
func TestNpmParser_ParseRealLockFile_DevDependencies(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()

	lockFile := filepath.Join("testdata", "npm", "package-lock.json")

	packages, err := parser.Parse(ctx, lockFile)
	if err != nil {
		t.Fatalf("Failed to parse real lock file: %v", err)
	}

	devCount := 0
	prodCount := 0

	for _, pkg := range packages {
		if pkg.Dev {
			devCount++
		} else {
			prodCount++
		}
	}

	t.Logf("Found %d dev dependencies and %d production dependencies", devCount, prodCount)

	// We should have both dev and prod dependencies
	if devCount == 0 {
		t.Error("Expected to find some dev dependencies")
	}
	if prodCount == 0 {
		t.Error("Expected to find some production dependencies")
	}

	// Jest and its dependencies should be dev
	// Express, lodash and their dependencies should be prod (mostly)
	if prodCount < 2 {
		t.Errorf("Expected at least 2 production dependencies (express, lodash), got %d", prodCount)
	}
}

// TestNpmParser_ParseRealLockFile_Count tests that we parse a reasonable number of packages
func TestNpmParser_ParseRealLockFile_Count(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()

	lockFile := filepath.Join("testdata", "npm", "package-lock.json")

	packages, err := parser.Parse(ctx, lockFile)
	if err != nil {
		t.Fatalf("Failed to parse real lock file: %v", err)
	}

	// With express, lodash, and jest + their transitive deps,
	// we should have a significant number of packages
	if len(packages) < 50 {
		t.Errorf("Expected at least 50 packages (including transitive deps), got %d", len(packages))
	}

	t.Logf("Total packages parsed: %d", len(packages))
}

// TestNpmParser_UpdateRealLockFile tests updating a real lock file
func TestNpmParser_UpdateRealLockFile(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()

	lockFile := filepath.Join("testdata", "npm", "package-lock.json")

	// Define updates
	updates := map[string]string{
		"lodash":  "4.17.22",
		"express": "4.19.0",
	}

	// Update the lock file content
	updatedContent, err := parser.Update(ctx, lockFile, updates)
	if err != nil {
		t.Fatalf("Failed to update lock file: %v", err)
	}

	// Verify the content is still valid JSON
	if !parser.Validate(updatedContent) {
		t.Error("Updated lock file content is not valid JSON")
	}

	// Verify updated versions are in the content
	if !strings.Contains(updatedContent, "4.17.22") {
		t.Error("Updated lodash version not found in updated content")
	}
	if !strings.Contains(updatedContent, "4.19.0") {
		t.Error("Updated express version not found in updated content")
	}
}

// TestYarnParser_ParseRealLockFile tests parsing a real yarn-generated yarn.lock
func TestYarnParser_ParseRealLockFile(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()

	lockFile := filepath.Join("testdata", "yarn", "yarn.lock")

	packages, err := parser.Parse(ctx, lockFile)
	if err != nil {
		t.Fatalf("Failed to parse real yarn.lock: %v", err)
	}

	// Verify we parsed packages
	if len(packages) == 0 {
		t.Fatal("Expected packages to be parsed, got 0")
	}

	t.Logf("Parsed %d packages from real yarn lock file", len(packages))

	// Check for expected top-level dependencies
	foundLodash := false
	foundExpress := false
	foundJest := false

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
		case "jest":
			foundJest = true
			if pkg.Version != "29.0.0" {
				t.Errorf("Expected jest version 29.0.0, got %s", pkg.Version)
			}
		}
	}

	if !foundLodash {
		t.Error("Expected to find lodash in parsed packages")
	}
	if !foundExpress {
		t.Error("Expected to find express in parsed packages")
	}
	if !foundJest {
		t.Error("Expected to find jest in parsed packages")
	}
}

// TestYarnParser_ParseRealLockFile_Count tests package count from yarn.lock
func TestYarnParser_ParseRealLockFile_Count(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()

	lockFile := filepath.Join("testdata", "yarn", "yarn.lock")

	packages, err := parser.Parse(ctx, lockFile)
	if err != nil {
		t.Fatalf("Failed to parse real yarn.lock: %v", err)
	}

	// Yarn should have similar number of packages as npm
	if len(packages) < 50 {
		t.Errorf("Expected at least 50 packages (including transitive deps), got %d", len(packages))
	}

	t.Logf("Total packages parsed from yarn.lock: %d", len(packages))
}

// TestPnpmParser_ParseRealLockFile tests parsing a real pnpm-generated pnpm-lock.yaml
func TestPnpmParser_ParseRealLockFile(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()

	lockFile := filepath.Join("testdata", "pnpm", "pnpm-lock.yaml")

	packages, err := parser.Parse(ctx, lockFile)
	if err != nil {
		t.Fatalf("Failed to parse real pnpm-lock.yaml: %v", err)
	}

	// Verify we parsed packages
	if len(packages) == 0 {
		t.Fatal("Expected packages to be parsed, got 0")
	}

	t.Logf("Parsed %d packages from real pnpm lock file", len(packages))

	// Check for expected top-level dependencies
	foundLodash := false
	foundExpress := false
	foundJest := false

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
		case "jest":
			foundJest = true
			if pkg.Version != "29.0.0" {
				t.Errorf("Expected jest version 29.0.0, got %s", pkg.Version)
			}
		}
	}

	if !foundLodash {
		t.Error("Expected to find lodash in parsed packages")
	}
	if !foundExpress {
		t.Error("Expected to find express in parsed packages")
	}
	if !foundJest {
		t.Error("Expected to find jest in parsed packages")
	}
}

// TestPnpmParser_ParseRealLockFile_Count tests package count from pnpm-lock.yaml
func TestPnpmParser_ParseRealLockFile_Count(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()

	lockFile := filepath.Join("testdata", "pnpm", "pnpm-lock.yaml")

	packages, err := parser.Parse(ctx, lockFile)
	if err != nil {
		t.Fatalf("Failed to parse real pnpm-lock.yaml: %v", err)
	}

	// pnpm should have similar number of packages as npm
	if len(packages) < 50 {
		t.Errorf("Expected at least 50 packages (including transitive deps), got %d", len(packages))
	}

	t.Logf("Total packages parsed from pnpm-lock.yaml: %d", len(packages))
}

// TestAllLockFiles_SameBasicPackages tests that all three lock files contain the same top-level packages
func TestAllLockFiles_SameBasicPackages(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()

	lockFiles := map[string]string{
		"npm":  filepath.Join("testdata", "npm", "package-lock.json"),
		"yarn": filepath.Join("testdata", "yarn", "yarn.lock"),
		"pnpm": filepath.Join("testdata", "pnpm", "pnpm-lock.yaml"),
	}

	results := make(map[string][]string)

	for pm, lockFile := range lockFiles {
		packages, err := parser.Parse(ctx, lockFile)
		if err != nil {
			t.Fatalf("Failed to parse %s lock file: %v", pm, err)
		}

		// Collect top-level package names
		topLevel := []string{}
		for _, pkg := range packages {
			if pkg.Name == "lodash" || pkg.Name == "express" || pkg.Name == "jest" {
				topLevel = append(topLevel, pkg.Name)
			}
		}

		results[pm] = topLevel
		t.Logf("%s: found top-level packages: %v", pm, topLevel)
	}

	// Verify all three have lodash, express, jest
	for pm, pkgs := range results {
		if len(pkgs) != 3 {
			t.Errorf("%s: expected 3 top-level packages (lodash, express, jest), got %d", pm, len(pkgs))
		}
	}
}
