package apt

import (
	"context"

	"rootio_patcher/pkg/rootio"
)

// MockAPIClient is a test double for APIClient
type MockAPIClient struct {
	AnalyzeAptPackagesFunc func(ctx context.Context, ecosystem, distroVersion string, packages []rootio.Package) (*rootio.AptAnalyzeResponse, error)
}

func (m *MockAPIClient) AnalyzeAptPackages(ctx context.Context, ecosystem, distroVersion string, packages []rootio.Package) (*rootio.AptAnalyzeResponse, error) {
	if m.AnalyzeAptPackagesFunc != nil {
		return m.AnalyzeAptPackagesFunc(ctx, ecosystem, distroVersion, packages)
	}
	return &rootio.AptAnalyzeResponse{}, nil
}

// MockScanner is a test double for Scanner
type MockScanner struct {
	DetectOSFunc     func(ctx context.Context) (*OSInfo, error)
	ListPackagesFunc func(ctx context.Context) ([]InstalledPackage, error)
}

func (m *MockScanner) DetectOS(ctx context.Context) (*OSInfo, error) {
	if m.DetectOSFunc != nil {
		return m.DetectOSFunc(ctx)
	}
	return &OSInfo{Ecosystem: "debian", DistroVersion: "12", Codename: "bookworm"}, nil
}

func (m *MockScanner) ListPackages(ctx context.Context) ([]InstalledPackage, error) {
	if m.ListPackagesFunc != nil {
		return m.ListPackagesFunc(ctx)
	}
	return []InstalledPackage{}, nil
}

// CommandCall records a single runner invocation
type CommandCall struct {
	Name string
	Args []string
}

// MockRunner is a test double for CommandRunner that records calls
type MockRunner struct {
	RunFunc func(ctx context.Context, name string, args ...string) error
	Calls   []CommandCall
}

func (m *MockRunner) Run(ctx context.Context, name string, args ...string) error {
	m.Calls = append(m.Calls, CommandCall{Name: name, Args: args})
	if m.RunFunc != nil {
		return m.RunFunc(ctx, name, args...)
	}
	return nil
}

func (m *MockRunner) calledWith(name string, args ...string) bool {
	for _, c := range m.Calls {
		if c.Name != name {
			continue
		}
		if len(args) == 0 {
			return true
		}
		joined := append([]string{name}, c.Args...)
		match := true
		for i, a := range args {
			if i >= len(joined) || joined[i] != a {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
