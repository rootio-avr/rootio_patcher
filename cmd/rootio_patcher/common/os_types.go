package common

import (
	"context"

	"rootio_patcher/pkg/rootio"
)

// OsAPIClient is the interface for calling the Root.io OS package analysis endpoint
type OsAPIClient interface {
	AnalyzeOsPackages(ctx context.Context, endpoint, ecosystem, distroVersion string, packages []rootio.Package) (*rootio.OsAnalyzeResponse, error)
}
