package rpm

import "context"

// MockScanner is a test double for Scanner
type MockScanner struct {
	ListPackagesFunc func(ctx context.Context) ([]InstalledPackage, error)
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
