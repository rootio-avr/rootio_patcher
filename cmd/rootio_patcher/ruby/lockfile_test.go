package ruby

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const lock = `GIT
  remote: https://github.com/x/y.git
  revision: abc
  specs:
    gitgem (0.1.0)

GEM
  remote: https://rubygems.org/
  specs:
    nokogiri (1.15.0-x86_64-linux)
    nokogiri (1.15.0)
      racc (~> 1.4)
    racc (1.7.1)
    rack (2.2.3)

PLATFORMS
  ruby

DEPENDENCIES
  gitgem!
  nokogiri (~> 1.15)
  rack

RUBY VERSION
   ruby 3.2.2p53

BUNDLED WITH
   2.4.10
`

func TestParseLockfile(t *testing.T) {
	gems := ParseLockfile(lock)
	assert.Equal(t, []LockedGem{
		{"nokogiri", "1.15.0", true},
		{"racc", "1.7.1", false},
		{"rack", "2.2.3", true},
	}, gems)
	assert.Equal(t, "2.2.3", ResolvedVersion(lock, "rack"))
	assert.Equal(t, "", ResolvedVersion(lock, "missing"))
}

func TestRestoreRubyAndBundlerInfo(t *testing.T) {
	updated := "RUBY VERSION\n   ruby 3.3.0p0\n\nBUNDLED WITH\n   2.5.0\n"
	got := RestoreRubyAndBundlerInfo(lock, updated)
	assert.Contains(t, got, "ruby 3.2.2p53")
	assert.Contains(t, got, "2.4.10")
}
