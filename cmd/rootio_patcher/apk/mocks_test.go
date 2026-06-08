package apk

import (
	"context"
	"os"

	"rootio_patcher/pkg/rootio"
)

// MockAPIClient is a test double for APIClient
type MockAPIClient struct {
	AnalyzeOsPackagesFunc func(ctx context.Context, endpoint, ecosystem, distroVersion string, packages []rootio.Package) (*rootio.OsAnalyzeResponse, error)
}

func (m *MockAPIClient) AnalyzeOsPackages(ctx context.Context, endpoint, ecosystem, distroVersion string, packages []rootio.Package) (*rootio.OsAnalyzeResponse, error) {
	if m.AnalyzeOsPackagesFunc != nil {
		return m.AnalyzeOsPackagesFunc(ctx, endpoint, ecosystem, distroVersion, packages)
	}
	return &rootio.OsAnalyzeResponse{}, nil
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
	return &OSInfo{DistroVersion: "3.18"}, nil
}

func (m *MockScanner) ListPackages(ctx context.Context) ([]InstalledPackage, error) {
	if m.ListPackagesFunc != nil {
		return m.ListPackagesFunc(ctx)
	}
	return []InstalledPackage{}, nil
}

// mockFS is a no-op fileSystem for tests — file operations succeed silently
type mockFS struct{}

func (mockFS) MkdirAll(_ string, _ os.FileMode) error            { return nil }
func (mockFS) WriteFile(_ string, _ []byte, _ os.FileMode) error { return nil }
func (mockFS) ReadFile(_ string) ([]byte, error) {
	return []byte("https://dl-cdn.alpinelinux.org/alpine/v3.19/main\n"), nil
}
func (mockFS) OpenFile(_ string, _ int, _ os.FileMode) (*os.File, error) {
	return os.CreateTemp("", "apk-test-repo")
}
func (mockFS) Remove(_ string) error { return nil }

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
