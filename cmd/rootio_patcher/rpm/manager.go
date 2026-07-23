package rpm

// Manager describes an RPM-based package manager (yum, dnf, microdnf).
// Root.io has no targeted patches for these ecosystems, so the workflow they
// drive (see App) only ever upgrades installed packages to the latest
// available version — it never calls the analyze API or installs patches.
type Manager struct {
	// Name identifies the package manager in CLI/log output (e.g. "yum").
	Name string
	// Binary is the executable invoked to refresh metadata and upgrade packages.
	Binary string
	// RefreshArgs refreshes the package metadata cache. Empty means the
	// manager has no separate refresh step (it refreshes as part of upgrading).
	RefreshArgs []string
	// UpgradeArgs builds the full argument list to upgrade the named packages.
	UpgradeArgs func(names []string) []string
}

func YumManager() Manager {
	return Manager{
		Name:        "yum",
		Binary:      "yum",
		RefreshArgs: []string{"makecache"},
		UpgradeArgs: func(names []string) []string {
			return append([]string{"update", "-y"}, names...)
		},
	}
}

func DnfManager() Manager {
	return Manager{
		Name:        "dnf",
		Binary:      "dnf",
		RefreshArgs: []string{"makecache"},
		UpgradeArgs: func(names []string) []string {
			return append([]string{"upgrade", "-y"}, names...)
		},
	}
}

// MicrodnfManager targets microdnf, the minimal dnf variant found in
// UBI/minimal RHEL-family images.
func MicrodnfManager() Manager {
	return Manager{
		Name:   "microdnf",
		Binary: "microdnf",
		// microdnf has no makecache subcommand; --refresh on upgrade covers it.
		UpgradeArgs: func(names []string) []string {
			return append([]string{"upgrade", "-y", "--refresh"}, names...)
		},
	}
}
