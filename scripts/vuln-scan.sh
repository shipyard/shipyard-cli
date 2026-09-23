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
# v1.8.1 and an unstripped scan called it unreachable. Because the verdict
# depends on it, a binary built without -s is refused rather than scanned.
#
# One host can scan every platform: govulncheck reads ELF, Mach-O and PE.
#
# A finding that genuinely does not apply is excepted in .govulncheck-ignore at
# the repo root (override with VULN_IGNORE_FILE), one per line:
#
#   <GO-ID> <expires YYYY-MM-DD> <reason>
#
# Every exception expires, so it is revisited rather than forgotten. An expired
# or malformed entry is not applied.
#
# Usage: ./scripts/vuln-scan.sh <binary> [binary...]
#
set -uo pipefail

GOVULNCHECK_VERSION="${GOVULNCHECK_VERSION:-v1.8.0}"
IGNORE_FILE="${VULN_IGNORE_FILE:-$(cd "$(dirname "$0")/.." && pwd)/.govulncheck-ignore}"

if [[ $# -eq 0 ]]; then
    echo "Usage: $0 <binary> [binary...]"
    exit 1
fi

# Load exceptions as a space-padded list of IDs. A malformed line fails the
# scan outright: an exception nobody can read is not one anybody reviewed.
EXCEPTED=" "
if [[ -f "$IGNORE_FILE" ]]; then
    TODAY=$(date -u +%Y-%m-%d)
    LINE_NO=0
    while IFS= read -r LINE || [[ -n "$LINE" ]]; do
        LINE_NO=$((LINE_NO + 1))
        [[ "$LINE" =~ ^[[:space:]]*(#|$) ]] && continue
        read -r ID EXPIRES REASON <<<"$LINE"
        if [[ ! "$ID" =~ ^GO-[0-9]{4}-[0-9]+$ || ! "$EXPIRES" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ || -z "$REASON" ]]; then
            echo "FAIL: $IGNORE_FILE:$LINE_NO: expected '<GO-ID> <YYYY-MM-DD> <reason>'"
            exit 1
        fi
        if [[ "$EXPIRES" < "$TODAY" ]]; then
            echo "Exception for $ID expired on $EXPIRES and is not applied"
            continue
        fi
        echo "Exception for $ID until $EXPIRES: $REASON"
        EXCEPTED="$EXCEPTED$ID "
    done <"$IGNORE_FILE"
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

    # An unstripped build lets govulncheck clear modules whose vulnerable
    # symbols it sees are never called, so it would pass what this gate exists
    # to catch. Go records the link flags in the binary; require -s among them.
    LDFLAGS=$(go version -m "$BINARY" 2>/dev/null | awk -F'\t' '$2 == "build" && $3 ~ /^-ldflags=/ { print $3 }')
    if [[ " ${LDFLAGS//[\"=]/ } " != *" -s "* ]]; then
        echo "ERROR: $BINARY was not built with -ldflags -s, so its scan would under-report"
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
            # 3 is govulncheck's documented "vulnerabilities found". Without
            # -show verbose, the text report lists only the findings behind that
            # verdict, one "Vulnerability #N: <ID>" line each.
            FOUND=$(echo "$OUTPUT" | sed -nE 's/^Vulnerability #[0-9]+: (GO-[0-9]+-[0-9]+).*/\1/p' | sort -u)
            UNEXCEPTED=""
            for ID in $FOUND; do
                [[ "$EXCEPTED" == *" $ID "* ]] || UNEXCEPTED="$UNEXCEPTED $ID"
            done
            if [[ -n "$FOUND" && -z "$UNEXCEPTED" ]]; then
                echo "PASS: $BINARY (excepted: ${FOUND//$'\n'/ })"
                CLEAN=$((CLEAN + 1))
            else
                echo "VULNERABLE: $BINARY"
                echo "$OUTPUT" | sed 's/^/  /'
                VULNERABLE=$((VULNERABLE + 1))
            fi
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
    echo "not apply, add it to .govulncheck-ignore with an expiry and the reason."
    exit 1
fi
