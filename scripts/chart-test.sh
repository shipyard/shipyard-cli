#!/usr/bin/env bash
#
# Tests the hosted MCP server's helm chart (helm/charts/mcp-host): a complete
# set of values renders valid manifests with the settings streaming and OAuth
# depend on, and each missing or unsafe value fails the render.
#
# Usage: ./scripts/chart-test.sh   (needs helm; kubeconform too, if installed)
#        REQUIRE_KUBECONFORM=1 fails instead of skipping without kubeconform (CI).
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
elif [[ -n "${REQUIRE_KUBECONFORM:-}" ]]; then
    fail "manifests match the Kubernetes schemas: kubeconform isn't installed"
else
    echo "skip manifests match the Kubernetes schemas: kubeconform isn't installed"
fi

expect_contains "ingress serves the resource URL's host" "- host: mcp.example.test" "${ok[@]}"
expect_contains "certificate covers that host" "- mcp.example.test" "${ok[@]}"
expect_contains "nginx doesn't buffer responses" 'proxy-buffering: "off"' "${ok[@]}"
expect_contains "nginx waits past the server's WriteTimeout" 'proxy-read-timeout: "120"' "${ok[@]}"
expect_contains "nginx limits requests per client" 'limit-rps: "50"' "${ok[@]}"
expect_contains "nginx limits connections per client" 'limit-connections: "200"' "${ok[@]}"
expect_contains "only the ingress controller reaches the pods" "kubernetes.io/metadata.name: ingress-nginx" "${ok[@]}"
expect_contains "the ingress controller's namespace is a value" "kubernetes.io/metadata.name: ingress-system" \
    "${ok[@]}" --set networkPolicy.ingressNamespace=ingress-system
expect_contains "server gets the resource URL" 'value: "https://mcp.example.test/mcp"' "${ok[@]}"
expect_contains "secret comes from a Secret, not a value" "secretKeyRef:" "${ok[@]}"
expect_contains "runs read-only" "readOnlyRootFilesystem: true" "${ok[@]}"
expect_contains "verbose is off by default" '"--listen", ":8080"]' "${ok[@]}"
expect_contains "ready only once core accepts the secret" "path: /readyz" "${ok[@]}"
expect_contains "alive while the process answers" "path: /healthz" "${ok[@]}"
expect_contains "client addresses come from the ingress" "name: SHIPYARD_MCP_BEHIND_PROXY" "${ok[@]}"
expect_contains "the garbage collector knows the memory limit" "name: GOMEMLIMIT" "${ok[@]}"
digest="sha256:$(printf '0123456789abcdef%.0s' 1 2 3 4)"
expect_contains "the image runs by digest when given one" \
    "image: \"us-east4-docker.pkg.dev/shipyard-infra/shipyardbuild/mcp-host:v1.0.0@${digest}\"" \
    "${ok[@]}" --set "image.digest=${digest}"
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
expect_absent "no X-Forwarded-For without the ingress" "SHIPYARD_MCP_BEHIND_PROXY" "${ok[@]}" --set ingress.enabled=false
expect_absent "no NetworkPolicy when it's turned off" "kind: NetworkPolicy" "${ok[@]}" --set networkPolicy.enabled=false
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
expect_refused "resource URL with a fragment" "query or fragment" \
    --set image.tag=v1.0.0 --set 'mcp.resourceUrl=https://mcp.example.test/mcp#f' --set mcp.clientSecret=x
expect_refused "resource URL without a host" "needs a host" \
    --set image.tag=v1.0.0 --set 'mcp.resourceUrl=https:///mcp' --set mcp.clientSecret=x
# The server sends its client secret and users' tokens to the site URL.
expect_refused "http site URL" "mcp.siteUrl must be https" "${ok[@]}" --set mcp.siteUrl=http://shipyard.example.test
expect_refused "site URL without a host" "mcp.siteUrl needs a host" "${ok[@]}" --set 'mcp.siteUrl=https:///x'
expect_refused "no site URL" "mcp.siteUrl is required" "${ok[@]}" --set mcp.siteUrl=
expect_refused "a digest that isn't one" "image.digest must be sha256" "${ok[@]}" --set image.digest=latest
expect_contains "verbose adds the flag" '"--verbose"' "${ok[@]}" --set mcp.verbose=true

# The install recipes, run against a fake helm that records what it was given:
# each environment's settings and the secret reach helm, and bad input is
# refused before helm runs.
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
fake_bin="$work/bin"
mkdir "$fake_bin"
cat > "$fake_bin/helm" <<EOF
#!/bin/sh
printf '%s\n' "\$*" > "$fake_bin/argv"
cat > "$fake_bin/stdin"
EOF
chmod +x "$fake_bin/helm"
# A fake docker that answers the digest lookup, or fails when docker_fails exists.
cat > "$fake_bin/docker" <<EOF
#!/bin/sh
[ -e "$fake_bin/docker_fails" ] && { echo "fake docker: not found" >&2; exit 1; }
printf '%s\n' "\$*" > "$fake_bin/docker_argv"
echo "$digest"
EOF
chmod +x "$fake_bin/docker"

