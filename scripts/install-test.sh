#!/usr/bin/env bash
#
# Tests scripts/install.sh against a fake release served from a file:// URL,
# with no network access and nothing installed outside a temp dir. Covers the
# checksum refusals, the install target guards and the platform check.
#
# Usage: ./scripts/install-test.sh
#
set -euo pipefail

INSTALL_SH="$(cd "$(dirname "$0")" && pwd)/install.sh"
VERSION=9.9.9

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

sha256() {
    if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'
    else shasum -a 256 "$1" | awk '{print $1}'; fi
}

case $(uname -s) in Linux) os=linux ;; Darwin) os=darwin ;; esac
case $(uname -m) in x86_64) arch=amd64 ;; arm64 | aarch64) arch=arm64 ;; esac
asset="shipyard-${os}-${arch}"

# A fake release: a binary that prints its version, and its checksums.txt.
release="${tmp}/release/download/v${VERSION}"
mkdir -p "$release"
printf '#!/bin/sh\necho "fake shipyard %s"\n' "$VERSION" > "${release}/${asset}"
good_sum="$(sha256 "${release}/${asset}")"
write_sums() { printf '%s  %s\n' "$@" > "${release}/checksums.txt"; }

failures=0
pass() { echo "ok   $1"; }
fail() { echo "FAIL $1" >&2; failures=$((failures + 1)); }

# run_install <install-dir> [extra env...]: runs install.sh with its output
# in ${tmp}/out; returns its exit code.
run_install() {
    local dir=$1; shift
    env SHIPYARD_RELEASES_URL="file://${tmp}/release" SHIPYARD_VERSION="$VERSION" \
        SHIPYARD_INSTALL_DIR="$dir" "$@" bash "$INSTALL_SH" > "${tmp}/out" 2>&1
}

# expect_refusal <name> <message> <install-dir> [extra env...]
expect_refusal() {
    local name=$1 message=$2 dir=$3; shift 3
    if run_install "$dir" "$@"; then
        fail "${name}: install.sh succeeded"
    elif ! grep -qF "$message" "${tmp}/out"; then
        fail "${name}: missing \"${message}\""; cat "${tmp}/out" >&2
    elif [[ -f "${dir}/shipyard" && ! -L "${dir}/shipyard" ]]; then
        fail "${name}: a binary was installed anyway"
    else
        pass "$name"
    fi
}

# Installs into a missing nested dir, runnable by everyone even under umask 077.
write_sums "$good_sum" "$asset" deadbeef "${asset}.sbom.json"
dir="${tmp}/a/b/bin"
# shellcheck disable=SC2012 # ls -l is the portable way to read the mode string
if (umask 077 && run_install "$dir") \
    && [[ "$("${dir}/shipyard")" == "fake shipyard ${VERSION}" ]] \
    && [[ "$(ls -l "${dir}/shipyard" | cut -c1-10)" == "-rwxr-xr-x" ]]; then
    pass "installs a verified binary with mode 0755"
else
    fail "installs a verified binary with mode 0755"; cat "${tmp}/out" >&2
fi

# A quoted "~/..." install dir lands under $HOME.
mkdir -p "${tmp}/home"
# shellcheck disable=SC2088 # the literal tilde is what's under test
if run_install '~/.local/bin' HOME="${tmp}/home" && [[ -x "${tmp}/home/.local/bin/shipyard" ]]; then
    pass "expands a quoted ~/ install dir"
else
    fail "expands a quoted ~/ install dir"; cat "${tmp}/out" >&2
fi

write_sums 0000000000000000000000000000000000000000000000000000000000000000 "$asset"
expect_refusal "refuses a checksum mismatch" "Checksum mismatch" "${tmp}/mismatch"

write_sums "$good_sum" "${asset}.sbom.json"
expect_refusal "refuses an asset missing from checksums.txt" "has no entry for ${asset}" "${tmp}/missing"

write_sums "$good_sum" "$asset"
mkdir -p "${tmp}/cellar" "${tmp}/brew"
echo original > "${tmp}/cellar/shipyard"
ln -s ../cellar/shipyard "${tmp}/brew/shipyard"
expect_refusal "refuses to replace a symlink" "is a symlink" "${tmp}/brew"
[[ "$(cat "${tmp}/cellar/shipyard")" == original ]] || fail "symlink target was modified"

mkdir -p "${tmp}/dirtarget/shipyard"
expect_refusal "refuses a directory at the target" "is a directory" "${tmp}/dirtarget"

# An unbuilt architecture is refused up front, not with a download 404.
mkdir -p "${tmp}/shim"
real_uname="$(command -v uname)"
# shellcheck disable=SC2016 # $1 and $@ belong to the shim
printf '#!/bin/sh\nif [ "$1" = -m ]; then echo i686; else exec %s "$@"; fi\n' "$real_uname" > "${tmp}/shim/uname"
chmod +x "${tmp}/shim/uname"
expect_refusal "refuses an unsupported platform" "Platform not supported" "${tmp}/i686" PATH="${tmp}/shim:${PATH}"

if (( failures > 0 )); then
    echo "${failures} install.sh test(s) failed" >&2
    exit 1
fi
echo "All install.sh tests passed"
