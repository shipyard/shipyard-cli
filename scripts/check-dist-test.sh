#!/usr/bin/env bash
#
# Tests scripts/check-dist.sh against a fake GoReleaser dist/: a complete one
# passes, and each kind of gap it exists to catch fails it.
#
# Usage: ./scripts/check-dist-test.sh
#
set -euo pipefail

CHECK="$(cd "$(dirname "$0")" && pwd)/check-dist.sh"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
dist="${tmp}/dist"

binaries=(
    shipyard-darwin-amd64 shipyard-darwin-arm64
    shipyard-linux-amd64 shipyard-linux-arm64
    shipyard-windows-amd64.exe shipyard-windows-arm64.exe
)

# make_dist: a dist/ with every binary and SPDX SBOM listed in checksums.txt.
# The binaries themselves aren't in dist/ under these names, as with GoReleaser.
make_dist() {
    rm -rf "$dist"
    mkdir -p "$dist"
    local binary
    for binary in "${binaries[@]}"; do
        echo '{"spdxVersion":"SPDX-2.3"}' > "${dist}/${binary}.sbom.json"
        printf '%064d  %s\n' 0 "$binary" >> "${dist}/checksums.txt"
        (cd "$dist" && sha256sum "${binary}.sbom.json") >> "${dist}/checksums.txt"
    done
}

# drop_line <name>: removes name's entry from checksums.txt.
drop_line() {
    awk -v name="$1" '$2 != name' "${dist}/checksums.txt" > "${dist}/checksums.new"
    mv "${dist}/checksums.new" "${dist}/checksums.txt"
}

failures=0
pass() { echo "ok   $1"; }
fail() { echo "FAIL $1" >&2; failures=$((failures + 1)); }

# expect_failure <name> <message>: check-dist.sh must exit non-zero and say why.
expect_failure() {
    if "$CHECK" "$dist" > "${tmp}/out" 2>&1; then
        fail "${1}: check-dist.sh passed"
    elif ! grep -qF "$2" "${tmp}/out"; then
        fail "${1}: missing \"${2}\""; cat "${tmp}/out" >&2
    else
        pass "$1"
    fi
}

make_dist
if "$CHECK" "$dist" > "${tmp}/out" 2>&1; then
    pass "accepts a complete dist"
else
    fail "accepts a complete dist"; cat "${tmp}/out" >&2
fi

make_dist
drop_line shipyard-linux-arm64
expect_failure "refuses an unlisted binary" "shipyard-linux-arm64: not in checksums.txt"

make_dist
drop_line shipyard-darwin-arm64.sbom.json
expect_failure "refuses an unlisted SBOM" "shipyard-darwin-arm64.sbom.json: not in checksums.txt"

make_dist
echo '{"bomFormat":"CycloneDX"}' > "${dist}/shipyard-linux-amd64.sbom.json"
expect_failure "refuses an SBOM that isn't SPDX" "not an SPDX document"

make_dist
echo '{"spdxVersion":"SPDX-2.3","packages":[]}' > "${dist}/shipyard-windows-amd64.exe.sbom.json"
expect_failure "refuses an SBOM that doesn't match its digest" "doesn't match its digest"

make_dist
rm "${dist}/checksums.txt"
expect_failure "refuses a dist with no checksums.txt" "No ${dist}/checksums.txt"

if (( failures > 0 )); then
    echo "${failures} check-dist.sh test(s) failed" >&2
    exit 1
fi
echo "All check-dist.sh tests passed"
