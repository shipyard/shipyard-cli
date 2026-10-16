#!/usr/bin/env bash
#
# Downloads a published release's Linux amd64 binary and its SBOM and checks
# both with the same `gh attestation verify` command README.md documents. Run by
# the release workflow after attesting; also usable by hand.
#
# Usage: ./scripts/verify-attestation.sh <tag> [repo]
#
set -euo pipefail

tag="${1:?usage: $0 <tag> [repo]}"
repo="${2:-shipyard/shipyard-cli}"

dir="$(mktemp -d)"
trap 'rm -rf "$dir"' EXIT

gh release download "$tag" -R "$repo" -D "$dir" \
    -p shipyard-linux-amd64 -p shipyard-linux-amd64.sbom.json

for file in "$dir"/*; do
    gh attestation verify "$file" -R "$repo" \
        --signer-workflow "${repo}/.github/workflows/release.yaml"
done
