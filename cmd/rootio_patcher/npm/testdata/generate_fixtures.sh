#!/bin/bash
set -e

echo "Generating lock file fixtures for testing..."
echo ""

# Get the directory where this script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Generate npm lock file
echo "=== Generating npm lock file ==="
cd "$SCRIPT_DIR/npm"
if command -v npm &> /dev/null; then
    rm -rf node_modules package-lock.json 2>/dev/null || true
    npm install --package-lock-only
    echo "✓ Generated package-lock.json"
else
    echo "⚠ npm not found, skipping"
fi
echo ""

# Generate yarn lock file
echo "=== Generating yarn lock file ==="
cd "$SCRIPT_DIR/yarn"
if command -v yarn &> /dev/null; then
    rm -rf node_modules yarn.lock 2>/dev/null || true
    yarn install --mode update-lockfile
    echo "✓ Generated yarn.lock"
else
    echo "⚠ yarn not found, skipping"
fi
echo ""

# Generate pnpm lock file
echo "=== Generating pnpm lock file ==="
cd "$SCRIPT_DIR/pnpm"
if command -v pnpm &> /dev/null; then
    rm -rf node_modules pnpm-lock.yaml 2>/dev/null || true
    pnpm install --lockfile-only
    echo "✓ Generated pnpm-lock.yaml"
else
    echo "⚠ pnpm not found, skipping"
fi
echo ""

echo "Done! Lock files generated in testdata/"
echo ""
echo "Generated files:"
ls -lh "$SCRIPT_DIR"/*/package-lock.json "$SCRIPT_DIR"/*/yarn.lock "$SCRIPT_DIR"/*/pnpm-lock.yaml 2>/dev/null | tail -n +2
