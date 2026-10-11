package ruby

import (
	"regexp"
	"strings"
)

// LockedGem is a gem resolved in Gemfile.lock.
type LockedGem struct {
	Name    string
	Version string
	Direct  bool
}

var (
	lockSpecRe    = regexp.MustCompile(`^ {4}(\S+) \(([^)]+)\)\s*$`)
	lockDepRe     = regexp.MustCompile(`^ {2}(\S+?)!?(?: \(.*\))?!?\s*$`)
	rubyInfoRe    = regexp.MustCompile(`RUBY VERSION\s+ruby\s+\S+`)
	bundlerInfoRe = regexp.MustCompile(`BUNDLED WITH\s+\S+`)
	platformRe    = regexp.MustCompile(`^(\d[\w.]*?)-[a-zA-Z].*$`)
)

// ParseLockfile returns the gems from rubygems-sourced (GEM) sections of Gemfile.lock.
// Gems from GIT and PATH sections are skipped: they can't be matched against an index.
// Platform suffixes ("1.15.0-x86_64-linux") are stripped from versions.
func ParseLockfile(content string) []LockedGem {
	var gems []LockedGem
	direct := map[string]bool{}
	section := ""
	inSpecs := false
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		if line != "" && line[0] != ' ' {
			section = strings.TrimSpace(line)
			inSpecs = false
			continue
		}
		switch section {
		case "GEM":
			if strings.TrimSpace(line) == "specs:" {
				inSpecs = true
			} else if m := lockSpecRe.FindStringSubmatch(line); m != nil && inSpecs {
				gems = append(gems, LockedGem{Name: m[1], Version: stripPlatform(m[2])})
			}
		case "DEPENDENCIES":
			if m := lockDepRe.FindStringSubmatch(line); m != nil {
				direct[m[1]] = true
			}
		}
	}
	seen := map[string]bool{}
	out := gems[:0]
	for _, g := range gems {
		if seen[g.Name+"@"+g.Version] {
			continue // same gem listed per-platform
		}
		seen[g.Name+"@"+g.Version] = true
		g.Direct = direct[g.Name]
		out = append(out, g)
	}
	return out
}

func stripPlatform(v string) string {
	if m := platformRe.FindStringSubmatch(v); m != nil {
		return m[1]
	}
	return v
}

// ResolvedVersion returns the version of name resolved in the lockfile, or "".
func ResolvedVersion(content, name string) string {
	for _, g := range ParseLockfile(content) {
		if g.Name == name {
			return g.Version
		}
	}
	return ""
}

// RestoreRubyAndBundlerInfo copies the RUBY VERSION / BUNDLED WITH blocks of original into
// updated, so running a different Ruby/Bundler than the project's doesn't churn the lockfile.
func RestoreRubyAndBundlerInfo(original, updated string) string {
	for _, re := range []*regexp.Regexp{rubyInfoRe, bundlerInfoRe} {
		if orig := re.FindString(original); orig != "" {
			updated = re.ReplaceAllLiteralString(updated, orig)
		}
	}
	return updated
}
