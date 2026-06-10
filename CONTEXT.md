# rootio_patcher — OS Remediation Context

The `rootio_patcher` CLI scans installed packages, asks the Root.io API which have fixes,
and applies them on the running system. This context covers the OS package managers
(currently APT/deb and APK/alpine; more to come).

## Language

**Remediation run**:
A single `rootio_patcher <ecosystem> remediate` invocation against one system.

**Root fix** (a.k.a. Root patch):
A Root.io-built package installed from the Root.io registry that replaces or upgrades a
vulnerable package (deb: `rootio-*` alias or a pinned `root.io` version; apk: an `…007…`-suffixed build).
_Avoid_: bare "patch".

**Distro upgrade**:
Bringing a package to the latest version available in the **official distro repository**, with no
Root.io involvement (`apt-get install -y <name>` / `apk add --upgrade <name>`).
_Avoid_: bare "upgrade", "official fix".

**Patched set**:
The packages receiving a Root fix in a Remediation run — the `patches[]` of the analyze response.

**Upgrade set**:
The packages targeted for a Distro upgrade in a run, computed **client-side** as
`installed − Patched set − hardcoded blacklist − ignored`. Executed by package name, so the
package manager upgrades only those with a newer candidate and no-ops the rest. On by default;
suppressed entirely by the `--skip-upgrades` opt-out flag.

**Ignored packages**:
Packages the user lists in `.rootioignore` / `--ignore` (`name@version`). On the OS path these are
excluded **client-side** from both the Upgrade set and the Patched set — "leave these alone."

**Upgradeable** _(deprecated)_:
The legacy server-computed `upgradeable[]` list (packages with a CVE fix in `cve_tickets`).
Superseded by the client-computed Upgrade set; the field is no longer consulted.

**Full support vs upgrades-only distro**:
A distro has *full support* when it has a Root.io registry (Root fixes are possible). An
*upgrades-only* distro has no registry — only Distro upgrades apply.

## Relationships

- A **Remediation run** produces one **Patched set** and one **Upgrade set**; they are **disjoint** (Upgrade set = installed − Patched set).
- A **Root fix** is sourced from the Root.io registry; a **Distro upgrade** is sourced from the official distro repo.
- On an **upgrades-only** distro the Patched set is always empty.

## Flagged ambiguities

- "upgrade the packages with no Root fix" was ambiguous between *system-wide* (all installed) and *vulnerable-only*. Resolved: the **Upgrade set is all installed − Patched set**, but executed by name so the package manager filters to actual upgrades — "broad list, manager-filtered," matching how the backend `ecosystem/os` installs non-Root packages.
- "Upgradeable" (server field) vs the client-computed **Upgrade set**. Resolved: client computes the set; the server field is deprecated.
