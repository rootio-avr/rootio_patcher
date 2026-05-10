package nuget

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rootio_patcher/cmd/rootio_patcher/common"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNuGetParser_Ecosystem(t *testing.T) {
	p := NewParser(testLogger())
	if p.Ecosystem() != common.EcosystemNuGet {
		t.Errorf("expected nuget, got %s", p.Ecosystem())
	}
}

func TestNuGetParser_CanHandle(t *testing.T) {
	p := NewParser(testLogger())
	tests := []struct {
		file string
		want bool
	}{
		{"MyApp.csproj", true},
		{"packages.config", true},
		{"pom.xml", false},
		{"package.json", false},
		{"go.mod", false},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			if got := p.CanHandle(tt.file); got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.file, got, tt.want)
			}
		})
	}
}

func TestNuGetParser_ParseCsproj(t *testing.T) {
	ctx := context.Background()
	p := NewParser(testLogger())

	fixture := filepath.Join("testdata", "csproj", "sample.csproj")
	packages, err := p.Parse(ctx, fixture)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// sample.csproj has 3 PackageReferences (not ProjectReference)
	if len(packages) != 3 {
		t.Fatalf("expected 3 packages, got %d: %+v", len(packages), packages)
	}

	byName := make(map[string]common.PackageInfo)
	for _, pkg := range packages {
		byName[pkg.Name] = pkg
	}

	if pkg, ok := byName["Newtonsoft.Json"]; !ok {
		t.Error("expected Newtonsoft.Json")
	} else if pkg.Version != "12.0.3" {
		t.Errorf("expected version 12.0.3, got %s", pkg.Version)
	}

	if pkg, ok := byName["Microsoft.Extensions.Logging"]; !ok {
		t.Error("expected Microsoft.Extensions.Logging")
	} else if pkg.Version != "6.0.0" {
		t.Errorf("expected version 6.0.0, got %s", pkg.Version)
	}

	// Ecosystem should be nuget for all
	for _, pkg := range packages {
		if pkg.Ecosystem != common.EcosystemNuGet {
			t.Errorf("expected ecosystem nuget, got %s", pkg.Ecosystem)
		}
	}
}

func TestNuGetParser_ParsePackagesConfig(t *testing.T) {
	ctx := context.Background()
	p := NewParser(testLogger())

	fixture := filepath.Join("testdata", "packages_config", "packages.config")
	packages, err := p.Parse(ctx, fixture)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// packages.config fixture has 3 packages
	if len(packages) != 3 {
		t.Fatalf("expected 3 packages, got %d: %+v", len(packages), packages)
	}

	byName := make(map[string]common.PackageInfo)
	for _, pkg := range packages {
		byName[pkg.Name] = pkg
	}

	if pkg, ok := byName["Newtonsoft.Json"]; !ok {
		t.Error("expected Newtonsoft.Json")
	} else if pkg.Version != "12.0.3" {
		t.Errorf("expected version 12.0.3, got %s", pkg.Version)
	}

	if pkg, ok := byName["log4net"]; !ok {
		t.Error("expected log4net")
	} else if pkg.Version != "2.0.12" {
		t.Errorf("expected version 2.0.12, got %s", pkg.Version)
	}
}

func TestNuGetParser_ParseDirectory(t *testing.T) {
	ctx := context.Background()
	p := NewParser(testLogger())

	// Parse the csproj testdata directory - should discover sample.csproj
	packages, err := p.Parse(ctx, filepath.Join("testdata", "csproj"))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(packages) == 0 {
		t.Fatal("expected packages from directory discovery, got none")
	}
}

