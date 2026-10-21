#!/usr/bin/env bash
#
# What release.sh asks GitHub while it watches a release, kept here so
# release-test.sh can test each answer with a stand-in gh. Sourced by both.

# release_state <tag> <repo>: prints published, draft, not-found or unknown.
#
# GoReleaser creates a draft and publishes it after the uploads, so a draft
# means the upload failed partway. Only a definite answer from both lookups
# below is not-found: any other gh error, or output that isn't true/false, is
# unknown, and gh's message goes to stderr. stderr is kept apart from the
# answer so a warning on a successful call can't hide a draft.
#
# Not `gh release view`: it looks up the published release and the draft at
# once, and when both fail it reports "release not found" even if the
# published lookup failed with a 503 (gh 2.101.0, FetchRelease). That would
# read a live release as missing.
release_state() {
    local err out rc=0
    err=$(mktemp)
    # The tags endpoint returns only a published release; a draft is a 404.
    out=$(gh api "repos/$2/releases/tags/$1" --jq .draft 2>"$err") || rc=$?
    if (( rc == 0 )); then
        case "$out" in
            false) echo published ;;
            *)     cat "$err" >&2; echo unknown ;;
        esac
    elif grep -q "HTTP 404" "$err"; then
        # Every page, so an older draft of this tag isn't missed. The list
        # shows drafts only to a token that can push; release.sh checks that
        # with can_push first.
        rc=0
        out=$(gh api --paginate "repos/$2/releases?per_page=100" \
            --jq ".[] | select(.tag_name == \"$1\") | .draft" 2>"$err") || rc=$?
        if (( rc != 0 )); then
            cat "$err" >&2
            echo unknown
        elif [[ -z "$out" ]]; then
            echo not-found
        elif grep -qx true <<< "$out"; then
            echo draft
        else
            echo published
        fi
    else
        cat "$err" >&2
        echo unknown
    fi
    rm -f "$err"
}

# tag_runs <tag> <sha> <repo>: IDs of the release.yaml push runs for tag at
# sha, newest first. Fails if gh does.
tag_runs() {
    gh run list --repo "$3" --workflow=release.yaml --event=push --limit=20 \
        --json databaseId,headBranch,headSha \
        --jq ".[] | select(.headBranch == \"$1\" and .headSha == \"$2\") | .databaseId"
}

# find_release_run <tag> <sha> <repo> <known-ids>: prints the ID of the run
# this push started, or nothing if it hasn't appeared yet. known-ids are the
# space-separated runs tag_runs listed before the push: a tag deleted and
# pushed again on the same commit matches its old run on name and commit.
find_release_run() {
    local id
    for id in $(tag_runs "$1" "$2" "$3" 2>/dev/null); do
        if [[ " $4 " != *" $id "* ]]; then
            echo "$id"
            return 0
        fi
    done
}

# runs_in_progress <tag> <repo>: prints yes if any release.yaml push run of
# tag, on any commit, is queued or going; no if none is; unknown if gh couldn't
# say (its message goes to stderr). A run listed before the push counts too:
# it may have been re-run since.
runs_in_progress() {
    local count
    if ! count=$(gh run list --repo "$2" --workflow=release.yaml --event=push --limit=20 \
        --json headBranch,status \
        --jq "[.[] | select(.headBranch == \"$1\" and .status != \"completed\")] | length"); then
        echo unknown
        return 0
    fi
    case "$count" in
        0)             echo no ;;
        *[!0-9]* | "") echo unknown ;;
        *)             echo yes ;;
    esac
}

# saved_checksums <run-id> <repo>: prints yes if the run kept an unexpired
# checksums artifact for re-attesting, no if it didn't, or unknown if gh
# couldn't say (its message goes to stderr).
saved_checksums() {
    local count
    if ! count=$(gh api "repos/$2/actions/runs/$1/artifacts" \
        --jq '[.artifacts[] | select(.name == "checksums" and (.expired | not))] | length'); then
        echo unknown
        return 0
    fi
    case "$count" in
        0)                    echo no ;;
        *[!0-9]* | "")        echo unknown ;;
        *)                    echo yes ;;
    esac
}

# run_conclusion <run-id> <repo>: prints the conclusion of a run that has
# completed (success, failure, cancelled, ...), or nothing if it hasn't or gh
# couldn't say. gh run watch also exits non-zero when it loses its connection,
# even if the run goes on to succeed.
run_conclusion() {
    gh run view "$1" --repo "$2" --json status,conclusion \
        --jq 'if .status == "completed" then .conclusion else "" end' 2>/dev/null || true
}

# can_push <repo>: succeeds only if gh's token can push to repo. Without push
# access the releases list hides drafts, so release_state couldn't tell a
# draft from a missing release.
can_push() {
    [[ "$(gh api "repos/$1" --jq .permissions.push 2>/dev/null)" == true ]]
}
