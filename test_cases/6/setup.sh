#!/bin/bash
set -e

echo "========================================"
echo "Test Case 6: Maven package remediation"
echo "========================================"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Verify Maven is installed
if ! command -v mvn &> /dev/null; then
    echo "Error: Maven is not installed"
    echo "Install with: brew install maven"
    exit 1
fi

# Clean up existing target if present
if [ -d "$SCRIPT_DIR/target" ]; then
    echo "Cleaning existing build artifacts..."
    cd "$SCRIPT_DIR"
    mvn clean
fi

# Validate and download dependencies
echo "Validating pom.xml and downloading dependencies..."
cd "$SCRIPT_DIR"
mvn dependency:resolve

echo ""
echo "✓ Setup complete!"
echo ""
echo "Dependencies:"
mvn dependency:tree

echo ""
echo "To run rootio_patcher remediation:"
echo "  cd $SCRIPT_DIR"
echo "  rootio_patcher maven remediate --file pom.xml --dry-run=false"
