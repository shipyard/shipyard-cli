#!/usr/bin/env bash
set -ex

RELEASES_URL="https://github.com/shipyard/shipyard-cli/releases"

last_version() {
    curl --silent --show-error --location --fail \
        --output /dev/null \
        --write-out "%{url_effective}" "$RELEASES_URL/latest" |
    grep -Eo '[0-9]+\.[0-9]+\.[0-9]+$'
}

main() {
    default_dir=/usr/local/bin

    case $(uname -m) in
        i386 | i686)        ARCH="386" ;;
        x86_64)             ARCH="amd64" ;;
        arm64 | aarch64)    ARCH="arm64" ;;
    esac

    [[ -z "$ARCH" ]] && { echo "Platform not supported."; exit 1; }

    VERSION="$(last_version)"
    echo "Downloading the latest binary..."
    URL="${RELEASES_URL}/download/v${VERSION}/shipyard-$(uname -s)-${ARCH}"

    curl --silent --show-error --location --fail --output "${PWD}/shipyard" "$URL"
    chmod +x shipyard
    sudo mv shipyard "$default_dir"
    echo "Installation Complete! Run 'shipyard --help' to get started."
}

main "$@"
