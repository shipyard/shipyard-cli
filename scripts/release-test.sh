#!/usr/bin/env bash
#
# Tests release_state (scripts/release-state.sh), which decides what
# release.sh offers after a failed release workflow. Only not-found may lead
# to the delete-tag prompt, so a gh error must never read as not-found.
#
# Usage: ./scripts/release-test.sh
#
set -euo pipefail

# shellcheck source=scripts/release-state.sh
source "$(cd "$(dirname "$0")" && pwd)/release-state.sh"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# A gh that answers `release view` the way GH_MODE says.
mkdir -p "${tmp}/bin"
cat > "${tmp}/bin/gh" <<'EOF'
#!/bin/sh
case "$GH_MODE" in
    published) echo false ;;
    draft)     echo true ;;
    draftwarn) echo "A new release of gh is available" >&2; echo true ;;
    missing)   echo "release not found" >&2; exit 1 ;;
    apierror)  echo "HTTP 503: Service Unavailable" >&2; exit 1 ;;
    garbled)   echo "<html>" ;;
esac
EOF
chmod +x "${tmp}/bin/gh"

failures=0
pass() { echo "ok   $1"; }
fail() { echo "FAIL $1" >&2; failures=$((failures + 1)); }

# expect_state <mode> <state> <name>: release_state prints state under mode.
expect_state() {
    local got
    got="$(GH_MODE=$1 PATH="${tmp}/bin:${PATH}" release_state v9.9.9 o/r 2> "${tmp}/err")"
    if [[ "$got" == "$2" ]]; then
        pass "$3"
    else
        fail "${3}: got \"${got}\", want \"${2}\""
    fi
}

expect_state published published "reads a published release"
expect_state draft draft "reads a draft"
expect_state draftwarn draft "reads a draft despite a gh warning"
expect_state missing not-found "reads a missing release"
expect_state apierror unknown "doesn't mistake an API error for a missing release"
grep -qF "HTTP 503" "${tmp}/err" || fail "an API error's message isn't passed on"
expect_state garbled unknown "doesn't guess from unexpected output"

if (( failures > 0 )); then
    echo "${failures} release_state test(s) failed" >&2
    exit 1
fi
echo "All release_state tests passed"
