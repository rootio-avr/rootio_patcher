package common

import (
	"fmt"
	"strings"

	"rootio_patcher/pkg/rootio"
)

// ReportOsDryRun prints the dry-run summary for OS-level remediations (apt, apk, …).
// cmd is the full apply command shown to the user, e.g. "rootio_patcher apt remediate --dry-run=false".
// useAlias controls whether the aliased (rootio-*) or original package name is shown.
func ReportOsDryRun(response *rootio.OsAnalyzeResponse, cmd string, useAlias bool) {
	fmt.Println("\n=== DRY-RUN MODE ===")

	if len(response.Upgradeable) > 0 {
		fmt.Printf("\n%d package(s) upgradeable via official repo:\n", len(response.Upgradeable))
		for _, u := range response.Upgradeable {
			cves := ""
			if len(u.CVEIDs) > 0 {
				cves = " (fixes: " + strings.Join(u.CVEIDs, ", ") + ")"
			}
			fmt.Printf("  • %s %s → %s%s\n", u.PackageName, u.CurrentVersion, u.UpgradeVersion, cves)
		}
	}

	if len(response.Patches) > 0 {
		label := "Root.io alias"
		if !useAlias {
			label = "Root.io non-aliased"
		}
		fmt.Printf("\n%d package(s) patchable via %s:\n", len(response.Patches), label)
		for _, p := range response.Patches {
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