# install <target> <secret> [make args...]
install() {
    local target="$1" secret="$2"
    shift 2
    rm -f "$fake_bin/argv" "$fake_bin/stdin" "$fake_bin/docker_argv"
    (cd "$ROOT" && SHIPYARD_MCP_CLIENT_SECRET="$secret" PATH="$fake_bin:$PATH" \
        make -s "$target" MCP_HOST_IMAGE=registry.example.test/mcp-host KUBE_CONTEXT=test-context "$@") 2>&1
}

# expect_install_args <name> <target> <args helm must get...>
expect_install_args() {
    local name="$1" target="$2" want
    shift 2
    if ! install "$target" "test-secret" MCP_HOST_TAG=v1.0.0 > /dev/null; then
        fail "$name: the install failed"
        return
    fi
    for want in "$@"; do
        if ! grep -qF -- "$want" "$fake_bin/argv"; then
            fail "$name: helm didn't get ${want}"
            return
        fi
    done
    if grep -qF 'clientSecret: "test-secret"' "$fake_bin/stdin"; then
        pass "$name"
    else
        fail "$name: the secret didn't reach helm"
    fi
}

expect_install_args "QA install" mcp-host.install.qa "--kube-context test-context " "--namespace qa-mcp-host " \
    "image.repository=registry.example.test/mcp-host " "image.tag=v1.0.0 " "image.digest=${digest} " \
    "mcp.resourceUrl=https://mcp.qa.shipyard.build/mcp " "mcp.siteUrl=https://qa.shipyard.build "
expect_install_args "production install" mcp-host.install.production "--kube-context test-context " "--namespace mcp-host " \
    "image.repository=registry.example.test/mcp-host " "image.tag=v1.0.0 " "image.digest=${digest} " \
    "mcp.resourceUrl=https://mcp.shipyard.build/mcp " "mcp.siteUrl=https://shipyard.build "

# expect_install_refused <name> <message> <secret> [make args...]
expect_install_refused() {
    local name="$1" want="$2" out
    shift 2
    if out="$(install mcp-host.install.qa "$@")"; then
        fail "$name: the install ran helm"
    elif [[ -e "$fake_bin/argv" ]]; then
        fail "$name: refused, but only after running helm"
    elif grep -qF -- "$want" <<< "$out"; then
        pass "$name"
    else
        fail "$name: refused, but without \"${want}\""
        printf '%s\n' "$out" | head -5
    fi
}

expect_install_refused "install refuses no kube context" "KUBE_CONTEXT is required" "test-secret" MCP_HOST_TAG=v1.0.0 KUBE_CONTEXT=
expect_install_refused "install refuses no tag" "MCP_HOST_TAG is required" "test-secret"
expect_install_refused "install refuses no secret" "SHIPYARD_MCP_CLIENT_SECRET is required" "" MCP_HOST_TAG=v1.0.0
expect_install_refused "install refuses a secret with a quote" "must be one token" 'abc"def' MCP_HOST_TAG=v1.0.0
expect_install_refused "install refuses a secret over two lines" "must be one token" \
    "$(printf 'first\nsecond')" MCP_HOST_TAG=v1.0.0
if install mcp-host.install.qa test-secret MCP_HOST_TAG=v1.0.0 > /dev/null \
    && grep -qF "registry.example.test/mcp-host:v1.0.0 " "$fake_bin/docker_argv"; then
    pass "install looks up the tag it deploys"
else
    fail "install didn't look up registry.example.test/mcp-host:v1.0.0"
fi
touch "$fake_bin/docker_fails"
expect_install_refused "install refuses a tag it can't look up" "fake docker: not found" "test-secret" MCP_HOST_TAG=v1.0.0
rm "$fake_bin/docker_fails"

# The image is built only at a release tag that's on origin, on a clean tree:
# checked in a clone of a scratch origin, against this checkout's Makefile.
# No tags: the commit under test may already carry a release tag.
git clone -q --bare --no-tags "$ROOT" "$work/origin.git"
git clone -q "$work/origin.git" "$work/repo"
# expect_tag_check <name> <passes|refused> [message]
expect_tag_check() {
    local name="$1" want="$2" message="${3:-}" out
    if out="$(make -s -C "$work/repo" -f "$ROOT/Makefile" check.mcp-host-tag 2>&1)"; then
        [[ "$want" == passes ]] && pass "$name" || fail "$name: the check passed"
    elif [[ "$want" == refused ]] && grep -qF -- "$message" <<< "$out"; then
        pass "$name"
    else
        fail "$name: ${out}"
    fi
}
tag="v0.0.0-chart-test"
expect_tag_check "image build refuses an untagged commit" refused "from a release tag"
git -C "$work/repo" -c tag.gpgSign=false tag "$tag"
expect_tag_check "image build refuses a tag that isn't on origin" refused "isn't on origin"
git -C "$work/origin.git" -c tag.gpgSign=false tag "$tag" "$(git -C "$work/repo" rev-parse HEAD)"
touch "$work/repo/untracked.go"
expect_tag_check "image build refuses an untracked file" refused "untracked files"
rm "$work/repo/untracked.go"
expect_tag_check "image build at a clean release tag on origin" passes

if [[ "$failures" -gt 0 ]]; then
    printf '%d chart check(s) failed\n' "$failures"
    exit 1
fi
echo "All chart checks passed."
