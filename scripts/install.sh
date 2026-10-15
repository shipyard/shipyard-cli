#!/usr/bin/env bash
set -ex

RELEASES_URL="https://github.com/shipyard/shipyard-cli/releases"

last_version() {
    curl --silent --show-error --location --fail \
        --output /dev/null \
        --write-out "%{url_effective}" "$RELEASES_URL/latest" |
    grep -Eo '[0-9]+\.[0-9]+\.[0-9]+$'
}

sha256_of() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{print $1}'
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$1" | awk '{print $1}'
    else
        echo "Neither sha256sum nor shasum is installed, so the download can't be verified." >&2
        exit 1
    fi
}

# verify_download refuses a binary whose SHA-256 isn't the one the release's
# checksums.txt lists for its asset name.
verify_download() {
    local file=$1 asset=$2 checksums=$3 want got
    want=$(awk -v asset="$asset" '$2 == asset {print $1}' "$checksums")
    if [[ -z "$want" ]]; then
        echo "checksums.txt has no entry for $asset; not installing." >&2
        exit 1
    fi
    got=$(sha256_of "$file")
    if [[ "$got" != "$want" ]]; then
        echo "Checksum mismatch for $asset (expected $want, got $got); not installing." >&2
        exit 1
    fi
}

main() {
    install_dir="${SHIPYARD_INSTALL_DIR:-/usr/local/bin}"

    case $(uname -m) in
        i386 | i686)        ARCH="386" ;;
        x86_64)             ARCH="amd64" ;;
        arm64 | aarch64)    ARCH="arm64" ;;
    esac

    [[ -z "$ARCH" ]] && { echo "Platform not supported."; exit 1; }

    VERSION="$(last_version)"
    ASSET="shipyard-$(uname -s | tr '[:upper:]' '[:lower:]')-${ARCH}"
    workdir="$(mktemp -d)"
    trap 'rm -rf "$workdir"' EXIT

    echo "Downloading the latest binary..."
    curl --silent --show-error --location --fail --output "${workdir}/shipyard" \
        "${RELEASES_URL}/download/v${VERSION}/${ASSET}"
    curl --silent --show-error --location --fail --output "${workdir}/checksums.txt" \
        "${RELEASES_URL}/download/v${VERSION}/checksums.txt"
    verify_download "${workdir}/shipyard" "$ASSET" "${workdir}/checksums.txt"

    chmod +x "${workdir}/shipyard"
    if [[ -w "$install_dir" ]]; then
        mv "${workdir}/shipyard" "$install_dir"
    else
        sudo mv "${workdir}/shipyard" "$install_dir"
    fi
    echo "Installation Complete! Run 'shipyard --help' to get started."
}

main "$@"
