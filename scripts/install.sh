#!/usr/bin/env bash
set -ex

# Both overrides exist for scripts/install-test.sh, which serves a fake
# release from a file:// URL; SHIPYARD_VERSION also pins a version (1.2.3).
RELEASES_URL="${SHIPYARD_RELEASES_URL:-https://github.com/shipyard/shipyard-cli/releases}"

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
    # The final mv would put the binary inside a directory, or replace a
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

    # Only the platforms .goreleaser.yaml builds; anything else would fail
    # later with a bare 404. Cleared first: Git Bash exports OS=Windows_NT.
    OS="" ARCH=""
    case $(uname -s) in
        Linux)  OS="linux" ;;
        Darwin) OS="darwin" ;;
    esac
    case $(uname -m) in
        x86_64)             ARCH="amd64" ;;
        arm64 | aarch64)    ARCH="arm64" ;;
    esac
    if [[ -z "${OS:-}" || -z "${ARCH:-}" ]]; then
        echo "Platform not supported: $(uname -s) $(uname -m). Releases are built for Linux and macOS on amd64 and arm64." >&2
        exit 1
    fi

    VERSION="${SHIPYARD_VERSION:-$(last_version)}"
    VERSION="${VERSION#v}"
    ASSET="shipyard-${OS}-${ARCH}"
    workdir="$(mktemp -d)"
    trap 'rm -rf "$workdir"' EXIT

    echo "Downloading shipyard ${VERSION} (${ASSET})..."
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
    # Stage beside the target and rename over it: install(1) writes onto the
    # final path, so an interrupted copy would leave a broken binary. The
    # staged copy is removed if we exit before the rename.
    staged="${install_dir}/.shipyard.new.$$"
    trap 'rm -rf "$workdir"; if [[ -e "$staged" ]]; then $as_root rm -f "$staged"; fi' EXIT
    $as_root install -m 0755 "${workdir}/shipyard" "$staged"
    $as_root mv -f "$staged" "$target"
    case ":${PATH}:" in
        *":${install_dir}:"*) ;;
        *) echo "Note: ${install_dir} is not on your PATH." ;;
    esac
    echo "Installation Complete! Run 'shipyard --help' to get started."
}

main "$@"
