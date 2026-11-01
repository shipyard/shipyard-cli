#!/usr/bin/env bash
#
# Tests the hosted MCP server's helm chart (helm/charts/mcp-host): a complete
# set of values renders valid manifests with the settings streaming and OAuth
# depend on, and each missing or unsafe value fails the render.
#
# Usage: ./scripts/chart-test.sh   (needs helm; kubeconform too, if installed)
#
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CHART="$ROOT/helm/charts/mcp-host"

ok=(--set image.tag=v1.0.0 --set mcp.resourceUrl=https://mcp.example.test/mcp --set mcp.clientSecret=test-secret)
failures=0

render() {
    helm template t "$CHART" "$@" 2>&1
}

pass() { printf 'ok   %s\n' "$1"; }
fail() { printf 'FAIL %s\n' "$1"; failures=$((failures + 1)); }

# expect_contains <name> <text> <helm args...>
expect_contains() {
    local name="$1" want="$2" out
    shift 2
    if out="$(render "$@")" && grep -qF -- "$want" <<< "$out"; then
        pass "$name"
    else
        fail "$name: rendered without ${want}"
        printf '%s\n' "$out" | head -20
    fi
}

# expect_refused <name> <message> <helm args...>
expect_refused() {
    local name="$1" want="$2" out
    shift 2
    if out="$(render "$@")"; then
        fail "$name: rendered, but should have been refused"
    elif grep -qF -- "$want" <<< "$out"; then
        pass "$name"
    else
        fail "$name: refused, but without \"${want}\""
        printf '%s\n' "$out" | head -5
    fi
}

helm lint --quiet "$CHART" "${ok[@]}" > /dev/null && pass "helm lint" || fail "helm lint"

if command -v kubeconform > /dev/null; then
    if render "${ok[@]}" | kubeconform -strict -summary - > /dev/null; then
        pass "manifests match the Kubernetes schemas"
    else
        fail "kubeconform"
    fi
else
    echo "skip manifests match the Kubernetes schemas: kubeconform isn't installed"
fi

expect_contains "ingress serves the resource URL's host" "- host: mcp.example.test" "${ok[@]}"
expect_contains "certificate covers that host" "- mcp.example.test" "${ok[@]}"
expect_contains "nginx doesn't buffer responses" 'proxy-buffering: "off"' "${ok[@]}"
expect_contains "nginx waits past the server's WriteTimeout" 'proxy-read-timeout: "120"' "${ok[@]}"
expect_contains "nginx limits requests per client" 'limit-rps: "10"' "${ok[@]}"
expect_contains "nginx limits connections per client" 'limit-connections: "20"' "${ok[@]}"
expect_contains "server gets the resource URL" 'value: "https://mcp.example.test/mcp"' "${ok[@]}"
expect_contains "secret comes from a Secret, not a value" "secretKeyRef:" "${ok[@]}"
expect_contains "runs read-only" "readOnlyRootFilesystem: true" "${ok[@]}"
expect_contains "verbose is off by default" '"--listen", ":8080"]' "${ok[@]}"
expect_contains "an existing Secret is used by name" "name: mcp-secrets" \
    --set image.tag=v1.0.0 --set mcp.resourceUrl=https://mcp.example.test/mcp \
    --set mcp.existingSecret=mcp-secrets --set mcp.existingSecretKey=client-secret
expect_contains "an existing Secret's key is used" "key: client-secret" \
    --set image.tag=v1.0.0 --set mcp.resourceUrl=https://mcp.example.test/mcp \
    --set mcp.existingSecret=mcp-secrets --set mcp.existingSecretKey=client-secret

# expect_absent <name> <text> <helm args...>: renders, and without <text>.
expect_absent() {
    local name="$1" unwanted="$2" out
    shift 2
    if ! out="$(render "$@")"; then
        fail "$name: didn't render"
        printf '%s\n' "$out" | head -5
    elif grep -qF -- "$unwanted" <<< "$out"; then
        fail "$name: rendered with ${unwanted}"
    else
        pass "$name"
    fi
}

# A PDB with one replica would block every drain.
expect_absent "no PDB with one replica" "kind: PodDisruptionBudget" "${ok[@]}" --set replicaCount=1
expect_absent "no chart Secret with an existing one" "kind: Secret" \
    --set image.tag=v1.0.0 --set mcp.resourceUrl=https://mcp.example.test/mcp \
    --set mcp.existingSecret=mcp-secrets

expect_refused "no image tag" "image.tag is required" \
    --set mcp.resourceUrl=https://mcp.example.test/mcp --set mcp.clientSecret=x
expect_refused "no resource URL" "mcp.resourceUrl is required" \
    --set image.tag=v1.0.0 --set mcp.clientSecret=x
expect_refused "http resource URL" "must be https" \
    --set image.tag=v1.0.0 --set mcp.resourceUrl=http://mcp.example.test/mcp --set mcp.clientSecret=x
expect_refused "resource URL with a port" "without a port" \
    --set image.tag=v1.0.0 --set mcp.resourceUrl=https://mcp.example.test:8443/mcp --set mcp.clientSecret=x
expect_refused "resource URL with a query" "query or fragment" \
    --set image.tag=v1.0.0 --set 'mcp.resourceUrl=https://mcp.example.test/mcp?x=1' --set mcp.clientSecret=x
expect_refused "no client secret" "mcp.clientSecret or mcp.existingSecret is required" \
    --set image.tag=v1.0.0 --set mcp.resourceUrl=https://mcp.example.test/mcp
expect_refused "both secret sources" "not both" "${ok[@]}" --set mcp.existingSecret=mcp-secrets
expect_refused "existing Secret without a key" "mcp.existingSecretKey is required" \
    --set image.tag=v1.0.0 --set mcp.resourceUrl=https://mcp.example.test/mcp \
    --set mcp.existingSecret=mcp-secrets --set mcp.existingSecretKey=
# The server sends its client secret and users' tokens to the site URL.
expect_refused "http site URL" "mcp.siteUrl must be https" "${ok[@]}" --set mcp.siteUrl=http://shipyard.example.test

# The install recipe, run against a fake helm that records what it was given:
# the secret reaches helm as given, and a secret that isn't one token is
# refused before helm runs.
fake_bin="$(mktemp -d)"
trap 'rm -rf "$fake_bin"' EXIT
cat > "$fake_bin/helm" <<EOF
#!/bin/sh
printf '%s\n' "\$*" > "$fake_bin/argv"
cat > "$fake_bin/stdin"
EOF
chmod +x "$fake_bin/helm"

install_qa() {
    rm -f "$fake_bin/argv" "$fake_bin/stdin"
    (cd "$ROOT" && SHIPYARD_MCP_CLIENT_SECRET="$1" PATH="$fake_bin:$PATH" \
        make -s mcp-host.install.qa MCP_HOST_TAG=v1.0.0 MCP_HOST_IMAGE=registry.example.test/mcp-host) 2>&1
}

if install_qa "test-secret" > /dev/null && grep -qF 'clientSecret: "test-secret"' "$fake_bin/stdin"; then
    pass "install sends the secret to helm"
else
    fail "install sends the secret to helm"
fi
if grep -qF 'image.repository=registry.example.test/mcp-host' "$fake_bin/argv" 2> /dev/null; then
    pass "install deploys the image it pushed"
else
    fail "install deploys the image it pushed: no image.repository=\$(MCP_HOST_IMAGE)"
fi

# expect_install_refused <name> <secret>
expect_install_refused() {
    local name="$1" out
    if out="$(install_qa "$2")"; then
        fail "$name: the install ran helm"
    elif [[ -e "$fake_bin/argv" ]]; then
        fail "$name: refused, but only after running helm"
    elif grep -qF "must be one token" <<< "$out"; then
        pass "$name"
    else
        fail "$name: refused, but without \"must be one token\""
        printf '%s\n' "$out" | head -5
    fi
}

expect_install_refused "install refuses a secret with a quote" 'abc"def'
expect_install_refused "install refuses a secret over two lines" "$(printf 'first\nsecond')"

if [[ "$failures" -gt 0 ]]; then
    printf '%d chart check(s) failed\n' "$failures"
    exit 1
fi
echo "All chart checks passed."
