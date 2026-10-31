#!/usr/bin/env bash
#
# Tests the hosted MCP server's helm chart (helm/charts/mcp-host): a complete
# set of values renders valid manifests with the settings streaming and OAuth
# depend on, and each missing or unsafe value fails the render.
#
# Usage: ./scripts/chart-test.sh   (needs helm; kubeconform too, if installed)
#
set -euo pipefail

CHART="$(cd "$(dirname "$0")/.." && pwd)/helm/charts/mcp-host"

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
fi

expect_contains "ingress serves the resource URL's host" "- host: mcp.example.test" "${ok[@]}"
expect_contains "certificate covers that host" "- mcp.example.test" "${ok[@]}"
expect_contains "nginx doesn't buffer responses" 'proxy-buffering: "off"' "${ok[@]}"
expect_contains "nginx waits past the server's WriteTimeout" 'proxy-read-timeout: "120"' "${ok[@]}"
expect_contains "server gets the resource URL" 'value: "https://mcp.example.test/mcp"' "${ok[@]}"
expect_contains "secret comes from a Secret, not a value" "secretKeyRef:" "${ok[@]}"
expect_contains "runs read-only" "readOnlyRootFilesystem: true" "${ok[@]}"
expect_contains "verbose is off by default" '"--listen", ":8080"]' "${ok[@]}"
expect_contains "an existing Secret is used by name" "name: mcp-secrets" \
    --set image.tag=v1.0.0 --set mcp.resourceUrl=https://mcp.example.test/mcp \
    --set mcp.existingSecret=mcp-secrets --set mcp.existingSecretKey=client-secret
if render "${ok[@]}" --set replicaCount=1 | grep -q "kind: PodDisruptionBudget"; then
    fail "no PDB with one replica: a PDB would block every drain"
else
    pass "no PDB with one replica"
fi
if render --set image.tag=v1.0.0 --set mcp.resourceUrl=https://mcp.example.test/mcp \
    --set mcp.existingSecret=mcp-secrets | grep -q "kind: Secret"; then
    fail "an existing Secret isn't recreated by the chart"
else
    pass "no chart Secret with an existing one"
fi

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

if [[ "$failures" -gt 0 ]]; then
    printf '%d chart check(s) failed\n' "$failures"
    exit 1
fi
echo "All chart checks passed."
