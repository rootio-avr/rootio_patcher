package common

import (
	"regexp"
	"strings"

	"rootio_patcher/pkg/rootio"
)

// pep503Separators collapses the characters PEP 503 treats as equivalent in a distribution name.
var pep503Separators = regexp.MustCompile(`[-_.]+`)

// PipRequirement is the pip requirement that installs the version a patch points to.
// A planned patch uses PEP 440 arbitrary equality (===) so only that exact version string
// matches: == 2.2.2 would also match the local version 2.2.2+root.io.1 and pip would pick the Root build.
func PipRequirement(patch rootio.PackagePatch) string {
	operator := "=="
	if patch.Planned {
		operator = "==="
	}
	return patch.Patch.Name + operator + patch.Patch.Version
}

// SamePipDistribution reports whether pip treats the two names as one distribution.
func SamePipDistribution(a, b string) bool {
	return pep503Separators.ReplaceAllString(strings.ToLower(a), "-") ==
		pep503Separators.ReplaceAllString(strings.ToLower(b), "-")
}
