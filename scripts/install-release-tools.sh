#!/usr/bin/env bash
#
# Installs the GoReleaser and syft versions pinned in release.yaml's env on a
# Linux x86_64 GitHub runner, refusing any download whose SHA-256 doesn't match
# the pinned one. The release job signs what these tools build, so a moved tag
# or a replaced release asset upstream must not change what runs.
#
# Pinned digests come from each release's signed checksums file (GoReleaser:
# checksums.txt + .sigstore.json; syft: <name>_checksums.txt + .sig/.pem),
# checked with cosign verify-blob when the version was bumped.
#
# Usage (in a workflow step): ./scripts/install-release-tools.sh
#
set -euo pipefail

: "${GORELEASER_VERSION:?}" "${GORELEASER_SHA256:?}" "${SYFT_VERSION:?}" "${SYFT_SHA256:?}"
: "${RUNNER_TEMP:?}" "${GITHUB_PATH:?}"

if [[ "$(uname -s)" != Linux || "$(uname -m)" != x86_64 ]]; then
    echo "Pinned digests are for Linux x86_64 tarballs; this is $(uname -s) $(uname -m)." >&2
    exit 1
fi

bin="${RUNNER_TEMP}/release-tools"
mkdir -p "$bin"

# fetch <url> <sha256> <file>: downloads url to file and checks its digest.
fetch() {
    curl --silent --show-error --location --fail --output "$3" "$1"
    echo "$2  $3" | sha256sum -c -
}

fetch "https://github.com/goreleaser/goreleaser/releases/download/v${GORELEASER_VERSION}/goreleaser_Linux_x86_64.tar.gz" \
    "$GORELEASER_SHA256" "${RUNNER_TEMP}/goreleaser.tar.gz"
tar -xzf "${RUNNER_TEMP}/goreleaser.tar.gz" -C "$bin" goreleaser

fetch "https://github.com/anchore/syft/releases/download/v${SYFT_VERSION}/syft_${SYFT_VERSION}_linux_amd64.tar.gz" \
    "$SYFT_SHA256" "${RUNNER_TEMP}/syft.tar.gz"
tar -xzf "${RUNNER_TEMP}/syft.tar.gz" -C "$bin" syft

echo "$bin" >> "$GITHUB_PATH"
"${bin}/goreleaser" --version | grep -m1 GitVersion
"${bin}/syft" version | grep -m1 '^Version'
