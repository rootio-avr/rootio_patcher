#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

usage() {
  echo "Usage: ROOTIO_API_KEY=<key> $0 [debian12|ubuntu2204|all]"
  echo ""
  echo "  Builds the patcher binary, then runs the apt e2e test(s) inside Docker."
  echo "  ROOTIO_API_KEY must be set (real API key, calls are made against the live API)."
  echo ""
  echo "  ROOTIO_API_URL  override API base URL (default: https://api.root.io)"
  exit 1
}

if [[ -z "${ROOTIO_API_KEY:-}" ]]; then
  echo "ERROR: ROOTIO_API_KEY is not set"
  usage
fi

TARGET="${1:-all}"
ROOTIO_API_URL="${ROOTIO_API_URL:-https://api.root.io}"

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
  local tag="rootio-patcher-e2e-apt-${name}"

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
    --tag "$tag" \
    "$SCRIPT_DIR"

  echo ""
  echo "✓ ${name} passed"
}

# ── 3. Dispatch ───────────────────────────────────────────────────────────────
case "$TARGET" in
  debian12)
    run_test "debian12" "$SCRIPT_DIR/Dockerfile.debian12"
    ;;
  ubuntu2204)
    run_test "ubuntu2204" "$SCRIPT_DIR/Dockerfile.ubuntu2204"
    ;;
  all)
    run_test "debian12"  "$SCRIPT_DIR/Dockerfile.debian12"
    run_test "ubuntu2204" "$SCRIPT_DIR/Dockerfile.ubuntu2204"
    echo ""
    echo "✓ All apt e2e tests passed"
    ;;
  *)
    usage
    ;;
esac

# ── 4. Cleanup binary ─────────────────────────────────────────────────────────
rm -f "$BINARY"
