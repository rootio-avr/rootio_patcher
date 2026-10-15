package common

import "rootio_patcher/pkg/rootio"

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
