#!/usr/bin/env bash
#
# Tests the release job's two integrity refusals with stand-in curl, gh and
# uname: install-release-tools.sh must not put a tool whose tarball doesn't
# match its pinned digest on the PATH, and verify-attestation.sh must not pass
# a release whose download is missing a file.
#
# Usage: ./scripts/release-tools-test.sh
#
set -euo pipefail

SCRIPTS="$(cd "$(dirname "$0")" && pwd)"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

failures=0
pass() { echo "ok   $1"; }
fail() { echo "FAIL $1" >&2; failures=$((failures + 1)); }

sha256() {
    if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'
    else shasum -a 256 "$1" | awk '{print $1}'; fi
}

# ── install-release-tools.sh ──────────────────────────────────────────────────

# Fake GoReleaser and syft tarballs, each holding a tool that prints the
# version line the installer greps for.
mkdir -p "${tmp}/tools"
printf '#!/bin/sh\necho "GitVersion:    0.0.1"\n' > "${tmp}/tools/goreleaser"
printf '#!/bin/sh\necho "Version:    0.0.2"\n' > "${tmp}/tools/syft"
chmod +x "${tmp}/tools/goreleaser" "${tmp}/tools/syft"
tar -czf "${tmp}/goreleaser.tgz" -C "${tmp}/tools" goreleaser
tar -czf "${tmp}/syft.tgz" -C "${tmp}/tools" syft
goreleaser_sum="$(sha256 "${tmp}/goreleaser.tgz")"
syft_sum="$(sha256 "${tmp}/syft.tgz")"

# The installer's pinned digests are for a Linux x86_64 runner, and it fetches
# with curl: the stand-ins report that platform and serve the fake tarballs.
# TAMPER names a tool whose tarball arrives altered.
mkdir -p "${tmp}/toolbin"
# shellcheck disable=SC2016 # $1 belongs to the stand-in
printf '#!/bin/sh\ncase "$1" in -s) echo Linux ;; -m) echo x86_64 ;; *) echo "Linux x86_64" ;; esac\n' \
    > "${tmp}/toolbin/uname"
# shellcheck disable=SC2016 # $1, $2 and $TAMPER belong to the stand-in
printf '#!/bin/sh\nout=""; url=""\nwhile [ $# -gt 0 ]; do case "$1" in --output) out=$2; shift ;; http*) url=$1 ;; esac; shift; done\ncase "$url" in *goreleaser*) tool=goreleaser ;; *syft*) tool=syft ;; *) exit 22 ;; esac\ncp "%s/${tool}.tgz" "$out"\nif [ "$TAMPER" = "$tool" ]; then echo tampered >> "$out"; fi\n' \
    "$tmp" > "${tmp}/toolbin/curl"
chmod +x "${tmp}/toolbin/uname" "${tmp}/toolbin/curl"

# run_tools <dir> [extra env...]: runs the installer with RUNNER_TEMP=<dir>,
# output in <dir>/out; returns its exit code.
run_tools() {
    local dir=$1; shift
    mkdir -p "$dir"
    : > "${dir}/github_path"
    env PATH="${tmp}/toolbin:${PATH}" RUNNER_TEMP="$dir" GITHUB_PATH="${dir}/github_path" \
        GORELEASER_VERSION=0.0.1 GORELEASER_SHA256="$goreleaser_sum" \
        SYFT_VERSION=0.0.2 SYFT_SHA256="$syft_sum" "$@" \
        "${SCRIPTS}/install-release-tools.sh" > "${dir}/out" 2>&1
}

if run_tools "${tmp}/good" && [[ "$(cat "${tmp}/good/github_path")" == "${tmp}/good/release-tools" ]] \
    && [[ -x "${tmp}/good/release-tools/goreleaser" && -x "${tmp}/good/release-tools/syft" ]]; then
    pass "installs tools whose digests match"
else
    fail "installs tools whose digests match"; cat "${tmp}/good/out" >&2
fi

for tool in goreleaser syft; do
    dir="${tmp}/tampered-${tool}"
    if run_tools "$dir" TAMPER="$tool"; then
        fail "refuses a tampered ${tool} tarball: installer succeeded"
    elif [[ -s "${dir}/github_path" ]]; then
        fail "refuses a tampered ${tool} tarball: tools were added to the PATH"
    elif [[ -e "${dir}/release-tools/${tool}" ]]; then
        fail "refuses a tampered ${tool} tarball: ${tool} was extracted"
    else
        pass "refuses a tampered ${tool} tarball"
    fi
done

# ── verify-attestation.sh ─────────────────────────────────────────────────────

# A gh whose release download fetches the names in DOWNLOAD and whose
# attestation verify logs its arguments and succeeds.
mkdir -p "${tmp}/ghbin"
# shellcheck disable=SC2016 # $1, $2 and $DOWNLOAD belong to the stand-in
printf '#!/bin/sh\ncase "$1 $2" in\n"release download")\n    while [ $# -gt 0 ]; do [ "$1" = -D ] && dir=$2; shift; done\n    for name in $DOWNLOAD; do echo "$name" > "$dir/$name"; done ;;\n"attestation verify") echo "$*" >> %s/verify.log ;;\n*) echo "unexpected gh call: $*" >&2; exit 2 ;;\nesac\n' \
    "$tmp" > "${tmp}/ghbin/gh"
chmod +x "${tmp}/ghbin/gh"

# run_verify [extra env...]: runs verify-attestation.sh on v9.9.9 of o/r, output
# in ${tmp}/out; returns its exit code.
run_verify() {
    : > "${tmp}/verify.log"
    env PATH="${tmp}/ghbin:${PATH}" "$@" "${SCRIPTS}/verify-attestation.sh" v9.9.9 o/r > "${tmp}/out" 2>&1
}

if run_verify DOWNLOAD="shipyard-linux-amd64 shipyard-linux-amd64.sbom.json" \
    && [[ "$(grep -c -- '--signer-workflow o/r/.github/workflows/release.yaml --source-ref refs/tags/v9.9.9' "${tmp}/verify.log")" == 2 ]]; then
    pass "verifies the binary and SBOM against the tag's release workflow"
else
    fail "verifies the binary and SBOM against the tag's release workflow"; cat "${tmp}/out" "${tmp}/verify.log" >&2
fi

if run_verify DOWNLOAD="shipyard-linux-amd64"; then
    fail "refuses a release missing its SBOM: verify-attestation.sh succeeded"
elif ! grep -qF "has no shipyard-linux-amd64.sbom.json" "${tmp}/out"; then
    fail "refuses a release missing its SBOM: failed for another reason"; cat "${tmp}/out" >&2
elif [[ -s "${tmp}/verify.log" ]]; then
    fail "refuses a release missing its SBOM: verified a partial download"
else
    pass "refuses a release missing its SBOM"
fi

if (( failures > 0 )); then
    echo "${failures} release tool test(s) failed" >&2
    exit 1
fi
echo "All release tool tests passed"
