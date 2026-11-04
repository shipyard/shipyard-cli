#!/usr/bin/env bash
#
# Tests scripts/check-reproducible.sh with GoReleaser stubbed: matching digests
# pass, and a changed digest, a binary missing from the rebuild or a dist/ with
# no binaries fail it. The rebuild must not share the caller's Go build cache.
#
# Usage: ./scripts/check-reproducible-test.sh
#
set -euo pipefail

CHECK="$(cd "$(dirname "$0")" && pwd)/check-reproducible.sh"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# check-reproducible.sh clones the repo it runs in, so give it a small one.
repo="${tmp}/repo"
git init --quiet "$repo"
cp "$CHECK" "${repo}/check-reproducible.sh"
git -C "$repo" add check-reproducible.sh
git -C "$repo" -c user.name=test -c user.email=test@example.com commit --quiet -m test

# The stub writes the rebuild's checksums.txt from STUB_SUMS and records the
# GOCACHE the rebuild ran with.
mkdir -p "${tmp}/bin"
cat > "${tmp}/bin/goreleaser" <<'EOF'
#!/usr/bin/env bash
echo "${GOCACHE:-}" > "$STUB_GOCACHE_LOG"
mkdir -p dist
cp "$STUB_SUMS" dist/checksums.txt
EOF
chmod +x "${tmp}/bin/goreleaser"

export PATH="${tmp}/bin:${PATH}"
export STUB_SUMS="${tmp}/rebuilt.txt"
export STUB_GOCACHE_LOG="${tmp}/gocache.log"
export GOCACHE="${tmp}/outer-gocache"

dist="${tmp}/dist"
mkdir -p "$dist"
sum() { printf '%064d' "$1"; }

failures=0
pass() { echo "ok   $1"; }
fail() { echo "FAIL $1" >&2; failures=$((failures + 1)); }

run_check() { (cd "$repo" && "${repo}/check-reproducible.sh" "$dist") > "${tmp}/out" 2>&1; }

# expect_failure <name> <message>: check-reproducible.sh must exit non-zero and say why.
expect_failure() {
    if run_check; then
        fail "${1}: check-reproducible.sh passed"
    elif ! grep -qF "$2" "${tmp}/out"; then
        fail "${1}: missing \"${2}\""; cat "${tmp}/out" >&2
    else
        pass "$1"
    fi
}

# Matching binaries pass. The SBOM is skipped: the rebuild doesn't make one.
printf '%s  shipyard-linux-amd64\n%s  shipyard-linux-amd64.sbom.json\n' "$(sum 1)" "$(sum 2)" > "${dist}/checksums.txt"
printf '%s  shipyard-linux-amd64\n' "$(sum 1)" > "$STUB_SUMS"
if run_check; then
    pass "accepts matching binaries and skips SBOMs"
else
    fail "accepts matching binaries and skips SBOMs"; cat "${tmp}/out" >&2
fi

rebuild_cache="$(cat "$STUB_GOCACHE_LOG")"
if [[ -n "$rebuild_cache" && "$rebuild_cache" != "$GOCACHE" ]]; then
    pass "rebuilds with its own Go build cache"
else
    fail "rebuilds with its own Go build cache: GOCACHE was '${rebuild_cache}'"
fi

printf '%s  shipyard-linux-amd64\n' "$(sum 9)" > "$STUB_SUMS"
expect_failure "refuses a changed digest" "FAIL shipyard-linux-amd64"

printf '%s  shipyard-darwin-arm64\n' "$(sum 1)" > "$STUB_SUMS"
expect_failure "refuses a binary missing from the rebuild" "rebuilt missing"

printf '%s  shipyard-linux-amd64.sbom.json\n' "$(sum 2)" > "${dist}/checksums.txt"
expect_failure "refuses a dist with no binaries" "No binaries"

rm "${dist}/checksums.txt"
expect_failure "refuses a dist with no checksums.txt" "No ${dist}/checksums.txt"

[[ "$failures" -eq 0 ]] || { echo "${failures} check(s) failed" >&2; exit 1; }
echo "All check-reproducible.sh tests passed"
