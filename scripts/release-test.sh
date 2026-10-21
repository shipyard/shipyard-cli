#!/usr/bin/env bash
#
# Tests scripts/release-state.sh, which decides what release.sh does while it
# watches a release and after one fails. Only not-found may lead to the
# delete-tag prompt, so a gh error must never read as not-found, a run that's
# still going must never read as failed, and an old run must never stand in
# for the one a push started.
#
# Usage: ./scripts/release-test.sh
#
set -euo pipefail

# shellcheck source=scripts/release-state.sh
source "$(cd "$(dirname "$0")" && pwd)/release-state.sh"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# A gh for the calls release-state.sh makes. Each endpoint answers from its
# GH_<ENDPOINT> variable: "fail:<message>" fails with that message, "raw:<text>"
# prints text as is, "pages:<array of pages>" is JSON that a --paginate call
# gets one page at a time, and anything else is JSON that the caller's --jq
# runs over, as gh would. GH_WARN, if set, goes to stderr first.
mkdir -p "${tmp}/bin"
cat > "${tmp}/bin/gh" <<'EOF'
#!/bin/sh
# The endpoint or subcommand is the first argument after $1 that isn't a flag.
target=""
for arg in "$@"; do
    case "$arg" in "$1" | -*) ;; *) target=$arg; break ;; esac
done
case "$1 $target" in
    "api repos/o/r")                  answer=$GH_REPO ;;
    "api repos/o/r/releases/tags/"*) answer=$GH_TAGS ;;
    "api repos/o/r/releases?"*)      answer=$GH_LIST ;;
    "api repos/o/r/actions/runs/"*)  answer=$GH_ARTIFACTS ;;
    "run list")                       answer=$GH_RUNS ;;
    "run view")                       answer=$GH_VIEW ;;
    *) echo "unexpected gh call: $*" >&2; exit 2 ;;
esac
if [ -n "$GH_WARN" ]; then echo "$GH_WARN" >&2; fi
case "$answer" in
    fail:*) echo "${answer#fail:}" >&2; exit 1 ;;
    raw:*)  echo "${answer#raw:}"; exit 0 ;;
esac
paginate=false query=""
while [ $# -gt 0 ]; do
    case "$1" in
        --paginate) paginate=true ;;
        --jq) query=$2; shift ;;
    esac
    shift
done
if [ -z "$query" ]; then echo "gh call without --jq" >&2; exit 2; fi
case "$answer" in
    pages:*)
        if ! $paginate; then
            printf '%s' "${answer#pages:}" | jq -c '.[0]' | jq -r "$query"; exit
        fi
        printf '%s' "${answer#pages:}" | jq -c '.[]' | while IFS= read -r page; do
            printf '%s' "$page" | jq -r "$query"
        done ;;
    *) printf '%s' "$answer" | jq -r "$query" ;;
esac
EOF
chmod +x "${tmp}/bin/gh"

failures=0
pass() { echo "ok   $1"; }
fail() { echo "FAIL $1" >&2; failures=$((failures + 1)); }

# expect <name> <want> <command...>: command, with the stand-in gh first on
# PATH, prints want. Its stderr is kept in ${tmp}/err.
expect() {
    local name=$1 want=$2 got
    shift 2
    got="$(PATH="${tmp}/bin:${PATH}" "$@" 2> "${tmp}/err")" || true
    if [[ "$got" == "$want" ]]; then
        pass "$name"
    else
        fail "${name}: got \"${got}\", want \"${want}\""
    fi
}

# pushable: can_push's verdict as yes or no.
pushable() { if can_push "$@"; then echo yes; else echo no; fi; }

not_found='fail:gh: Not Found (HTTP 404)'
unavailable='fail:gh: Service Unavailable (HTTP 503)'
published='{"tag_name":"v9.9.9","draft":false,"prerelease":false}'
drafts='[{"tag_name":"v9.9.9","draft":true},{"tag_name":"v9.9.8","draft":false}]'
others='[{"tag_name":"v9.9.8","draft":false}]'

# ── release_state ─────────────────────────────────────────────────────────────

GH_TAGS=$published expect "reads a published release" published release_state v9.9.9 o/r
GH_TAGS=$not_found GH_LIST=$drafts expect "reads a draft" draft release_state v9.9.9 o/r
GH_TAGS=$not_found GH_LIST=$drafts GH_WARN="A new release of gh is available" \
    expect "reads a draft despite a gh warning" draft release_state v9.9.9 o/r
GH_TAGS=$not_found GH_LIST=$others expect "reads a missing release" not-found release_state v9.9.9 o/r
# The drafts list has no entry here either, which is what gh release view
# reports as "release not found" when its published lookup fails.
GH_TAGS=$unavailable GH_LIST=$others \
    expect "doesn't mistake an API error for a missing release" unknown release_state v9.9.9 o/r
grep -qF "HTTP 503" "${tmp}/err" || fail "an API error's message isn't passed on"
GH_TAGS=$not_found GH_LIST=$unavailable \
    expect "doesn't mistake a failed drafts lookup for a missing release" unknown release_state v9.9.9 o/r
