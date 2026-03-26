#!/usr/bin/env bash
# benchmark.sh — measure Gradle build times with or without the Root.io plugin.
#
# Usage:
#   ./benchmark.sh [OPTIONS] [-- GRADLE_ARGS...]
#
# Options:
#   -n, --runs N          Number of builds to run (default: 10)
#   -t, --task TASK       Gradle task to run (default: build)
#   -d, --dir DIR         Gradle project directory (default: current dir)
#       --full-clean      Also clear ~/.gradle/caches/modules* between runs
#                         (simulates cold CI; warning: very slow, re-downloads all deps)
#   -h, --help            Show this help
#
# Examples:
#   # Run 10 builds of the current project
#   ./benchmark.sh
#
#   # Compare with/without plugin
#   ./benchmark.sh -n 15 -d /path/to/my-project
#   # (toggle the plugin in build.gradle.kts, then run again)
#
#   # Pass extra flags to gradle
#   ./benchmark.sh -- --no-daemon --info
#
# Output: median, mean, p95, min, max build times in seconds.

set -euo pipefail

# ── defaults ────────────────────────────────────────────────────────────────
RUNS=10
TASK="build"
PROJECT_DIR="."
FULL_CLEAN=false
EXTRA_GRADLE_ARGS=()

# ── parse args ───────────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
    case "$1" in
        -n|--runs)      RUNS="$2";        shift 2 ;;
        -t|--task)      TASK="$2";        shift 2 ;;
        -d|--dir)       PROJECT_DIR="$2"; shift 2 ;;
        --full-clean)   FULL_CLEAN=true;  shift   ;;
        -h|--help)
            sed -n '2,/^set -euo/{ /^set -euo/d; s/^# \{0,1\}//; p }' "$0"
            exit 0 ;;
        --)             shift; EXTRA_GRADLE_ARGS=("$@"); break ;;
        *) echo "Unknown option: $1" >&2; exit 1 ;;
    esac
done

PROJECT_DIR="$(cd "$PROJECT_DIR" && pwd)"

