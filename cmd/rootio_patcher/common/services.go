package common

import (
	"context"
	"errors"

	"rootio_patcher/pkg/rootio"
)

// ErrPatchesAvailable is returned in dry-run mode when patches are available.
// Callers should map this to exit code 2.
var ErrPatchesAvailable = errors.New("patches available")

// InstalledPackage represents a package installed in an environment
type InstalledPackage struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Location string `json:"location,omitempty"`
}

// APIClient defines the interface for calling the Root.io API
type APIClient interface {
	AnalyzePackages(ctx context.Context, packages []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error)
}

// PipExecutorInterface defines the interface for executing pip commands
type PipExecutorInterface interface {
	ApplyPatch(ctx context.Context, patch rootio.PackagePatch) error
	ApplyPatchForPip(ctx context.Context, patch rootio.PackagePatch) error
}

// PackageCollectorInterface defines the interface for collecting packages
type PackageCollectorInterface interface {
	CollectPackages(ctx context.Context) ([]InstalledPackage, error)
}