func TestNuGetParser_ParseCsproj_SkipsProjectReference(t *testing.T) {
	ctx := context.Background()
	p := NewParser(testLogger())

	tmpDir := t.TempDir()
	csproj := filepath.Join(tmpDir, "MyApp.csproj")
	content := `<Project Sdk="Microsoft.NET.Sdk">
  <ItemGroup>
    <PackageReference Include="Newtonsoft.Json" Version="12.0.3" />
    <ProjectReference Include="../Other/Other.csproj" />
  </ItemGroup>
</Project>`
	if err := os.WriteFile(csproj, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	packages, err := p.Parse(ctx, csproj)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(packages) != 1 {
		t.Fatalf("expected 1 package (no ProjectReference), got %d", len(packages))
	}
	if packages[0].Name != "Newtonsoft.Json" {
		t.Errorf("expected Newtonsoft.Json, got %s", packages[0].Name)
	}
}

func TestNuGetParser_UpdateCsproj(t *testing.T) {
	ctx := context.Background()
	p := NewParser(testLogger())

	tmpDir := t.TempDir()
	csproj := filepath.Join(tmpDir, "MyApp.csproj")
	content := `<Project Sdk="Microsoft.NET.Sdk">
  <ItemGroup>
    <PackageReference Include="Newtonsoft.Json" Version="12.0.3" />
    <PackageReference Include="Serilog" Version="2.10.0" />
  </ItemGroup>
</Project>`
	if err := os.WriteFile(csproj, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Format: "aliasName:aliasVersion" — both name and version are rewritten
	updates := map[string]string{
		"Newtonsoft.Json": "Rootio.Newtonsoft.Json:13.0.1",
	}

	result, err := p.Update(ctx, csproj, updates)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if !strings.Contains(result, `Include="Rootio.Newtonsoft.Json"`) {
		t.Error("expected alias name Rootio.Newtonsoft.Json in result")
	}
	if !strings.Contains(result, `Version="13.0.1"`) {
		t.Error("expected updated version 13.0.1 in result")
	}
	if strings.Contains(result, `Include="Newtonsoft.Json"`) {
		t.Error("old package name Newtonsoft.Json should be replaced")
	}
	if strings.Contains(result, `Version="12.0.3"`) {
		t.Error("old version 12.0.3 should be replaced")
	}
	// Serilog should be unchanged
	if !strings.Contains(result, `Version="2.10.0"`) {
		t.Error("Serilog version should be unchanged")
	}
}

func TestNuGetParser_UpdateCsproj_ChildElement(t *testing.T) {
	ctx := context.Background()
	p := NewParser(testLogger())

	tmpDir := t.TempDir()
	csproj := filepath.Join(tmpDir, "MyApp.csproj")
	content := `<Project Sdk="Microsoft.NET.Sdk">
  <ItemGroup>
    <PackageReference Include="Newtonsoft.Json">
      <Version>12.0.3</Version>
    </PackageReference>
  </ItemGroup>
</Project>`
	if err := os.WriteFile(csproj, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	updates := map[string]string{
		"Newtonsoft.Json": "Rootio.Newtonsoft.Json:13.0.1",
	}

	result, err := p.Update(ctx, csproj, updates)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if !strings.Contains(result, `Include="Rootio.Newtonsoft.Json"`) {
		t.Error("expected alias name in result")
	}
	if !strings.Contains(result, `<Version>13.0.1</Version>`) {
		t.Error("expected updated version in child element")
	}
}

func TestNuGetParser_UpdatePackagesConfig(t *testing.T) {
	ctx := context.Background()
	p := NewParser(testLogger())

	tmpDir := t.TempDir()
	cfg := filepath.Join(tmpDir, "packages.config")
	content := `<?xml version="1.0" encoding="utf-8"?>
<packages>
  <package id="Newtonsoft.Json" version="12.0.3" targetFramework="net48" />
  <package id="log4net" version="2.0.12" targetFramework="net48" />
</packages>`
	if err := os.WriteFile(cfg, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Format: "aliasName:aliasVersion" — both id and version are rewritten
	updates := map[string]string{
		"Newtonsoft.Json": "Rootio.Newtonsoft.Json:13.0.1",
	}

	result, err := p.Update(ctx, cfg, updates)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if !strings.Contains(result, `id="Rootio.Newtonsoft.Json"`) {
		t.Error("expected alias name Rootio.Newtonsoft.Json in result")
	}
	if !strings.Contains(result, `version="13.0.1"`) {
		t.Error("expected updated version 13.0.1 in result")
	}
	if strings.Contains(result, `id="Newtonsoft.Json"`) {
		t.Error("old package id should be replaced")
	}
	if strings.Contains(result, `version="12.0.3"`) {
		t.Error("old version 12.0.3 should be replaced")
	}
	// log4net should be unchanged
	if !strings.Contains(result, `version="2.0.12"`) {
		t.Error("log4net version should be unchanged")
	}
}

func TestNuGetParser_Validate(t *testing.T) {
	p := NewParser(testLogger())

	valid := `<Project><ItemGroup><PackageReference Include="Foo" Version="1.0" /></ItemGroup></Project>`
	if !p.Validate(valid) {
		t.Error("expected valid XML to return true")
	}

	invalid := `<Project><ItemGroup><PackageReference Include="Foo" Version="1.0"`
	if p.Validate(invalid) {
		t.Error("expected invalid XML to return false")
	}
}

func TestNuGetParser_ParseFileNotFound(t *testing.T) {
	ctx := context.Background()
	p := NewParser(testLogger())

	_, err := p.Parse(ctx, "/nonexistent/file.csproj")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}