GH_TAGS='raw:<html>' expect "doesn't guess from unexpected output" unknown release_state v9.9.9 o/r
# More than one page of releases: a draft past the first page is still found.
GH_TAGS=$not_found GH_LIST="pages:[${others},[{\"tag_name\":\"v9.9.9\",\"draft\":true}]]" \
    expect "finds a draft past the first page" draft release_state v9.9.9 o/r
GH_TAGS=$not_found GH_LIST="pages:[${others},${others}]" \
    expect "reads a missing release across pages" not-found release_state v9.9.9 o/r

# ── can_push ──────────────────────────────────────────────────────────────────

# The drafts list shows drafts only to a token that can push, so release.sh
# checks that before it relies on a "not found".
GH_REPO='{"permissions":{"push":true}}' expect "reads a token that can push" yes pushable o/r
GH_REPO='{"permissions":{"push":false}}' expect "reads a token that can't push" no pushable o/r
GH_REPO=$unavailable expect "doesn't assume push access on a gh error" no pushable o/r

# ── find_release_run ──────────────────────────────────────────────────────────

old_run='{"databaseId":1,"headBranch":"v9.9.9","headSha":"aaa"}'
new_run='{"databaseId":2,"headBranch":"v9.9.9","headSha":"aaa"}'
other_commit='{"databaseId":3,"headBranch":"v9.9.9","headSha":"bbb"}'

# A tag deleted and pushed again on the same commit matches its old run on
# both name and commit, so only a run that wasn't there before the push counts.
GH_RUNS="[${old_run}]" expect "waits for a re-pushed tag's own run" "" \
    find_release_run v9.9.9 aaa o/r "1"
GH_RUNS="[${new_run},${old_run}]" expect "finds a re-pushed tag's own run" 2 \
    find_release_run v9.9.9 aaa o/r "1"
GH_RUNS="[${other_commit}]" expect "ignores a run of another commit" "" \
    find_release_run v9.9.9 aaa o/r ""
GH_RUNS="[${new_run},${other_commit}]" expect "finds the first push's run" 2 \
    find_release_run v9.9.9 aaa o/r ""
GH_RUNS=$unavailable expect "finds nothing on a gh error" "" find_release_run v9.9.9 aaa o/r ""

# Before the push, a failed lookup must stop release.sh, not pass as "no runs".
if GH_RUNS=$unavailable PATH="${tmp}/bin:${PATH}" tag_runs v9.9.9 aaa o/r > /dev/null 2>&1; then
    fail "listing a tag's runs fails on a gh error"
else
    pass "listing a tag's runs fails on a gh error"
fi

# ── runs_in_progress ──────────────────────────────────────────────────────────

# Right before the tag is deleted: any run of the tag that's queued or going,
# on any commit and whether or not it was listed before the push, keeps it.
done_run='{"databaseId":1,"headBranch":"v9.9.9","headSha":"aaa","status":"completed"}'
rerun='{"databaseId":1,"headBranch":"v9.9.9","headSha":"aaa","status":"queued"}'
moved='{"databaseId":4,"headBranch":"v9.9.9","headSha":"ccc","status":"in_progress"}'
other_tag='{"databaseId":5,"headBranch":"v9.9.8","headSha":"aaa","status":"in_progress"}'
GH_RUNS="[${done_run},${other_tag}]" expect "reads a tag with no run going" no runs_in_progress v9.9.9 o/r
GH_RUNS="[${rerun}]" expect "sees an earlier run that was re-run" yes runs_in_progress v9.9.9 o/r
GH_RUNS="[${moved},${done_run}]" expect "sees a run of the tag on another commit" yes runs_in_progress v9.9.9 o/r
GH_RUNS=$unavailable expect "doesn't assume no run is going on a gh error" unknown runs_in_progress v9.9.9 o/r

# ── saved_checksums ───────────────────────────────────────────────────────────

GH_ARTIFACTS='{"artifacts":[{"name":"dist","expired":false},{"name":"checksums","expired":false}]}' \
    expect "finds saved checksums" yes saved_checksums 1 o/r
GH_ARTIFACTS='{"artifacts":[{"name":"checksums","expired":true}]}' \
    expect "doesn't count expired checksums" no saved_checksums 1 o/r
GH_ARTIFACTS='{"artifacts":[{"name":"dist","expired":false}]}' \
    expect "reads a run with no saved checksums" no saved_checksums 1 o/r
GH_ARTIFACTS=$unavailable \
    expect "doesn't mistake an API error for no saved checksums" unknown saved_checksums 1 o/r

# ── run_conclusion ────────────────────────────────────────────────────────────

# gh run watch also exits non-zero when it loses its connection mid-run, even
# if the run then succeeds.
GH_VIEW='{"status":"completed","conclusion":"failure"}' expect "reads a failed run" failure run_conclusion 1 o/r
GH_VIEW='{"status":"completed","conclusion":"success"}' expect "reads a run that succeeded" success run_conclusion 1 o/r
GH_VIEW='{"status":"in_progress","conclusion":null}' expect "reads a run that's still going" "" run_conclusion 1 o/r
GH_VIEW=$unavailable expect "doesn't assume a run finished on a gh error" "" run_conclusion 1 o/r

if (( failures > 0 )); then
    echo "${failures} release-state.sh test(s) failed" >&2
    exit 1
fi
echo "All release-state.sh tests passed"
