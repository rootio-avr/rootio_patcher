package rootio

// Package represents a Python package with its version
type Package struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// AnalyzePackagesRequest is the request for analyzing packages for vulnerabilities
type AnalyzePackagesRequest struct {
	Packages []Package `json:"packages"`
	Ignore   []Package `json:"ignore,omitempty"`
}

// PatchInfo contains package name and version
type PatchInfo struct {
	Name    string `json:"name"`    // Package name
	Version string `json:"version"` // Package version
}

// PackagePatch represents a package that needs to be patched
type PackagePatch struct {
	PackageName string    `json:"package_name"` // Currently installed package name
	Version     string    `json:"version"`      // Currently installed version
	Patch       PatchInfo `json:"patch"`        // Patch details
	PatchAlias  PatchInfo `json:"patch_alias"`  // Root.io aliased package details
	CVEIDs      []string  `json:"cve_ids"`      // Fixed CVEs
}

// SkippedPackage represents a package that was skipped during analysis
type SkippedPackage struct {
	PackageName string `json:"package_name"`
	Reason      string `json:"reason"`
}

// AnalyzePackagesResponse is the response from the remediate endpoint
type AnalyzePackagesResponse struct {
	Patches []PackagePatch   `json:"patches"`
	Skipped []SkippedPackage `json:"skipped"`
}

// OsAnalyzeRequest is the request for OS-level package analysis endpoints (e.g. /v3/analyze/apt)
type OsAnalyzeRequest struct {
	Ecosystem       string    `json:"ecosystem"`
	OsDistroVersion string    `json:"os_distro_version"`
	Packages        []Package `json:"packages"`
}

// UpgradeableOsPackage represents a package with a fix available in the official distro repo
type UpgradeableOsPackage struct {
	PackageName    string   `json:"package_name"`
	CurrentVersion string   `json:"current_version"`
	UpgradeVersion string   `json:"upgrade_version"`
	CVEIDs         []string `json:"cve_ids"`
}

// OsAnalyzeResponse is the response from OS-level package analysis endpoints (e.g. /v3/analyze/apt)
type OsAnalyzeResponse struct {
	Patches     []PackagePatch         `json:"patches"`
	Upgradeable []UpgradeableOsPackage `json:"upgradeable"`
	Skipped     []SkippedPackage       `json:"skipped"`
}
