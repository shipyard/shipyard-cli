#!/bin/bash
set -e

RELEASES_URL="https://github.com/shipyardbuild/shipyard-cli/releases"

last_version() {
    curl --silent --location --fail \
    --output /dev/null --write-out %{url_effective} ${RELEASES_URL}/latest |
    grep -Eo '[0-9]+\.[0-9]+\.[0-9]+$'
}

main() {
    default_dir=/usr/local/bin

    case $(uname -m) in
        i386 | i686)    ARCH="386" ;;
        x86_64)         ARCH="amd64" ;;
        arm64)          ARCH="arm64" ;;
    esac

    VERSION="$(last_version)"
    URL="${RELEASES_URL}/download/v${VERSION}/shipyard-$(uname -s)-${ARCH}"
    
    curl --silent -L --output "${default_dir}/shipyard" --fail "$URL"
    chmod +x ${default_dir}/shipyard
}

main "$@"