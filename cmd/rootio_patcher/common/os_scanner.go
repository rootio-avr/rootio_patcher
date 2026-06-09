package common

import "context"

// SystemProbe executes OS-level queries and returns their raw output for parsing
type SystemProbe interface {
	ReadOSRelease(ctx context.Context) (string, error)
	QueryInstalledPackages(ctx context.Context) ([]byte, error)
}

// Scanner is the interface for reading OS info and installed packages
type Scanner[T any] interface {
	DetectOS(ctx context.Context) (*T, error)
	ListPackages(ctx context.Context) ([]InstalledPackage, error)
}

// OsScanner is a generic scanner that delegates OS detection and package listing
// to OS-specific probe and parse functions.
type OsScanner[T any] struct {
	Probe         SystemProbe
	ParseRelease  func(string) (*T, error)
	ParsePackages func([]byte) ([]InstalledPackage, error)
}

func (s *OsScanner[T]) DetectOS(ctx context.Context) (*T, error) {
	line, err := s.Probe.ReadOSRelease(ctx)
	if err != nil {
		return nil, err
	}
	return s.ParseRelease(line)
}

func (s *OsScanner[T]) ListPackages(ctx context.Context) ([]InstalledPackage, error) {
	out, err := s.Probe.QueryInstalledPackages(ctx)
	if err != nil {
		return nil, err
	}
	return s.ParsePackages(out)
}
