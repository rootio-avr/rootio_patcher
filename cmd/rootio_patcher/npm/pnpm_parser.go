package npm

import (
	"context"
	"fmt"
	"strings"

	"rootio_patcher/cmd/rootio_patcher/common"
)

// PnpmParser handles parsing of pnpm-lock.yaml files
type PnpmParser struct{}

// NewPnpmParser creates a new pnpm parser
func NewPnpmParser() *PnpmParser {
	return &PnpmParser{}
}

// Ecosystem returns the ecosystem name
func (p *PnpmParser) Ecosystem() common.Ecosystem {
	return common.EcosystemNpm
}

// FilePatterns returns file patterns this parser handles
func (p *PnpmParser) FilePatterns() []string {
	return []string{"pnpm-lock.yaml"}
}

// CanHandle checks if this parser can handle the given file
func (p *PnpmParser) CanHandle(fileName string) bool {
	return fileName == "pnpm-lock.yaml" || strings.HasSuffix(fileName, "/pnpm-lock.yaml")
}

// Parse parses pnpm-lock.yaml files
func (p *PnpmParser) Parse(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
	return nil, fmt.Errorf("pnpm-lock.yaml parsing not yet implemented")
}

// Update updates package versions in pnpm-lock.yaml
func (p *PnpmParser) Update(ctx context.Context, filePath string, updates map[string]string) (string, error) {
	return "", fmt.Errorf("pnpm-lock.yaml update not supported - run 'pnpm install' to regenerate")
}

// Validate validates pnpm-lock.yaml format
func (p *PnpmParser) Validate(content string) bool {
	return strings.Contains(content, "lockfileVersion:")
}

// UpdatePackageJSON updates package.json with pnpm overrides (nested under "pnpm")
func (p *PnpmParser) UpdatePackageJSON(ctx context.Context, overrides map[string]string) error {
	patcher := NewPackageJSONPatcher()
	return patcher.Patch(ctx, PatchOptions{
		Updates:                  overrides,
		UpdateDirectDependencies: true,
		OverridesPath:            "pnpm.overrides",
	})
}
