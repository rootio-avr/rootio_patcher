#!/bin/bash
set -e

echo "========================================"
echo "Test Case 7: yarn2+ package remediation"
echo "========================================"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Clean up existing node_modules if present
if [ -d "$SCRIPT_DIR/node_modules" ]; then
    echo "Removing existing node_modules..."
    rm -rf "$SCRIPT_DIR/node_modules"
fi

# Enable corepack and set yarn to stable (modern)
echo "Setting up Yarn Modern (v4+)..."
corepack enable

cd "$SCRIPT_DIR"

# Ensure we're using Yarn Modern
if [ ! -f ".yarnrc.yml" ]; then
    echo "Setting yarn version to stable..."
    yarn set version stable
fi

# Install packages from yarn.lock
echo "Installing yarn packages..."
yarn install

echo ""
echo "✓ Installation complete!"
echo ""
echo "Yarn version:"
yarn --version

echo ""
echo "Direct dependencies from package.json:"
node -e "const pkg = require('./package.json'); console.log('dependencies:', Object.keys(pkg.dependencies || {}).length); console.log('devDependencies:', Object.keys(pkg.devDependencies || {}).length);"

echo ""
echo "To run rootio_patcher remediation:"
echo "  cd $SCRIPT_DIR"
echo "  rootio_patcher npm remediate --package-manager yarn --dry-run=false"
echo ""
echo "Note: --package-manager yarn auto-detects Yarn v1 vs v2+ from yarn.lock format"
