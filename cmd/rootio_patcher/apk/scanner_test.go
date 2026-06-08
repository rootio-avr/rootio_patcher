package apk

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
	apkOut    []byte
	apkErr    error
}

func (m *mockProbe) ReadOSRelease(_ context.Context) (string, error) {
	return m.osRelease, m.osErr
}

func (m *mockProbe) QueryInstalledPackages(_ context.Context) ([]byte, error) {
	return m.apkOut, m.apkErr
}

// --- DetectOS ---

func TestScanner_DetectOS_Alpine318(t *testing.T) {
	s := NewScannerWithProbe(&mockProbe{osRelease: "3.18.4"})
	info, err := s.DetectOS(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "3.18", info.DistroVersion)
}

func TestScanner_DetectOS_Alpine319(t *testing.T) {
	s := NewScannerWithProbe(&mockProbe{osRelease: "3.19.0"})
	info, err := s.DetectOS(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "3.19", info.DistroVersion)
}

func TestScanner_DetectOS_MissingMinor(t *testing.T) {
	s := NewScannerWithProbe(&mockProbe{osRelease: "3"})
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
	raw := "curl-8.5.0-r0\nopenssl-3.1.4-r5\nca-certificates-20240226-r0\n"
	s := NewScannerWithProbe(&mockProbe{apkOut: []byte(raw)})
	pkgs, err := s.ListPackages(context.Background())
	require.NoError(t, err)
	require.Len(t, pkgs, 3)
	assert.Equal(t, InstalledPackage{Name: "curl", Version: "8.5.0-r0"}, pkgs[0])
	assert.Equal(t, InstalledPackage{Name: "openssl", Version: "3.1.4-r5"}, pkgs[1])
	assert.Equal(t, InstalledPackage{Name: "ca-certificates", Version: "20240226-r0"}, pkgs[2])
}

func TestScanner_ListPackages_HyphenatedName(t *testing.T) {
	raw := "ca-certificates-20240226-r0\nutil-linux-2.39-r1\n"
	s := NewScannerWithProbe(&mockProbe{apkOut: []byte(raw)})
	pkgs, err := s.ListPackages(context.Background())
	require.NoError(t, err)
	require.Len(t, pkgs, 2)
	assert.Equal(t, "ca-certificates", pkgs[0].Name)
	assert.Equal(t, "20240226-r0", pkgs[0].Version)
	assert.Equal(t, "util-linux", pkgs[1].Name)
	assert.Equal(t, "2.39-r1", pkgs[1].Version)
}

func TestScanner_ListPackages_SkipsEmptyLines(t *testing.T) {
	raw := "curl-8.5.0-r0\n\n\n"
	s := NewScannerWithProbe(&mockProbe{apkOut: []byte(raw)})
	pkgs, err := s.ListPackages(context.Background())
	require.NoError(t, err)
	require.Len(t, pkgs, 1)
}

func TestScanner_ListPackages_Empty(t *testing.T) {
	s := NewScannerWithProbe(&mockProbe{apkOut: []byte("")})
	pkgs, err := s.ListPackages(context.Background())
	require.NoError(t, err)
	assert.Empty(t, pkgs)
}

func TestScanner_ListPackages_ProbeError(t *testing.T) {
	s := NewScannerWithProbe(&mockProbe{apkErr: errors.New("apk not found")})
	_, err := s.ListPackages(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "apk not found")
}
