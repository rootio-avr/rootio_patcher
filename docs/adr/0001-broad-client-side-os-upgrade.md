# 1. Compute the OS Upgrade set client-side and upgrade all non-Root packages

Date: 2026-06-10

## Status

Proposed

## Context

`rootio_patcher`'s OS path (apt/apk) applies two kinds of remediation: **Root fixes**
(the Patched set, installed from `pkg.root.io`) and **Distro upgrades** of packages that
have no Root fix.

The original (ENG-3669) implementation drove Distro upgrades from a server-computed
`upgradeable[]` list returned by `/v3/analyze/{apt,apk}`. That list is sourced from
`cve_tickets` (packages with a tracked CVE that has an official fixed version) — so it only
upgrades CVE-tracked packages and misses everything else that simply has a newer distro
version. The intent of the feature is broader: bring **every** non-Root package up to date.

Forces at play:
- The patcher already scans every installed package and receives the Patched set in the
  analyze response, so it can compute the upgrade target locally with no server help.
- The backend image-patching path (`internal/services/v3/patcher/ecosystem/os`) upgrades
  non-Root packages with a plain install-by-name (`apt-get install -y <names>` /
  `apk add --upgrade <names>`), letting the package manager pick the latest candidate.
- The patcher's APT `Setup` adds the Root.io repo under a **global** `Pin-Priority: 1001`.
  A broad upgrade run under that pin raised a fear of "leaking" rootio versions into
  non-Root packages.

## Decision

We will compute the **Upgrade set client-side** as
`installed − Patched set − hardcoded blacklist − ignored`, and upgrade it by name
(`apt-get install -y <list>` / `apk add --upgrade <list>`), on by default, atomically.
The server `upgradeable[]` field is no longer consulted.

Key sub-decisions:
- **Broad scope**, not CVE-scoped — every installed non-Root package is offered for upgrade;
  the package manager no-ops those already current.
- **Sequencing**: `Setup (repo+pin) → install patches → delete the rootio source + pin (+ index
  refresh) → broad upstream upgrade → final cleanup`. The rootio source is **removed before** the
  upstream upgrade so that step physically cannot resolve a rootio candidate — a structural
  guarantee deliberately chosen over *relying on* the curated registry (defense-in-depth). A
  parallel-agent debate had concluded it was safe to leave the source in place (curated registry,
  disjoint sets, `blockOriginalFromRegistry`); we override that in favor of the stronger guarantee.
- **Patch survival**: patches are installed *before* the source is removed (they need the repo),
  and they survive the subsequent upstream upgrade by **exclusion + no-downgrade** — patched
  packages are absent from the upgrade list and the package managers do not downgrade by default.
  No `apt-mark hold` / apk world-pin is used; we accept the residual risk that a listed package
  hard-requiring a higher version of a patched dependency could pull the upstream version.
- On **upgrades-only** distros (no patches) there is no source to remove: index-update →
  upstream upgrade → clear caches.
- **Atomic, fail-hard**: any failure aborts the run with a non-zero exit (matches the backend).
- **Opt-out** via `--skip-upgrades`; per-package opt-out via client-side `--ignore`/`.rootioignore`
  (applied to both the Upgrade set and the Patched set).
- **EOL distros**: port the backend's archive-mirror source rewriting (buster/stretch/oracular).

## Consequences

### Positive

- Upgrades far more than CVE-tracked packages — maximal vuln reduction for non-Root packages.
- No API change required; the client owns the diff. Server role shrinks to `patches` + `skipped`.
- Uses the backend's install-by-name model for upgrades.
- The upstream upgrade *provably* cannot pull rootio versions — the source is gone before it runs,
  so "no overrides" holds structurally rather than by trusting registry hygiene.

### Negative

- **Bigger blast radius**: a broad `apt-get install` is one transaction; under fail-hard a single
  un-upgradable package aborts the whole run, including Root patches. Mitigated by the hardcoded
  blacklist, `--skip-upgrades`, and `--ignore`.
- Larger, less deterministic image/system changes than a CVE-scoped upgrade.
- Dry-run can only list Upgrade-set **names** (no `current → target`/CVE detail), because the
  client doesn't know target versions.
- EOL source-rewriting is now duplicated logic to keep in sync with the backend `debian.go`.
- **Residual patch-clobber risk**: with the source/pin removed, the upstream upgrade is unprotected;
  a listed package that hard-requires a newer version of a patched dependency can pull the upstream
  version and silently undo that patch. Accepted (no holds); the curated registry + disjoint sets
  remain a secondary safety net.

### Neutral

- `OsExecutor.InstallUpgrades` changes signature from `[]UpgradeableOsPackage` to `[]string`.
- `--allow-downgrades` is dropped from the upgrade command (a distro upgrade should never downgrade).
- `UpgradeableOsPackage` / the response `upgradeable[]` field remain for back-compat but unused;
  backend cleanup is a separate change.
- The pattern generalizes to future ecosystems (`dnf upgrade <list>`, `zypper update <list>`).
- **Patches now run before upgrades** (reversed from the original order), and a new executor step
  deletes the rootio source + pin and refreshes the index between the patch and upgrade phases.
  `Cleanup` splits into "remove source/pin" (mid-run) and "remove key/auth + clear caches" (end).
