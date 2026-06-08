#!/usr/bin/env bash
#
# Smoke test for the shipyard CLI binary.
# Validates that a built binary is functional without any network access.
#
# Usage: ./scripts/smoke-test.sh <path-to-binary>
#
set -euo pipefail

BINARY="${1:-}"
if [[ -z "$BINARY" ]]; then
    echo "Usage: $0 <path-to-binary>"
    exit 1
fi

if [[ ! -x "$BINARY" ]]; then
    echo "FAIL: $BINARY is not executable or does not exist"
    exit 1
fi

PASS=0
FAIL=0

check() {
    local name="$1"
    shift
    if "$@" >/dev/null 2>&1; then
        echo "PASS: $name"
        PASS=$((PASS + 1))
    else
        echo "FAIL: $name"
        FAIL=$((FAIL + 1))
    fi
}

check_output() {
    local name="$1"
    local pattern="$2"
    shift 2
    local output
    output=$("$@" 2>&1) || true
    if echo "$output" | grep -qE "$pattern"; then
        echo "PASS: $name"
        PASS=$((PASS + 1))
    else
        echo "FAIL: $name (expected pattern: $pattern)"
        echo "  got: $output"
        FAIL=$((FAIL + 1))
    fi
}

check_output_not() {
    local name="$1"
    local pattern="$2"
    shift 2
    local output
    output=$("$@" 2>&1) || true
    if echo "$output" | grep -qE "$pattern"; then
        echo "FAIL: $name (unexpected pattern found: $pattern)"
        echo "  got: $output"
        FAIL=$((FAIL + 1))
    else
        echo "PASS: $name"
        PASS=$((PASS + 1))
    fi
}

echo "=== Shipyard CLI Smoke Tests ==="
echo "Binary: $BINARY"
echo ""

# 1. Binary executes
check "binary executes (--version exits 0)" "$BINARY" --version

# 2. Version is stamped (not "undefined")
check_output_not "version is stamped (not undefined)" "undefined" "$BINARY" --version

# 3. Help text contains expected subcommands
for subcmd in get login update rebuild exec logs mcp; do
    check_output "help contains '$subcmd' subcommand" "$subcmd" "$BINARY" --help
done

# 4. Subcommand help works
check "get --help exits 0" "$BINARY" get --help
check "mcp --help exits 0" "$BINARY" mcp --help

# 5. Config init with temp HOME doesn't crash
TEMP_HOME=$(mktemp -d)
check "config init with temp HOME" env HOME="$TEMP_HOME" "$BINARY" --help
rm -rf "$TEMP_HOME"

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="

if [[ "$FAIL" -gt 0 ]]; then
    exit 1
fi
