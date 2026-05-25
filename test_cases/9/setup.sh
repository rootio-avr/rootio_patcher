#!/bin/bash
set -e

echo "========================================"
echo "Test Case 9: Composer package remediation"
echo "symfony/http-foundation 6.4.3"
echo "========================================"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Check prerequisites
if ! command -v php &>/dev/null; then
    echo "ERROR: php not found in PATH"
    exit 1
fi
if ! command -v composer &>/dev/null; then
    echo "ERROR: composer not found in PATH"
    exit 1
fi

# Clean up existing vendor and lock file
if [ -d "$SCRIPT_DIR/vendor" ]; then
    echo "Removing existing vendor directory..."
    rm -rf "$SCRIPT_DIR/vendor"
fi
if [ -f "$SCRIPT_DIR/composer.lock" ]; then
    echo "Removing existing composer.lock..."
    rm "$SCRIPT_DIR/composer.lock"
fi

# Install packages — this resolves versions and generates composer.lock
echo "Installing Composer packages (this will generate composer.lock)..."
cd "$SCRIPT_DIR"
composer install --no-interaction --prefer-dist

echo ""
echo "✓ Installation complete!"
echo ""
echo "Installed packages:"
composer show --no-dev 2>/dev/null || composer show

echo ""
echo "To verify the library works:"
echo "  php $SCRIPT_DIR/index.php"
echo ""
echo "Environment variables:"
echo "  ROOTIO_API_KEY   (required) your Root.io API key"
echo "  ROOTIO_API_URL   (optional) API endpoint, default: https://api.root.io"
echo "  ROOTIO_PKG_URL   (optional) package registry URL, default: https://pkg.root.io"
echo "  LOG_LEVEL        (optional) debug|info|warn|error, default: info"
echo ""
echo "To run rootio_patcher (dry-run, production):"
echo "  ROOTIO_API_KEY=your-key \\"
echo "  rootio_patcher composer remediate --file=$SCRIPT_DIR/composer.json"
echo ""
echo "To run against a local/staging server:"
echo "  ROOTIO_API_KEY=your-key \\"
echo "  ROOTIO_API_URL=http://localhost:3000 \\"
echo "  ROOTIO_PKG_URL=http://localhost:8080 \\"
echo "  rootio_patcher composer remediate --file=$SCRIPT_DIR/composer.json"
echo ""
echo "To apply patches:"
echo "  ROOTIO_API_KEY=your-key \\"
echo "  rootio_patcher composer remediate --file=$SCRIPT_DIR/composer.json --dry-run=false"
