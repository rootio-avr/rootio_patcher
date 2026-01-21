#!/bin/bash
set -e

echo "========================================"
echo "PyPI Patcher Test Cases Runner"
echo "========================================"
echo ""

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_CASES=("01_only_patchable" "02_only_non_patchable" "03_mixed" "04_transient_deps")

# Check if a specific test case was requested
if [ $# -eq 1 ]; then
    if [[ " ${TEST_CASES[@]} " =~ " $1 " ]]; then
        TEST_CASES=("$1")
        echo "Running single test case: $1"
        echo ""
    else
        echo "Error: Unknown test case '$1'"
        echo "Available test cases: ${TEST_CASES[*]}"
        exit 1
    fi
fi

# Function to run a test case
run_test_case() {
    local test_case=$1
    local test_dir="$SCRIPT_DIR/$test_case"

    if [ ! -f "$test_dir/setup.sh" ]; then
        echo "⚠️  Skipping $test_case: setup.sh not found"
        return
    fi

    echo "========================================"
    echo "Running: $test_case"
    echo "========================================"
    echo ""

    cd "$test_dir"
    bash setup.sh

    echo ""
    echo "✓ Completed: $test_case"
    echo ""
}

# Run all test cases
for test_case in "${TEST_CASES[@]}"; do
    run_test_case "$test_case"
done

echo "========================================"
echo "All test cases completed!"
echo "========================================"
echo ""
echo "Summary:"
for test_case in "${TEST_CASES[@]}"; do
    venv_path="$SCRIPT_DIR/$test_case/.venv"
    if [ -d "$venv_path" ]; then
        echo "✓ $test_case: $venv_path"
    else
        echo "✗ $test_case: Failed or skipped"
    fi
done
echo ""
echo "Usage examples:"
echo "  Run all:     bash $0"
echo "  Run single:  bash $0 01_only_patchable"
echo ""
echo "To activate a specific test environment:"
echo "  source $SCRIPT_DIR/01_only_patchable/.venv/bin/activate"
