package common

import (
	"context"
)

// Ecosystem represents a package ecosystem (npm, pypi, maven, etc.)
type Ecosystem string

const (
	EcosystemPyPI  Ecosystem = "pypi"
	EcosystemNpm   Ecosystem = "npm"
	EcosystemMaven Ecosystem = "maven"
)

// PackageInfo represents a package with its metadata
type PackageInfo struct {
	Name              string    `json:"name"`
	Version           string    `json:"version"`
	VersionConstraint string    `json:"version_constraint,omitempty"`
	Ecosystem         Ecosystem `json:"ecosystem"`
	Direct            bool      `json:"direct"`
	Dev               bool      `json:"dev"`
	Location          string    `json:"location,omitempty"`
}

// Parser defines the interface for ecosystem-specific dependency parsers
type Parser interface {
	// Ecosystem returns the ecosystem name (npm, pypi, maven, etc.)
	Ecosystem() Ecosystem

	// FilePatterns returns file patterns this parser handles
	FilePatterns() []string

	// CanHandle checks if this parser can handle the given file
	CanHandle(fileName string) bool

	// Parse parses a dependency file and returns list of packages
	Parse(ctx context.Context, filePath string) ([]PackageInfo, error)

	// Update updates package versions and returns new file content
	Update(ctx context.Context, filePath string, updates map[string]string) (string, error)

	// Validate validates the updated content
	Validate(content string) bool
}
