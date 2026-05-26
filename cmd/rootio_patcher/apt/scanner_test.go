package apt

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockProbe is a test double for SystemProbe
type mockProbe struct {
	osRelease string
	osErr     error
	dpkgOut   []byte
	dpkgErr   error
}

func (m *mockProbe) ReadOSRelease(_ context.Context) (string, error) {
	return m.osRelease, m.osErr
}

func (m *mockProbe) QueryInstalledPackages(_ context.Context) ([]byte, error) {
	return m.dpkgOut, m.dpkgErr
}

// --- DetectOS ---

func TestScanner_DetectOS_Debian12(t *testing.T) {
	s := NewScannerWithProbe(&mockProbe{osRelease: "debian 12 bookworm"})
	info, err := s.DetectOS(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "debian", info.Ecosystem)
	assert.Equal(t, "12", info.DistroVersion)
	assert.Equal(t, "bookworm", info.Codename)
}

func TestScanner_DetectOS_Ubuntu2204(t *testing.T) {
	s := NewScannerWithProbe(&mockProbe{osRelease: "ubuntu 22.04 jammy"})
	info, err := s.DetectOS(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "ubuntu", info.Ecosystem)
	assert.Equal(t, "22.04", info.DistroVersion)
	assert.Equal(t, "jammy", info.Codename)
}

func TestScanner_DetectOS_UnsupportedOS(t *testing.T) {
	s := NewScannerWithProbe(&mockProbe{osRelease: "alpine 3.18 edge"})
	_, err := s.DetectOS(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported OS")
}

func TestScanner_DetectOS_MissingVersionID(t *testing.T) {
	s := NewScannerWithProbe(&mockProbe{osRelease: "debian"})
	_, err := s.DetectOS(context.Background())
	require.Error(t, err)
}

func TestScanner_DetectOS_ProbeError(t *testing.T) {
	s := NewScannerWithProbe(&mockProbe{osErr: errors.New("no such file")})
	_, err := s.DetectOS(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no such file")
}

// --- ListPackages ---

func TestScanner_ListPackages_Normal(t *testing.T) {
	raw := "curl\t7.88.1-10+deb12u5\nopenssl\t3.0.11-1\nca-certificates\t20230311\n"
	s := NewScannerWithProbe(&mockProbe{dpkgOut: []byte(raw)})
	pkgs, err := s.ListPackages(context.Background())
	require.NoError(t, err)
	require.Len(t, pkgs, 3)
	assert.Equal(t, InstalledPackage{Name: "curl", Version: "7.88.1-10+deb12u5"}, pkgs[0])
	assert.Equal(t, InstalledPackage{Name: "openssl", Version: "3.0.11-1"}, pkgs[1])
}

func TestScanner_ListPackages_SkipsEmptyAndMalformedLines(t *testing.T) {
	raw := "no-tab-here\ncurl\t7.88.1\n\n\t\n"
	s := NewScannerWithProbe(&mockProbe{dpkgOut: []byte(raw)})
	pkgs, err := s.ListPackages(context.Background())
	require.NoError(t, err)
	require.Len(t, pkgs, 1)
	assert.Equal(t, "curl", pkgs[0].Name)
}

func TestScanner_ListPackages_Empty(t *testing.T) {
	s := NewScannerWithProbe(&mockProbe{dpkgOut: []byte("")})
	pkgs, err := s.ListPackages(context.Background())
	require.NoError(t, err)
	assert.Empty(t, pkgs)
}

func TestScanner_ListPackages_ProbeError(t *testing.T) {
	s := NewScannerWithProbe(&mockProbe{dpkgErr: errors.New("dpkg not found")})
	_, err := s.ListPackages(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dpkg not found")
}
