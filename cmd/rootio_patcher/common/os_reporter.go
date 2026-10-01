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
func ReportOsDryRun(response *rootio.OsAnalyzeResponse, filteredPatches []rootio.PackagePatch, upgradeNames []string, cmd string) {
	fmt.Println("\n=== DRY-RUN MODE ===")

	if len(upgradeNames) > 0 {
		fmt.Printf("\n%d package(s) will be offered for upgrade:\n", len(upgradeNames))
		for _, name := range upgradeNames {
			fmt.Printf("  • %s\n", name)
		}
	}

	if len(filteredPatches) > 0 {
		fmt.Printf("\n%d package(s) patchable via Root.io:\n", len(filteredPatches))
		for _, p := range filteredPatches {
			cves := ""
			if len(p.CVEIDs) > 0 {
				cves = " (fixes: " + strings.Join(p.CVEIDs, ", ") + ")"
			}
			fmt.Printf("  • %s %s → %s %s%s\n",
				p.PackageName, p.Version,
				p.Patch.Name, p.Patch.Version,
				cves)
		}
	}

	fmt.Printf("\nTo apply: %s\n", cmd)
}
