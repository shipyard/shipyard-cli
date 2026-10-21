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

# An existing install is replaced whole, with no staged copy left beside it.
mkdir -p "${tmp}/upgrade"
echo old > "${tmp}/upgrade/shipyard"
if run_install "${tmp}/upgrade" \
    && [[ "$("${tmp}/upgrade/shipyard")" == "fake shipyard ${VERSION}" ]] \
    && [[ -z "$(find "${tmp}/upgrade" -name '.shipyard.new.*')" ]]; then
    pass "replaces an existing install"
else
    fail "replaces an existing install"; cat "${tmp}/out" >&2
fi

# A failed rename fails the install and leaves no staged copy behind.
mkdir -p "${tmp}/mvshim" "${tmp}/mvfail"
printf '#!/bin/sh\nexit 1\n' > "${tmp}/mvshim/mv"
chmod +x "${tmp}/mvshim/mv"
if ! run_install "${tmp}/mvfail" PATH="${tmp}/mvshim:${PATH}" \
    && [[ -z "$(find "${tmp}/mvfail" -name '.shipyard.new.*')" && ! -e "${tmp}/mvfail/shipyard" ]]; then
    pass "cleans up after a failed rename"
else
    fail "cleans up after a failed rename"; cat "${tmp}/out" >&2
fi

# With no version given, it's read from where releases/latest redirects, as for
# every curl | bash user. A curl shim answers that one request.
mkdir -p "${tmp}/curlshim"
# shellcheck disable=SC2016 # $* and $@ belong to the shim
printf '#!/bin/sh\ncase "$*" in *url_effective*) echo "https://github.com/shipyard/shipyard-cli/releases/tag/v%s"; exit 0 ;; esac\nexec %s "$@"\n' \
    "$VERSION" "$(command -v curl)" > "${tmp}/curlshim/curl"
chmod +x "${tmp}/curlshim/curl"
if run_install "${tmp}/latest" SHIPYARD_VERSION= PATH="${tmp}/curlshim:${PATH}" \
    && [[ "$("${tmp}/latest/shipyard")" == "fake shipyard ${VERSION}" ]]; then
    pass "installs the latest release by default"
else
    fail "installs the latest release by default"; cat "${tmp}/out" >&2
fi

if run_install "${tmp}/vprefix" SHIPYARD_VERSION="v${VERSION}" && [[ -x "${tmp}/vprefix/shipyard" ]]; then
    pass "accepts a v-prefixed version"
else
    fail "accepts a v-prefixed version"; cat "${tmp}/out" >&2
fi

# sudo is used only for a dir we can't write, as /usr/local/bin usually is. The
# shim logs each call and makes the dir writable, standing in for root.
if (( EUID == 0 )); then
    echo "skip sudo cases: running as root"
else
    mkdir -p "${tmp}/sudoshim" "${tmp}/readonly" "${tmp}/writable"
    chmod 555 "${tmp}/readonly"
    # shellcheck disable=SC2016 # $* and $@ belong to the shim
    printf '#!/bin/sh\necho "$*" >> %s/sudo.log\nchmod u+w %s/readonly\nexec "$@"\n' "$tmp" "$tmp" > "${tmp}/sudoshim/sudo"
    chmod +x "${tmp}/sudoshim/sudo"
    : > "${tmp}/sudo.log"
    if run_install "${tmp}/readonly" PATH="${tmp}/sudoshim:${PATH}" \
        && grep -q '^install -m 0755 ' "${tmp}/sudo.log" \
        && grep -q '^mv -f ' "${tmp}/sudo.log" \
        && [[ -x "${tmp}/readonly/shipyard" ]]; then
        pass "uses sudo for a dir it can't write"
    else
        fail "uses sudo for a dir it can't write"; cat "${tmp}/out" "${tmp}/sudo.log" >&2
    fi
    : > "${tmp}/sudo.log"
    if run_install "${tmp}/writable" PATH="${tmp}/sudoshim:${PATH}" && [[ ! -s "${tmp}/sudo.log" ]]; then
        pass "doesn't use sudo for a dir it can write"
    else
        fail "doesn't use sudo for a dir it can write"; cat "${tmp}/out" "${tmp}/sudo.log" >&2
    fi
    chmod u+w "${tmp}/readonly"
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

# A release with no checksums.txt can't be verified, so nothing is installed.
rm "${release}/checksums.txt"
if run_install "${tmp}/nosums"; then
    fail "refuses a release with no checksums.txt: install.sh succeeded"
elif ! grep -q "^curl: .*checksums\.txt" "${tmp}/out"; then
    fail "refuses a release with no checksums.txt: failed for another reason"; cat "${tmp}/out" >&2
elif [[ -e "${tmp}/nosums/shipyard" ]]; then
    fail "refuses a release with no checksums.txt: a binary was installed anyway"
else
    pass "refuses a release with no checksums.txt"
fi
write_sums "$good_sum" "$asset"

# Current macOS ships sha256sum too, so the shasum fallback is forced with a
# PATH that holds only what install.sh needs. The shasum wrapper logs its use.
mkdir -p "${tmp}/nosha"
for t in bash curl awk grep mktemp mkdir install mv rm uname cat; do
    ln -sf "$(command -v "$t")" "${tmp}/nosha/${t}"
done
printf '#!/bin/sh\necho used >> %s/shasum.log\nexec %s "$@"\n' "$tmp" "$(command -v shasum)" > "${tmp}/nosha/shasum"
chmod +x "${tmp}/nosha/shasum"
if run_install "${tmp}/shasum" PATH="${tmp}/nosha" \
    && [[ -s "${tmp}/shasum.log" && -x "${tmp}/shasum/shipyard" ]]; then
    pass "verifies with shasum when sha256sum is missing"
else
    fail "verifies with shasum when sha256sum is missing"; cat "${tmp}/out" >&2
fi

rm "${tmp}/nosha/shasum"
expect_refusal "refuses when no hash tool is installed" "can't be verified" "${tmp}/nohash" PATH="${tmp}/nosha"

# An unbuilt architecture is refused up front, not with a download 404.
mkdir -p "${tmp}/shim"
real_uname="$(command -v uname)"
# shellcheck disable=SC2016 # $1 and $@ belong to the shim
printf '#!/bin/sh\nif [ "$1" = -m ]; then echo i686; else exec %s "$@"; fi\n' "$real_uname" > "${tmp}/shim/uname"
chmod +x "${tmp}/shim/uname"
expect_refusal "refuses an unsupported platform" "Platform not supported" "${tmp}/i686" PATH="${tmp}/shim:${PATH}"

# Git Bash exports OS=Windows_NT, which mustn't stand in for a detected OS.
mkdir -p "${tmp}/winshim"
# shellcheck disable=SC2016 # $1 and $@ belong to the shim
printf '#!/bin/sh\nif [ "$1" = -s ]; then echo MINGW64_NT-10.0; else exec %s "$@"; fi\n' "$real_uname" > "${tmp}/winshim/uname"
chmod +x "${tmp}/winshim/uname"
expect_refusal "ignores an inherited OS variable" "Platform not supported" "${tmp}/gitbash" \
    PATH="${tmp}/winshim:${PATH}" OS=Windows_NT
expect_refusal "ignores an inherited ARCH variable" "Platform not supported" "${tmp}/archenv" \
    PATH="${tmp}/shim:${PATH}" ARCH=amd64

if (( failures > 0 )); then
    echo "${failures} install.sh test(s) failed" >&2
    exit 1
fi
echo "All install.sh tests passed"
