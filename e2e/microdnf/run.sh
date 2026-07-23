#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

usage() {
  echo "Usage: $0 [ubi9-minimal|ubi9-minimal-ignore|all]"
  echo ""
  echo "  Builds the patcher binary, then runs the microdnf e2e test(s) inside Docker."
  echo "  Root.io has no targeted patches for microdnf, so these tests only exercise"
  echo "  the broad upgrade-all path — no API key or network calls to root.io."
  exit 1
}

TARGET="${1:-all}"

# ── 1. Build the patcher binary matching the Docker host architecture ─────────
DOCKER_ARCH="$(docker version --format '{{.Server.Arch}}' 2>/dev/null || uname -m)"
case "$DOCKER_ARCH" in
  arm64|aarch64) GOARCH=arm64 ;;
  *)             GOARCH=amd64 ;;
esac
echo "==> Building rootio_patcher (linux/${GOARCH})..."
BINARY="$SCRIPT_DIR/rootio_patcher"
(cd "$REPO_ROOT" && GOOS=linux GOARCH="$GOARCH" go build -o "$BINARY" ./cmd/rootio_patcher)
echo "    Binary: $BINARY"

# ── 2. Run a single Dockerfile test ──────────────────────────────────────────
run_test() {
  local name="$1"
  local dockerfile="$2"
  local tag="rootio-patcher-e2e-microdnf-${name}"

  echo ""
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "  Running e2e: ${name}"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

  docker build \
    --no-cache \
    --progress=plain \
    --file "$dockerfile" \
    --tag "$tag" \
    "$SCRIPT_DIR"

  echo ""
  echo "✓ ${name} passed"
}

# ── 3. Dispatch ───────────────────────────────────────────────────────────────
case "$TARGET" in
  ubi9-minimal)
    run_test "ubi9-minimal" "$SCRIPT_DIR/Dockerfile.ubi9-minimal"
    ;;
  ubi9-minimal-ignore)
    run_test "ubi9-minimal-ignore" "$SCRIPT_DIR/Dockerfile.ubi9-minimal.ignore"
    ;;
  all)
    run_test "ubi9-minimal" "$SCRIPT_DIR/Dockerfile.ubi9-minimal"
    run_test "ubi9-minimal-ignore" "$SCRIPT_DIR/Dockerfile.ubi9-minimal.ignore"
    echo ""
    echo "✓ All microdnf e2e tests passed"
    ;;
  *)
    usage
    ;;
esac

# ── 4. Cleanup binary ─────────────────────────────────────────────────────────
rm -f "$BINARY"
