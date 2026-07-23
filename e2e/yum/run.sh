#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

usage() {
  echo "Usage: $0 [rockylinux8|rockylinux8-ignore|all]"
  echo ""
  echo "  Builds the patcher binary, then runs the yum e2e test(s) inside Docker."
  echo "  Root.io has no targeted patches for yum, so these tests only exercise"
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
  local tag="rootio-patcher-e2e-yum-${name}"

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
  rockylinux8)
    run_test "rockylinux8" "$SCRIPT_DIR/Dockerfile.rockylinux8"
    ;;
  rockylinux8-ignore)
    run_test "rockylinux8-ignore" "$SCRIPT_DIR/Dockerfile.rockylinux8.ignore"
    ;;
  all)
    run_test "rockylinux8" "$SCRIPT_DIR/Dockerfile.rockylinux8"
    run_test "rockylinux8-ignore" "$SCRIPT_DIR/Dockerfile.rockylinux8.ignore"
    echo ""
    echo "✓ All yum e2e tests passed"
    ;;
  *)
    usage
    ;;
esac

# ── 4. Cleanup binary ─────────────────────────────────────────────────────────
rm -f "$BINARY"
