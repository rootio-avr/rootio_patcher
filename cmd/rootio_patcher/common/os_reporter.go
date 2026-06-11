package common

import (
	"fmt"
	"strings"

	"rootio_patcher/pkg/rootio"
)

// ReportOsDryRun prints the dry-run summary for OS-level remediations (apt, apk, …).
// filteredPatches are the patches that will actually be applied (ignore list already applied),
// and upgradeNames are the package names that will be offered for a broad upstream upgrade.
// cmd is the full apply command shown to the user, e.g. "rootio_patcher apt remediate --dry-run=false".
// useAlias controls whether the aliased (rootio-*) or original package name is shown.
func ReportOsDryRun(response *rootio.OsAnalyzeResponse, filteredPatches []rootio.PackagePatch, upgradeNames []string, cmd string, useAlias bool) {
	fmt.Println("\n=== DRY-RUN MODE ===")

	if len(upgradeNames) > 0 {
		fmt.Printf("\n%d package(s) will be offered for upgrade:\n", len(upgradeNames))
		for _, name := range upgradeNames {
			fmt.Printf("  • %s\n", name)
		}
	}

	if len(filteredPatches) > 0 {
		label := "Root.io alias"
		if !useAlias {
			label = "Root.io non-aliased"
		}
		fmt.Printf("\n%d package(s) patchable via %s:\n", len(filteredPatches), label)
		for _, p := range filteredPatches {
			cves := ""
			if len(p.CVEIDs) > 0 {
				cves = " (fixes: " + strings.Join(p.CVEIDs, ", ") + ")"
			}
			patchInfo := GetPatchInfo(p, useAlias)
			fmt.Printf("  • %s %s → %s %s%s\n",
				p.PackageName, p.Version,
				patchInfo.Name, patchInfo.Version,
				cves)
		}
	}

	fmt.Printf("\nTo apply: %s\n", cmd)
}
