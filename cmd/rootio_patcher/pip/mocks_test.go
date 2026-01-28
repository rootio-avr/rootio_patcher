package pip

import (
	"context"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/pkg/rootio"
)

// MockAPIClient is a mock implementation of APIClient for testing
type MockAPIClient struct {
	AnalyzePackagesFunc func(ctx context.Context, packages []rootio.Package) (*rootio.AnalyzePackagesResponse, error)
}

func (m *MockAPIClient) AnalyzePackages(ctx context.Context, packages []rootio.Package) (*rootio.AnalyzePackagesResponse, error) {
	if m.AnalyzePackagesFunc != nil {
		return m.AnalyzePackagesFunc(ctx, packages)
	}
	return &rootio.AnalyzePackagesResponse{}, nil
}

// MockPipService is a mock implementation of Service for testing
type MockPipService struct {
	ListPackagesFunc     func(ctx context.Context) ([]common.InstalledPackage, error)
	ApplyPatchFunc       func(ctx context.Context, patch rootio.PackagePatch) error
	ApplyPatchForPipFunc func(ctx context.Context, patch rootio.PackagePatch) error
}

func (m *MockPipService) ListPackages(ctx context.Context) ([]common.InstalledPackage, error) {
	if m.ListPackagesFunc != nil {
		return m.ListPackagesFunc(ctx)
	}
	return []common.InstalledPackage{}, nil
}

func (m *MockPipService) ApplyPatch(ctx context.Context, patch rootio.PackagePatch) error {
	if m.ApplyPatchFunc != nil {
		return m.ApplyPatchFunc(ctx, patch)
	}
	return nil
}

func (m *MockPipService) ApplyPatchForPip(ctx context.Context, patch rootio.PackagePatch) error {
	if m.ApplyPatchForPipFunc != nil {
		return m.ApplyPatchForPipFunc(ctx, patch)
	}
	return nil
}
