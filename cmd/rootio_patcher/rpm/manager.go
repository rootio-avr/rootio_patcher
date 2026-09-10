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

// releaseVerArgs returns ["--releasever=<v>"], or nil when v is empty. Amazon
// Linux 2023 minimal images pin a stale release snapshot in their repo
// config, so upstream fixes (e.g. openssl-fips-provider-latest) are
// unavailable until the caller overrides it, typically with "latest".
func releaseVerArgs(releaseVer string) []string {
	if releaseVer == "" {
		return nil
	}
	return []string{"--releasever=" + releaseVer}
}

func YumManager(releaseVer string) Manager {
	return Manager{
		Name:        "yum",
		Binary:      "yum",
		RefreshArgs: append([]string{"makecache"}, releaseVerArgs(releaseVer)...),
		UpgradeArgs: func(names []string) []string {
			args := append([]string{"update", "-y"}, releaseVerArgs(releaseVer)...)
			return append(args, names...)
		},
	}
}

func DnfManager(releaseVer string) Manager {
	return Manager{
		Name:        "dnf",
		Binary:      "dnf",
		RefreshArgs: append([]string{"makecache"}, releaseVerArgs(releaseVer)...),
		UpgradeArgs: func(names []string) []string {
			args := append([]string{"upgrade", "-y"}, releaseVerArgs(releaseVer)...)
			return append(args, names...)
		},
	}
}

// MicrodnfManager targets microdnf, the minimal dnf variant found in
// UBI/minimal RHEL-family images (and Amazon Linux 2023 minimal images).
func MicrodnfManager(releaseVer string) Manager {
	return Manager{
		Name:   "microdnf",
		Binary: "microdnf",
		// microdnf has no makecache subcommand; --refresh on upgrade covers it.
		UpgradeArgs: func(names []string) []string {
			args := append([]string{"upgrade", "-y", "--refresh"}, releaseVerArgs(releaseVer)...)
			return append(args, names...)
		},
	}
}
