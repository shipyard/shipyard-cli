#!/usr/bin/env bash
#
# Tests vuln-scan.sh's stripped-binary guard. Release binaries are built with
# -trimpath, so Go doesn't record -ldflags in them and the guard reads the
# binary's symbol table instead. Builds a tiny program for each executable
# format (ELF, PE, Mach-O) with and without -s: vuln-scan.sh must scan the
# stripped builds and refuse the others. govulncheck is stubbed to report
# every binary clean, so this tests the guard, not the vulnerability database.
#
# Needs Go. Usage: ./scripts/vuln-scan-test.sh
#
set -euo pipefail

SCAN="$(cd "$(dirname "$0")" && pwd)/vuln-scan.sh"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

src="${tmp}/src"
mkdir -p "$src" "${tmp}/gotmp"
export GOTMPDIR="${tmp}/gotmp"
printf 'module probe\n\ngo 1.21\n' > "${src}/go.mod"
printf 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("hi") }\n' > "${src}/main.go"

mkdir -p "${tmp}/bin"
cat > "${tmp}/bin/govulncheck" <<'EOF'
#!/usr/bin/env bash
[[ "${1:-}" == -version ]] && { echo "govulncheck stub"; exit 0; }
exit 0
EOF
chmod +x "${tmp}/bin/govulncheck"
export PATH="${tmp}/bin:${PATH}"
export VULN_IGNORE_FILE="${tmp}/no-ignore-file"

failures=0
pass() { echo "ok   $1"; }
fail() { echo "FAIL $1" >&2; failures=$((failures + 1)); }

# build <goos/goarch> <ldflags> <out>: a -trimpath build, as the release does.
build() {
    (cd "$src" && CGO_ENABLED=0 GOOS="${1%/*}" GOARCH="${1#*/}" GOFLAGS='' \
        go build -trimpath -buildvcs=false -ldflags="$2" -o "$3" .)
}

for target in linux/amd64 windows/amd64 darwin/arm64; do
    name="${target//\//-}"
    build "$target" "-s -w" "${tmp}/${name}-stripped"
    build "$target" "-w" "${tmp}/${name}-unstripped"

    if "$SCAN" "${tmp}/${name}-stripped" > "${tmp}/out" 2>&1; then
        pass "${target}: scans a stripped binary"
    else
        fail "${target}: scans a stripped binary"; cat "${tmp}/out" >&2
    fi

    if "$SCAN" "${tmp}/${name}-unstripped" > "${tmp}/out" 2>&1; then
        fail "${target}: refuses an unstripped binary: vuln-scan.sh passed"
    elif ! grep -qF "was not built with -ldflags -s" "${tmp}/out"; then
        fail "${target}: refuses an unstripped binary: wrong error"; cat "${tmp}/out" >&2
    else
        pass "${target}: refuses an unstripped binary"
    fi
done

# Not an executable at all: go tool nm fails, and the guard must not treat
# that as stripped.
echo "not a binary" > "${tmp}/text-file"
if "$SCAN" "${tmp}/text-file" > "${tmp}/out" 2>&1; then
    fail "refuses a file go tool nm can't read: vuln-scan.sh passed"
else
    pass "refuses a file go tool nm can't read"
fi

[[ "$failures" -eq 0 ]] || { echo "${failures} check(s) failed" >&2; exit 1; }
echo "All vuln-scan.sh guard tests passed"
