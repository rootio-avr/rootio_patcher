package apt

import (
	"context"
	"fmt"
)

// eolRule describes the sed transformations needed for an EOL distro codename.
type eolRule struct {
	// rewrites is a list of (file, fromURL, toURL) triples.
	// Each is guarded so a missing file is skipped rather than causing an error.
	rewrites []fileRewrite
	// dropUpdatesIn is the list of files from which "<codename>-updates" lines should be removed.
	// Each is guarded the same way.
	dropUpdatesIn []string
}

type fileRewrite struct {
	file string
	from string
	to   string
}

// eolCodenames maps each EOL codename to its rewrite rules.
// Add codenames here as releases reach EOL.
// Keep in sync with backend .../ecosystem/os/debian.go (debianEndOfLifeCodenames / ubuntuEndOfLifeCodenames).
var eolCodenames = map[string]eolRule{
	// Debian EOL releases
	"jessie": {
		rewrites: []fileRewrite{
			{"/etc/apt/sources.list", "http://deb.debian.org/debian", "http://archive.debian.org/debian"},
			{"/etc/apt/sources.list", "http://deb.debian.org/debian-security", "http://archive.debian.org/debian-security"},
		},
		dropUpdatesIn: []string{"/etc/apt/sources.list"},
	},
	"stretch": {
		rewrites: []fileRewrite{
			{"/etc/apt/sources.list", "http://deb.debian.org/debian", "http://archive.debian.org/debian"},
			{"/etc/apt/sources.list", "http://deb.debian.org/debian-security", "http://archive.debian.org/debian-security"},
		},
		dropUpdatesIn: []string{"/etc/apt/sources.list"},
	},
	"buster": {
		rewrites: []fileRewrite{
			{"/etc/apt/sources.list", "http://deb.debian.org/debian", "http://archive.debian.org/debian"},
			{"/etc/apt/sources.list", "http://deb.debian.org/debian-security", "http://archive.debian.org/debian-security"},
		},
		dropUpdatesIn: []string{"/etc/apt/sources.list"},
	},
	// Ubuntu EOL releases.
	//
	// NOTE: the "<codename>-updates" line-drop (dropUpdatesIn) is applied only to the
	// legacy /etc/apt/sources.list. For the deb822 /etc/apt/sources.list.d/ubuntu.sources,
	// "-updates" appears as a token within a "Suites:" line (not its own line), so we do
	// NOT drop it there; instead we rely on the archive mirror (old-releases.ubuntu.com)
	// serving the rewritten suites for EOL Ubuntu. This matches backend .../os/debian.go,
	// which likewise drops "-updates" only from sources.list. If the backend's approach
	// changes, keep these in sync.
	"mantic": {
		rewrites: []fileRewrite{
			{"/etc/apt/sources.list", "http://archive.ubuntu.com/ubuntu", "http://old-releases.ubuntu.com/ubuntu"},
			{"/etc/apt/sources.list", "http://security.ubuntu.com/ubuntu", "http://old-releases.ubuntu.com/ubuntu"},
			// deb822 format (Ubuntu >= 24.10); guarded — won't exist on older Ubuntu
			{"/etc/apt/sources.list.d/ubuntu.sources", "http://archive.ubuntu.com/ubuntu", "http://old-releases.ubuntu.com/ubuntu"},
			{"/etc/apt/sources.list.d/ubuntu.sources", "http://security.ubuntu.com/ubuntu", "http://old-releases.ubuntu.com/ubuntu"},
		},
		dropUpdatesIn: []string{"/etc/apt/sources.list"},
	},
	"oracular": {
		rewrites: []fileRewrite{
			{"/etc/apt/sources.list", "http://archive.ubuntu.com/ubuntu", "http://old-releases.ubuntu.com/ubuntu"},
			{"/etc/apt/sources.list", "http://security.ubuntu.com/ubuntu", "http://old-releases.ubuntu.com/ubuntu"},
			// deb822 format (Ubuntu >= 24.10); guarded — won't exist on older Ubuntu
			{"/etc/apt/sources.list.d/ubuntu.sources", "http://archive.ubuntu.com/ubuntu", "http://old-releases.ubuntu.com/ubuntu"},
			{"/etc/apt/sources.list.d/ubuntu.sources", "http://security.ubuntu.com/ubuntu", "http://old-releases.ubuntu.com/ubuntu"},
		},
		dropUpdatesIn: []string{"/etc/apt/sources.list"},
	},
}

// rewriteSourcesForEOL rewrites /etc/apt/sources.list (and ubuntu.sources where applicable)
// so that EOL distros can reach their archive mirrors instead of the now-404 standard mirrors.
// If codename is not in the EOL table this is a no-op.
// All file operations are guarded: a missing file is silently skipped so the patcher
// doesn't abort on containers that lack a particular sources file.
// The transformations are idempotent.
func (e *Executor) rewriteSourcesForEOL(ctx context.Context, codename string) error {
	rule, ok := eolCodenames[codename]
	if !ok {
		return nil
	}

	for _, rw := range rule.rewrites {
		script := fmt.Sprintf(
			`test -f %s && sed -i 's|%s|%s|g' %s || true`,
			rw.file, rw.from, rw.to, rw.file,
		)
		if err := e.runner.Run(ctx, "sh", "-c", script); err != nil {
			return fmt.Errorf("eol rewrite %s: %w", rw.file, err)
		}
	}

	for _, f := range rule.dropUpdatesIn {
		script := fmt.Sprintf(
			`test -f %s && sed -i '/%s-updates/d' %s || true`,
			f, codename, f,
		)
		if err := e.runner.Run(ctx, "sh", "-c", script); err != nil {
			return fmt.Errorf("eol drop-updates %s: %w", f, err)
		}
	}

	return nil
}
