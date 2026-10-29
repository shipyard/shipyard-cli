#!/usr/bin/env bash
#
# Checks that the release build is reproducible: rebuilds the snapshot from a
# fresh clone at a different path and compares each binary's SHA-256 with the
# one in dist/checksums.txt. A path baked into the binary (no -trimpath) or a
# build-time timestamp fails here. SBOMs are skipped: they record when they
# were generated.
#
# Run it after `goreleaser release --snapshot --clean`, from the repo root.
#
# Usage: ./scripts/check-reproducible.sh [dist-dir]
#
set -euo pipefail

dist="${1:-dist}"
sums="${dist}/checksums.txt"
[[ -f "$sums" ]] || { echo "No ${sums}" >&2; exit 1; }

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
clone="${work}/a/different/path/shipyard-cli"

git clone --quiet --no-hardlinks . "$clone"
git -C "$clone" checkout --quiet --detach "$(git rev-parse HEAD)"
(cd "$clone" && goreleaser release --snapshot --clean --skip=sbom > "${work}/build.log" 2>&1) || {
    cat "${work}/build.log" >&2; echo "Rebuild failed" >&2; exit 1
}

failed=0
checked=0
while read -r sum name; do
    [[ "$name" == *.sbom.json ]] && continue
    rebuilt="$(awk -v name="$name" '$2 == name { print $1 }' "${clone}/dist/checksums.txt")"
    if [[ "$rebuilt" != "$sum" ]]; then
        echo "FAIL ${name}: ${sum}, rebuilt ${rebuilt:-missing}" >&2
        failed=1
    else
        echo "ok   ${name}"
    fi
    checked=$((checked + 1))
done < "$sums"

[[ "$checked" -gt 0 ]] || { echo "No binaries in ${sums}" >&2; exit 1; }
exit "$failed"
