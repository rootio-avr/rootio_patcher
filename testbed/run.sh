#!/usr/bin/env bash
# Testbed for rootio_patcher yarn parsing.
#
# Reproduces:
#   1. Yarn 4 lockfile with __metadata.version: 9 -> currently rejected by
#      yarn2_parser.go's version allowlist (expected failure).
#   2. Yarn 4 lockfile with __metadata.version: 8 -> currently accepted
#      (control / sanity check).
#   3. Yarn 1 classic lockfile -> handled by yarn_parser.go (control;
#      paste the colleague's actual error to compare).
#
# We run with a fake ROOTIO_API_KEY because the version-gate error fires
# before any network call.

set -u
cd "$(dirname "$0")/.."

BIN="./rootio_patcher_testbed"

echo "==> Building rootio_patcher ..."
go build -o "$BIN" ./cmd/rootio_patcher || { echo "build failed"; exit 1; }

# Only set a fake key if the caller hasn't already provided one.
export ROOTIO_API_KEY="${ROOTIO_API_KEY:-testbed-fake-key}"
export LOG_LEVEL="${LOG_LEVEL:-debug}"

run_case() {
  local label="$1"
  local dir="$2"
  echo
  echo "==== $label  ($dir) ===="
  echo "-- yarn.lock first 6 lines --"
  head -n 6 "$dir/yarn.lock"
  echo "-- patcher output --"
  "$BIN" npm remediate \
      --package-manager=yarn \
      -C "$dir" \
      --dry-run=true 2>&1 | sed 's/^/    /'
  echo "(exit=$?)"
}

run_case "Yarn 1 classic"      testbed/yarn1
run_case "Yarn 4 (cacheKey v8)" testbed/yarn4_v8
run_case "Yarn 4 (cacheKey v9)" testbed/yarn4_v9

echo
echo "==> Done. Expected (with real ROOTIO_API_KEY):"
echo "    yarn1     : parses, suggests overrides"
echo "    yarn4_v8  : parses, suggests overrides"
echo "    yarn4_v9  : parses, suggests overrides  (regression check — was previously rejected)"
