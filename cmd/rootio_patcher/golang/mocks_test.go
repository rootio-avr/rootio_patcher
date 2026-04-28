package golang

import (
	"context"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/pkg/rootio"
)

// MockAPIClient is a mock implementation of common.APIClient for testing.
type MockAPIClient struct {
	AnalyzePackagesFunc func(ctx context.Context, packages []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error)
}

func (m *MockAPIClient) AnalyzePackages(ctx context.Context, packages []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error) {
	if m.AnalyzePackagesFunc != nil {
		return m.AnalyzePackagesFunc(ctx, packages, ecosystem)
	}
	return &rootio.AnalyzePackagesResponse{}, nil
}

// MockGoModParser is a mock implementation of goModParser for testing.
type MockGoModParser struct {
	ParseFunc func(ctx context.Context, filePath string) ([]common.PackageInfo, error)
	PatchFunc func(ctx context.Context, filePath string, updates []GoModUpdate) (string, error)
}

func (m *MockGoModParser) Parse(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
	if m.ParseFunc != nil {
		return m.ParseFunc(ctx, filePath)
	}
	return []common.PackageInfo{}, nil
}

func (m *MockGoModParser) Patch(ctx context.Context, filePath string, updates []GoModUpdate) (string, error) {
	if m.PatchFunc != nil {
		return m.PatchFunc(ctx, filePath, updates)
	}
	return "", nil
}

// CommandCall records a single invocation of MockCommandRunner.
type CommandCall struct {
	Dir  string
	Env  []string
	Name string
	Args []string
}

// MockCommandRunner is a mock implementation of CommandRunner that records calls.
type MockCommandRunner struct {
	RunFunc func(ctx context.Context, dir string, env []string, name string, args ...string) error
	Calls   []CommandCall
}

func (m *MockCommandRunner) Run(ctx context.Context, dir string, env []string, name string, args ...string) error {
	m.Calls = append(m.Calls, CommandCall{Dir: dir, Env: env, Name: name, Args: args})
	if m.RunFunc != nil {
		return m.RunFunc(ctx, dir, env, name, args...)
	}
	return nil
}
