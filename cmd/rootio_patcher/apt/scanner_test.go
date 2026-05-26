package apt

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parsePackages mirrors the scanner's dpkg-query parsing logic so we can unit-test it
// without invoking the real binary.
func parsePackages(output string) ([]InstalledPackage, error) {
	var packages []InstalledPackage
	sc := bufio.NewScanner(bytes.NewReader([]byte(output)))
	for sc.Scan() {
		line := sc.Text()
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			continue
		}
		packages = append(packages, InstalledPackage{Name: parts[0], Version: parts[1]})
	}
	return packages, sc.Err()
}

func TestParsePackages_NormalOutput(t *testing.T) {
	raw := "curl\t7.88.1-10+deb12u5\nopenssl\t3.0.11-1\nca-certificates\t20230311\n"
	pkgs, err := parsePackages(raw)
	require.NoError(t, err)
	require.Len(t, pkgs, 3)
	assert.Equal(t, InstalledPackage{Name: "curl", Version: "7.88.1-10+deb12u5"}, pkgs[0])
	assert.Equal(t, InstalledPackage{Name: "openssl", Version: "3.0.11-1"}, pkgs[1])
	assert.Equal(t, InstalledPackage{Name: "ca-certificates", Version: "20230311"}, pkgs[2])
}

func TestParsePackages_SkipsEmptyLines(t *testing.T) {
	raw := "curl\t7.88.1\n\n\nopenssl\t3.0.11\n"
	pkgs, err := parsePackages(raw)
	require.NoError(t, err)
	assert.Len(t, pkgs, 2)
}

func TestParsePackages_SkipsMalformedLines(t *testing.T) {
	raw := "no-tab-here\ncurl\t7.88.1\n\t\n"
	pkgs, err := parsePackages(raw)
	require.NoError(t, err)
	require.Len(t, pkgs, 1)
	assert.Equal(t, "curl", pkgs[0].Name)
}

func TestParsePackages_Empty(t *testing.T) {
	pkgs, err := parsePackages("")
	require.NoError(t, err)
	assert.Empty(t, pkgs)
}

// parseOSRelease mirrors the DetectOS parsing logic for unit testing
func parseOSRelease(output string) (*OSInfo, error) {
	parts := strings.Fields(strings.TrimSpace(output))
	if len(parts) < 2 {
		return nil, nil
	}
	id := strings.ToLower(parts[0])
	versionID := parts[1]
	codename := ""
	if len(parts) >= 3 {
		codename = parts[2]
	}
	var ecosystem string
	switch id {
	case "debian":
		ecosystem = "debian"
	case "ubuntu":
		ecosystem = "ubuntu"
	default:
		return nil, nil
	}
	return &OSInfo{Ecosystem: ecosystem, DistroVersion: versionID, Codename: codename}, nil
}

func TestParseOSRelease_Debian12(t *testing.T) {
	info, err := parseOSRelease("debian 12 bookworm")
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "debian", info.Ecosystem)
	assert.Equal(t, "12", info.DistroVersion)
	assert.Equal(t, "bookworm", info.Codename)
}

func TestParseOSRelease_Ubuntu2204(t *testing.T) {
	info, err := parseOSRelease("ubuntu 22.04 jammy")
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "ubuntu", info.Ecosystem)
	assert.Equal(t, "22.04", info.DistroVersion)
	assert.Equal(t, "jammy", info.Codename)
}

func TestParseOSRelease_UnsupportedOS(t *testing.T) {
	info, _ := parseOSRelease("alpine 3.18 edge")
	assert.Nil(t, info)
}
