package composer

import (
	"context"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/pkg/rootio"
)

type MockAPIClient struct {
	AnalyzePackagesFunc func(ctx context.Context, packages []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error)
}

func (m *MockAPIClient) AnalyzePackages(ctx context.Context, packages []rootio.Package, ecosystem string) (*rootio.AnalyzePackagesResponse, error) {
	if m.AnalyzePackagesFunc != nil {
		return m.AnalyzePackagesFunc(ctx, packages, ecosystem)
	}
	return &rootio.AnalyzePackagesResponse{}, nil
}

type MockParser struct {
	ParseFunc  func(ctx context.Context, filePath string) ([]common.PackageInfo, error)
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
	return "{}", nil
}

func (m *MockParser) Validate(content string) bool { return true }

func (m *MockParser) Ecosystem() common.Ecosystem { return common.EcosystemComposer }
func (m *MockParser) FilePatterns() []string      { return []string{"composer.json"} }
func (m *MockParser) CanHandle(fileName string) bool {
	return fileName == "composer.json"
}

type MockCommandRunner struct {
	RunFunc func(ctx context.Context, dir string, env []string, name string, args ...string) error
	Calls   []MockCommandCall
}

type MockCommandCall struct {
	Dir  string
	Env  []string
	Name string
	Args []string
}

func (m *MockCommandRunner) Run(ctx context.Context, dir string, env []string, name string, args ...string) error {
	m.Calls = append(m.Calls, MockCommandCall{Dir: dir, Env: env, Name: name, Args: args})
	if m.RunFunc != nil {
		return m.RunFunc(ctx, dir, env, name, args...)
	}
	return nil
}
