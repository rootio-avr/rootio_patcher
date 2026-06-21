# Implementation Blueprint: Client-Side Broad OS Upgrade

> Companion to `docs/adr/0001-broad-client-side-os-upgrade.md` and `CONTEXT.md`.
> CLI uses **kong** (flags are struct tags on `*RemediateCmd`), not cobra.

## Files to modify

### 1. `cmd/rootio_patcher/common/os_app.go`
- Add `skipUpgrades bool` and `ignoreSet map[string]struct{}` to `OsApp[T]` and `NewOsApp`.
- Add `PackageBlacklist map[string]bool` to `OsAppConfig[T]` (per-ecosystem, no interface change).
- New package-level helpers:
  - `computeUpgradeSet(installed, patches, blacklist, ignoreSet) []string` — names = installed − patchedNames − blacklist − ignoredNames (ignore keys are `name@version`; match by name).
  - `filterPatches(patches, ignoreSet) []rootio.PackagePatch` — drop patches whose `PackageName` is ignored.
- `OsExecutor` interface: change `InstallUpgrades(ctx, names []string)`; add `RemoveRootioRepo(ctx)`; `Cleanup` becomes end-of-run only.
- **Reordered `Run()`**:
  - has-patches: `Setup → InstallPatches → RemoveRootioRepo → (if !skipUpgrades) InstallUpgrades → Cleanup`
  - upgrades-only: `IndexUpdate → (if !skipUpgrades) InstallUpgrades → PostUpgradesOnly`
  - "nothing to do" must also account for `skipUpgrades` (if skipUpgrades && no patches → nothing).

### 2. `cmd/rootio_patcher/common/os_reporter.go`
- New signature `ReportOsDryRun(response, filteredPatches, upgradeNames, cmd, useAlias)`.
- Replace the `response.Upgradeable` block with: `"%d package(s) will be offered for upgrade"` + the names. Patches section iterates `filteredPatches`.

### 3. `cmd/rootio_patcher/apt/executor.go`
- `InstallUpgrades(ctx, names []string)` → `apt-get install -y <names>` (DROP `--allow-downgrades`).
- New `RemoveRootioRepo`: `rm rootio.list`, `rm preferences.d/rootio`, then `IndexUpdate`.
- `Cleanup` → GPG key + auth.conf + clear `/var/lib/apt/lists/*` only.
- Wire `rewriteSourcesForEOL(codename)` into `Setup` before `apt-get update`.

### 4. `cmd/rootio_patcher/apt/eol.go` (NEW)
- `eolCodenames` table (buster/stretch/jessie → archive.debian.org; oracular/mantic → old-releases.ubuntu.com; `dropUpdates`).
- `rewriteSourcesForEOL(ctx, codename)`: no-op unless EOL; rewrite base URLs in `/etc/apt/sources.list`, drop `-updates`. Prefer the existing `sh -c` sed style (consistent with `addSource`/`setPinPriority`) unless unit-testing demands an `aptFS` abstraction like apk has.

### 5. `cmd/rootio_patcher/apk/executor.go`
- `InstallUpgrades(ctx, names []string)` → `apk add --upgrade <names>`; remove the internal `packageBlacklist` check (now applied upstream in `computeUpgradeSet`) but leave a comment documenting the invariant.
- New `RemoveRootioRepo`: remove the rootio line from `/etc/apk/repositories`, then `IndexUpdate`.
- `Cleanup` → remove key file only.

### 6. `cmd/rootio_patcher/apt/app.go` + `apk/app.go`
- `NewApp`/`NewAppWithServices` gain `skipUpgrades bool, ignoreSet map[string]struct{}`; thread to `common.NewOsApp`.
- Set `PackageBlacklist` in `aptConfig()` (empty initially) and `apkConfig()` (`alpine-baselayout: true`).

### 7. `cmd/rootio_patcher/main.go`
- Add to `AptRemediateCmd` + `ApkRemediateCmd`:
  - `SkipUpgrades bool \`default:"false" help:"Skip broad upstream upgrade; apply patches only"\``
  - `Ignore []string \`help:"..." name:"ignore" sep:","\`` (matches pip/npm pattern).
- `Run`: `ignoreSet := common.LoadIgnoreList(".rootioignore", cmd.Ignore)`, pass new args.

## Build sequence (independently testable)
1. Interface + `OsApp` core + `computeUpgradeSet`/`filterPatches` (+ unit tests).
2. Reporter signature/output (+ test).
3. apt executor: `InstallUpgrades`, `RemoveRootioRepo`, split `Cleanup`.
4. apt EOL (`eol.go`) + wire into `Setup` (+ table-driven test).
5. apk executor: same shape changes.
6. apt/apk `app.go` signatures + blacklist config.
7. main.go kong flags + ignore loading.

## Risks / verify
- Confirm `common.InstalledPackage` definition location (likely `common/services.go`) for the import in `computeUpgradeSet`.
- `NewAppWithServices` takes the concrete `*Executor`; implement `RemoveRootioRepo` on it (Step 3/5) before Step 6 or it won't compile.
- Keep `rootio.UpgradeableOsPackage` + `response.Upgradeable` for JSON-decode back-compat; never read them.
- `PostUpgradesOnly` unchanged (apt clears caches via `Cleanup` in has-patches path; apk no-op).
- Residual patch-clobber risk (accepted): no holds; rely on exclusion + no-downgrade.