# ── helpers ──────────────────────────────────────────────────────────────────
RED='\033[0;31m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; BOLD='\033[1m'; RESET='\033[0m'

info()  { echo -e "${CYAN}[bench]${RESET} $*"; }
warn()  { echo -e "${YELLOW}[bench]${RESET} $*"; }
error() { echo -e "${RED}[bench]${RESET} $*" >&2; }

# Pick gradlew or fall back to system gradle
if [[ -x "$PROJECT_DIR/gradlew" ]]; then
    GRADLE="$PROJECT_DIR/gradlew"
elif command -v gradle &>/dev/null; then
    GRADLE="gradle"
else
    error "No gradlew found in $PROJECT_DIR and no 'gradle' on PATH"
    exit 1
fi

# Portable high-resolution timestamp in milliseconds
now_ms() {
    if command -v gdate &>/dev/null; then
        gdate +%s%3N          # macOS with coreutils
    elif date +%s%3N | grep -q '^[0-9]\{13\}$'; then
        date +%s%3N           # Linux date supports %3N
    else
        python3 -c 'import time; print(int(time.time()*1000))'
    fi
}

clean_between_runs() {
    info "Cleaning caches..."

    # Plugin's own response cache (always clear so API calls happen every run)
    rm -rf "$PROJECT_DIR/.gradle/rootio-cache"

    # Gradle build cache and configuration cache
    rm -rf "$PROJECT_DIR/.gradle/build-cache"
    rm -rf "$PROJECT_DIR/.gradle/configuration-cache"
    rm -rf "$PROJECT_DIR/build"

    # Also clear sub-project build dirs (multi-module projects)
    find "$PROJECT_DIR" -mindepth 2 -maxdepth 3 -name "build" -type d \
        ! -path "$PROJECT_DIR/.gradle/*" -exec rm -rf {} + 2>/dev/null || true

    if $FULL_CLEAN; then
        warn "--full-clean: removing ~/.gradle/caches/modules* (will re-download deps)"
        rm -rf ~/.gradle/caches/modules-*
    fi
}

# ── pre-flight ────────────────────────────────────────────────────────────────
if [[ ! -f "$PROJECT_DIR/build.gradle.kts" && ! -f "$PROJECT_DIR/build.gradle" ]]; then
    error "No build.gradle(.kts) found in $PROJECT_DIR"
    exit 1
fi

echo
echo -e "${BOLD}Gradle Build Benchmark${RESET}"
echo "  Project : $PROJECT_DIR"
echo "  Task    : $TASK"
echo "  Runs    : $RUNS"
echo "  Gradle  : $GRADLE"
$FULL_CLEAN && echo -e "  Mode    : ${YELLOW}full-clean (re-downloads deps)${RESET}" \
           || echo "  Mode    : project-clean (keeps downloaded deps)"
[[ ${#EXTRA_GRADLE_ARGS[@]} -gt 0 ]] && echo "  Extra   : ${EXTRA_GRADLE_ARGS[*]}"
echo

# ── main loop ─────────────────────────────────────────────────────────────────
declare -a TIMES_MS   # store raw millisecond durations

for ((i = 1; i <= RUNS; i++)); do
    info "Run $i / $RUNS"
    clean_between_runs

    T_START=$(now_ms)

    # Run gradle; suppress noisy output but keep errors visible
    "$GRADLE" -p "$PROJECT_DIR" "$TASK" \
        --no-configuration-cache \
        "${EXTRA_GRADLE_ARGS[@]+"${EXTRA_GRADLE_ARGS[@]}"}" \
        2>&1 | tail -5      # show last few lines so user sees success/failure

    T_END=$(now_ms)
    ELAPSED=$(( T_END - T_START ))
    TIMES_MS+=("$ELAPSED")

    printf "  → %.2fs\n\n" "$(echo "scale=2; $ELAPSED / 1000" | bc)"
done

# ── statistics (pure bash + awk, no python required) ─────────────────────────
compute_stats() {
    local -a arr=("$@")
    local n=${#arr[@]}

    # Sort ascending
    IFS=$'\n' sorted=($(echo "${arr[*]}" | tr ' ' '\n' | sort -n))
    unset IFS

    awk -v n="$n" '
    BEGIN {
        # Read values from ARGV into vals[]
    }
    {
        for (i = 1; i <= NF; i++) vals[NR*100+i] = $1
    }
    ' <<< "" # placeholder – we use awk inline below

    # Use awk to compute stats from space-separated list
    printf '%s\n' "${sorted[@]}" | awk -v n="$n" '
    {
        vals[NR] = $1
        sum += $1
    }
    END {
        mean = sum / n

        # median
        if (n % 2 == 1)
            median = vals[int(n/2) + 1]
        else
            median = (vals[n/2] + vals[n/2 + 1]) / 2.0

        # p95: nearest-rank method
        p95_idx = int(0.95 * n + 0.5)
        if (p95_idx < 1) p95_idx = 1
        if (p95_idx > n) p95_idx = n
        p95 = vals[p95_idx]

        min_v = vals[1]
        max_v = vals[n]

        printf "mean=%.0f median=%.0f p95=%.0f min=%.0f max=%.0f\n",
               mean, median, p95, min_v, max_v
    }
    '
}

STATS=$(compute_stats "${TIMES_MS[@]}")

# Parse individual values from "key=val ..." output (bash 3 compatible)
S_mean=$(echo   "$STATS" | grep -o 'mean=[0-9]*'   | cut -d= -f2)
S_median=$(echo "$STATS" | grep -o 'median=[0-9]*' | cut -d= -f2)
S_p95=$(echo    "$STATS" | grep -o 'p95=[0-9]*'    | cut -d= -f2)
S_min=$(echo    "$STATS" | grep -o 'min=[0-9]*'    | cut -d= -f2)
S_max=$(echo    "$STATS" | grep -o 'max=[0-9]*'    | cut -d= -f2)

ms_to_s() { echo "scale=2; $1 / 1000" | bc; }

echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
echo -e "${BOLD}Results (${RUNS} runs, task: ${TASK})${RESET}"
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
printf "  %-8s  %6ss\n" "Median"  "$(ms_to_s "$S_median")"
printf "  %-8s  %6ss\n" "Mean"    "$(ms_to_s "$S_mean")"
printf "  %-8s  %6ss\n" "P95"     "$(ms_to_s "$S_p95")"
printf "  %-8s  %6ss\n" "Min"     "$(ms_to_s "$S_min")"
printf "  %-8s  %6ss\n" "Max"     "$(ms_to_s "$S_max")"
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"

# Raw times for reference
echo -e "\nRaw times (s):"
printf '  '
for ms in "${TIMES_MS[@]}"; do
    printf "%.2f  " "$(ms_to_s "$ms")"
done
echo
