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
    # A quoted "~/bin" reaches us unexpanded, so match the literal tilde.
    # shellcheck disable=SC2088
    case "$install_dir" in
        "~")    install_dir="$HOME" ;;
        "~/"*)  install_dir="${HOME}/${install_dir#"~/"}" ;;
    esac
    target="${install_dir}/shipyard"
    # install(1) would put the binary inside a directory, or replace a
    # symlink such as Homebrew's, and still report success.
    if [[ -L "$target" ]]; then
        echo "${target} is a symlink (Homebrew creates one); not replacing it." >&2
        echo "Upgrade a Homebrew install with 'brew upgrade shipyard', or remove the link and rerun." >&2
        exit 1
    fi
    if [[ -d "$target" ]]; then
        echo "${target} is a directory; not installing over it." >&2
        exit 1
    fi

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

    # Create the install dir if it's missing, and use sudo only when we can't
    # create or write it ourselves. install(1) sets the mode regardless of
    # umask and, under sudo, makes the binary root-owned, so a later
    # `sudo shipyard upgrade` can't run a binary any user process could replace.
    as_root=""
    if ! mkdir -p "$install_dir" 2>/dev/null || [[ ! -w "$install_dir" ]]; then
        as_root=sudo
        sudo mkdir -p "$install_dir"
    fi
    $as_root install -m 0755 "${workdir}/shipyard" "$target"
    case ":${PATH}:" in
        *":${install_dir}:"*) ;;
        *) echo "Note: ${install_dir} is not on your PATH." ;;
    esac
    echo "Installation Complete! Run 'shipyard --help' to get started."
}

main "$@"
