package common

import (
	"fmt"
	"log/slog"
	"net/url"

	"rootio_patcher/pkg/rootio"
)

// Reporter handles terminal output and reporting
type Reporter struct {
	logger *slog.Logger
	pkgURL string
}

// NewReporter creates a new reporter
func NewReporter(pkgURL string, logger *slog.Logger) *Reporter {
	return &Reporter{
		logger: logger,
		pkgURL: pkgURL,
	}
}

// ReportDryRun shows what would be done in dry-run mode
func (r *Reporter) ReportDryRun(patches []rootio.PackagePatch, useAlias bool) {
	fmt.Println("\n=== DRY-RUN MODE ===")
	fmt.Println("The following operations would be performed:")
	fmt.Println()

	// Parse pkgURL to get scheme and host
	parsedURL, err := url.Parse(r.pkgURL)
	var indexURLTemplate string
	if err != nil {
		// Fallback if parsing fails
		indexURLTemplate = fmt.Sprintf("%s/pypi/simple/", r.pkgURL)
	} else {
		indexURLTemplate = fmt.Sprintf("%s://root:<your_api_key>@%s/pypi/simple/",
			parsedURL.Scheme, parsedURL.Host)
	}

	for i, patch := range patches {
		// Select patch based on useAlias flag
		var patchInfo rootio.PatchInfo
		var patchType string
		if useAlias {
			patchInfo = patch.PatchAlias
			patchType = "Aliased"
		} else {
			patchInfo = patch.Patch
			patchType = "Non-Aliased"
		}

		fmt.Printf("%d. Package: %s @ %s\n", i+1, patch.PackageName, patch.Version)
		fmt.Printf("   Patch (%s): %s @ %s\n", patchType, patchInfo.Name, patchInfo.Version)
		fmt.Printf("   CVEs Fixed: %v\n", patch.CVEIDs)
		fmt.Printf("   Commands:\n")
		fmt.Printf("     pip uninstall -y %s\n", patch.PackageName)
		fmt.Printf("     pip install --no-deps --index-url %s %s==%s\n\n",
			indexURLTemplate, patchInfo.Name, patchInfo.Version)
	}

	fmt.Println("To apply these patches, run with --dry-run=false")
	if useAlias {
		fmt.Println("To use original package names instead of aliases, add --use-alias=false")
	} else {
		fmt.Println("To use aliased package names (recommended), add --use-alias=true")
	}
}
