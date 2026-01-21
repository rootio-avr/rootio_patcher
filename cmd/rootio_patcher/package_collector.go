package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
)

// PackageCollector collects installed Python packages
type PackageCollector struct {
	pythonPath string
	logger     *slog.Logger
}

// InstalledPackage represents a package installed in the Python environment
type InstalledPackage struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Location string `json:"location,omitempty"`
}

// NewPackageCollector creates a new package collector
func NewPackageCollector(pythonPath string, logger *slog.Logger) *PackageCollector {
	return &PackageCollector{
		pythonPath: pythonPath,
		logger:     logger,
	}
}

// CollectPackages collects all installed packages using pip list
func (c *PackageCollector) CollectPackages(ctx context.Context) ([]InstalledPackage, error) {
	c.logger.DebugContext(ctx, "Using Python executable", slog.String("path", c.pythonPath))

	// Run: python -m pip list --format=json
	//nolint:gosec // Subprocess is safe - using fixed pip list arguments, pythonPath from config
	cmd := exec.CommandContext(ctx, c.pythonPath, "-m", "pip", "list", "--format=json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run pip list: %w", err)
	}

	// Parse JSON output
	var packages []InstalledPackage
	if err := json.Unmarshal(output, &packages); err != nil {
		return nil, fmt.Errorf("failed to parse pip list output: %w", err)
	}

	return packages, nil
}

// GetArch returns the system architecture for the API request
func GetArch() string {
	return runtime.GOARCH // returns "amd64" or "arm64"
}
