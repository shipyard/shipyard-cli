#!/usr/bin/env bash
#
# Vulnerability scan for built shipyard binaries.
#
# Scans the bytes that ship, not the source tree, so a vulnerable module
# compiled into the release is caught even when nothing in this repo calls it.
# That is deliberate: the binary carries the vulnerable code to every user, and
# a call path that does not exist today can appear in the next refactor.
#
# Release binaries are built with -s -w. Stripped of their symbol tables,
# govulncheck cannot do call-graph analysis and falls back to a module-level
# verdict, which is the conservative answer this gate wants. Scanning an
# unstripped build instead would report far less: GO-2026-5942 shipped in
# v1.8.1 and an unstripped scan called it unreachable.
#
# One host can scan every platform: govulncheck reads ELF, Mach-O and PE.
#
# Usage: ./scripts/vuln-scan.sh <binary> [binary...]
#
set -uo pipefail

GOVULNCHECK_VERSION="${GOVULNCHECK_VERSION:-v1.8.0}"

if [[ $# -eq 0 ]]; then
    echo "Usage: $0 <binary> [binary...]"
    exit 1
fi

# Prefer an installed govulncheck, then GOPATH/bin, then install the pinned
# version. The vulnerability database is fetched at scan time, so pinning the
# scanner keeps the tool reproducible without freezing what it knows about.
GOVULNCHECK=""
if command -v govulncheck >/dev/null 2>&1; then
    GOVULNCHECK=govulncheck
elif [[ -x "$(go env GOPATH)/bin/govulncheck" ]]; then
    GOVULNCHECK="$(go env GOPATH)/bin/govulncheck"
else
    echo "Installing govulncheck $GOVULNCHECK_VERSION..."
    if ! go install "golang.org/x/vuln/cmd/govulncheck@$GOVULNCHECK_VERSION"; then
        echo "FAIL: could not install govulncheck"
        exit 1
    fi
    GOVULNCHECK="$(go env GOPATH)/bin/govulncheck"
fi

echo "=== Shipyard Vulnerability Scan ==="
"$GOVULNCHECK" -version 2>&1 | sed 's/^/  /'
echo ""

CLEAN=0
VULNERABLE=0
ERRORED=0

for BINARY in "$@"; do
    if [[ ! -f "$BINARY" ]]; then
        echo "ERROR: $BINARY does not exist"
        ERRORED=$((ERRORED + 1))
        continue
    fi

    OUTPUT=$("$GOVULNCHECK" -mode=binary "$BINARY" 2>&1)
    STATUS=$?

    case "$STATUS" in
        0)
            echo "PASS: $BINARY"
            CLEAN=$((CLEAN + 1))
            ;;
        3)
            # 3 is govulncheck's documented "vulnerabilities found".
            echo "VULNERABLE: $BINARY"
            echo "$OUTPUT" | sed 's/^/  /'
            VULNERABLE=$((VULNERABLE + 1))
            ;;
        *)
            # Anything else is the scanner failing, which must not read as a
            # clean bill of health.
            echo "ERROR: $BINARY (govulncheck exited $STATUS)"
            echo "$OUTPUT" | sed 's/^/  /'
            ERRORED=$((ERRORED + 1))
            ;;
    esac
done

echo ""
echo "=== Results: $CLEAN clean, $VULNERABLE vulnerable, $ERRORED errored ==="

if [[ "$VULNERABLE" -gt 0 || "$ERRORED" -gt 0 ]]; then
    echo ""
    echo "Upgrade the module the report names, or, if the finding genuinely does"
    echo "not apply, record why before releasing."
    exit 1
fi
