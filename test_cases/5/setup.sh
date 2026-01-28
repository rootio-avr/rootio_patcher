#!/bin/bash
set -e

echo "========================================"
echo "Test Case 5: npm package remediation"
echo "========================================"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Clean up existing node_modules if present
if [ -d "$SCRIPT_DIR/node_modules" ]; then
    echo "Removing existing node_modules..."
    rm -rf "$SCRIPT_DIR/node_modules"
fi

# Install packages from package-lock.json
echo "Installing npm packages..."
cd "$SCRIPT_DIR"
npm ci

echo ""
echo "✓ Installation complete!"
echo ""
echo "Installed packages:"
npm list --depth=0

echo ""
echo "To run rootio_patcher remediation:"
echo "  cd $SCRIPT_DIR"
echo "  rootio_patcher npm remediate --file package-lock.json --dry-run=false"
