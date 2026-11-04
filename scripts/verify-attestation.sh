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

# gh release download succeeds when any one pattern matches, so check that
# each file arrived rather than verifying whatever did.
names=(shipyard-linux-amd64 shipyard-linux-amd64.sbom.json)
for name in "${names[@]}"; do
    if [[ ! -f "${dir}/${name}" ]]; then
        echo "Release ${tag} has no ${name}." >&2
        exit 1
    fi
done
for name in "${names[@]}"; do
    gh attestation verify "${dir}/${name}" -R "$repo" \
        --signer-workflow "${repo}/.github/workflows/release.yaml" \
        --source-ref "refs/tags/${tag}"
done
