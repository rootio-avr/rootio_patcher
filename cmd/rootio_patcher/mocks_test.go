package main

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

// MockPipExecutor is a mock implementation of PipExecutorInterface for testing
type MockPipExecutor struct {
	ApplyPatchFunc       func(ctx context.Context, patch rootio.PackagePatch) error
	ApplyPatchForPipFunc func(ctx context.Context, patch rootio.PackagePatch) error
}

func (m *MockPipExecutor) ApplyPatch(ctx context.Context, patch rootio.PackagePatch) error {
	if m.ApplyPatchFunc != nil {
		return m.ApplyPatchFunc(ctx, patch)
	}
	return nil
}

func (m *MockPipExecutor) ApplyPatchForPip(ctx context.Context, patch rootio.PackagePatch) error {
	if m.ApplyPatchForPipFunc != nil {
		return m.ApplyPatchForPipFunc(ctx, patch)
	}
	return nil
}

// MockPackageCollector is a mock implementation of PackageCollectorInterface for testing
type MockPackageCollector struct {
	CollectPackagesFunc func(ctx context.Context) ([]common.InstalledPackage, error)
}

func (m *MockPackageCollector) CollectPackages(ctx context.Context) ([]common.InstalledPackage, error) {
	if m.CollectPackagesFunc != nil {
		return m.CollectPackagesFunc(ctx)
	}
	return []common.InstalledPackage{}, nil
}
