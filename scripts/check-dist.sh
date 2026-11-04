#!/usr/bin/env bash
#
# Checks a GoReleaser dist/ before it's released: every platform's binary is
# listed in checksums.txt, each has an SPDX SBOM that is listed too, and every
# SBOM file matches its listed digest. README.md and the attestation step rely
# on these names.
#
# Usage: ./scripts/check-dist.sh [dist-dir]
#
set -euo pipefail

dist="${1:-dist}"
sums="${dist}/checksums.txt"
[[ -f "$sums" ]] || { echo "No ${sums}" >&2; exit 1; }

binaries=(
    shipyard-darwin-amd64 shipyard-darwin-arm64
    shipyard-linux-amd64 shipyard-linux-arm64
    shipyard-windows-amd64.exe shipyard-windows-arm64.exe
)

listed() { awk -v name="$1" '$2 == name { found = 1 } END { exit !found }' "$sums"; }

failed=0
for binary in "${binaries[@]}"; do
    sbom="${binary}.sbom.json"
    if ! listed "$binary"; then
        echo "FAIL ${binary}: not in checksums.txt" >&2; failed=1; continue
    fi
    if ! listed "$sbom"; then
        echo "FAIL ${sbom}: not in checksums.txt" >&2; failed=1; continue
    fi
    if ! jq -e '(.spdxVersion // "") | startswith("SPDX-")' "${dist}/${sbom}" >/dev/null 2>&1; then
        echo "FAIL ${sbom}: missing or not an SPDX document" >&2; failed=1; continue
    fi
    echo "ok   ${binary} + ${sbom}"
done

# The SBOMs sit in dist/ under their release names, so their digests can be
# checked directly. (The binaries are renamed only on upload.)
if ! (cd "$dist" && grep '\.sbom\.json$' checksums.txt | sha256sum -c --quiet -); then
    echo "FAIL an SBOM doesn't match its digest in checksums.txt" >&2
    failed=1
fi

exit "$failed"
