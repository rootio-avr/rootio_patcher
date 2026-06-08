#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

usage() {
  echo "Usage: ROOTIO_API_KEY=<key> $0 [alpine318|all]"
  echo ""
  echo "  Builds the patcher binary, then runs the apk e2e test(s) inside Docker."
  echo "  ROOTIO_API_KEY must be set (real API key, calls are made against the live API)."
  echo ""
  echo "  ROOTIO_API_URL  override API base URL (default: https://api.root.io)"
  echo "  ROOTIO_PKG_URL  override package registry URL (default: https://pkg.root.io)"
  exit 1
}

if [[ -z "${ROOTIO_API_KEY:-}" ]]; then
  echo "ERROR: ROOTIO_API_KEY is not set"
  usage
fi

TARGET="${1:-all}"
ROOTIO_API_URL="${ROOTIO_API_URL:-https://api.root.io}"
ROOTIO_PKG_URL="${ROOTIO_PKG_URL:-https://pkg.root.io}"

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
  local tag="rootio-patcher-e2e-apk-${name}"

  echo ""
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "  Running e2e: ${name}"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

  docker build \
    --no-cache \
    --progress=plain \
    --file "$dockerfile" \
    --build-arg "ROOTIO_API_KEY=${ROOTIO_API_KEY}" \
    --build-arg "ROOTIO_API_URL=${ROOTIO_API_URL}" \
    --build-arg "ROOTIO_PKG_URL=${ROOTIO_PKG_URL}" \
    --tag "$tag" \
    "$SCRIPT_DIR"

  echo ""
  echo "✓ ${name} passed"
}

# ── 3. Dispatch ───────────────────────────────────────────────────────────────
case "$TARGET" in
  alpine318)
    run_test "alpine318" "$SCRIPT_DIR/Dockerfile.alpine318"
    ;;
  all)
    run_test "alpine318" "$SCRIPT_DIR/Dockerfile.alpine318"
    echo ""
    echo "✓ All apk e2e tests passed"
    ;;
  *)
    usage
    ;;
esac

# ── 4. Cleanup binary ─────────────────────────────────────────────────────────
rm -f "$BINARY"
