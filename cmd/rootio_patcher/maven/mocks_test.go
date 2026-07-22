package maven

import (
	"context"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/pkg/rootio"
)

// MockAPIClient is a mock implementation of APIClient for testing
type MockAPIClient struct {
	AnalyzePackagesFunc func(ctx context.Context, packages []rootio.Package, ignore []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error)
}

func (m *MockAPIClient) AnalyzePackages(ctx context.Context, packages []rootio.Package, ignore []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error) {
	if m.AnalyzePackagesFunc != nil {
		return m.AnalyzePackagesFunc(ctx, packages, ignore, ecosystem)
	}
	return &rootio.AnalyzePackagesResponse{}, nil
}

// MockParser is a mock implementation of Parser for testing
type MockParser struct {
	ParseFunc func(ctx context.Context, filePath string) ([]common.PackageInfo, error)
	UpdateFunc func(ctx context.Context, filePath string, updates map[string]string) (string, error)
}

func (m *MockParser) Parse(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
	if m.ParseFunc != nil {
		return m.ParseFunc(ctx, filePath)
	}
	return []common.PackageInfo{}, nil
}

func (m *MockParser) Update(ctx context.Context, filePath string, updates map[string]string) (string, error) {
	if m.UpdateFunc != nil {
		return m.UpdateFunc(ctx, filePath, updates)
	}
	return "", nil
}

func (m *MockParser) Validate(content string) bool {
	return true
}

func (m *MockParser) Ecosystem() common.Ecosystem {
	return common.EcosystemMaven
}

func (m *MockParser) FilePatterns() []string {
	return []string{"pom.xml"}
}

func (m *MockParser) CanHandle(fileName string) bool {
	return fileName == "pom.xml"
}

func (m *MockParser) UpdatePackageJSON(ctx context.Context, overrides map[string]string) error {
	// Mock implementation - does nothing (Maven doesn't use package.json)
	return nil
}

// MockMavenParser is a mock that uses real MavenParser for Update but allows mocking Parse
type MockMavenParser struct {
	*MavenParser
	ParseFunc func(ctx context.Context, filePath string) ([]common.PackageInfo, error)
}

func (m *MockMavenParser) Parse(ctx context.Context, filePath string) ([]common.PackageInfo, error) {
	if m.ParseFunc != nil {
		return m.ParseFunc(ctx, filePath)
	}
	return []common.PackageInfo{}, nil
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
