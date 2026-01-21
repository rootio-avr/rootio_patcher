#!/bin/bash
set -e

echo "========================================"
echo "Test Case 2: Only Non-Patchable Vulnerabilities"
echo "========================================"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VENV_DIR="$SCRIPT_DIR/.venv"

# Clean up existing venv if present
if [ -d "$VENV_DIR" ]; then
    echo "Removing existing virtual environment..."
    rm -rf "$VENV_DIR"
fi

# Create virtual environment
echo "Creating virtual environment..."
python3 -m venv "$VENV_DIR"

# Activate virtual environment
echo "Activating virtual environment..."
source "$VENV_DIR/bin/activate"

# Upgrade pip
echo "Upgrading pip..."
pip install --upgrade pip

# Install packages
echo "Installing packages with non-patchable vulnerabilities..."
pip install -r "$SCRIPT_DIR/requirements.txt"

echo ""
echo "✓ Installation complete!"
echo ""
echo "Virtual environment: $VENV_DIR"
echo "To activate: source $VENV_DIR/bin/activate"
echo ""
echo "Installed packages:"
pip list | grep -E "(Django|PyYAML|urllib3)"
