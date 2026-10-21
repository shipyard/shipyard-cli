#!/usr/bin/env bash
#
# release_state, sourced by release.sh and tested by release-test.sh.

# release_state <tag> <repo>: prints published, draft, not-found or unknown.
#
# GoReleaser creates a draft and publishes it after the uploads, so a draft
# means the upload failed partway. Only a definite "release not found" is
# not-found: any other gh error, or output that isn't true/false, is unknown,
# and gh's message goes to stderr. stderr is kept apart from the answer so a
# warning on a successful call can't hide a draft.
release_state() {
    local err is_draft rc=0
    err=$(mktemp)
    is_draft=$(gh release view "$1" --repo "$2" --json isDraft --jq .isDraft 2>"$err") || rc=$?
    if (( rc == 0 )); then
        case "$is_draft" in
            true)  echo draft ;;
            false) echo published ;;
            *)     cat "$err" >&2; echo unknown ;;
        esac
    elif grep -q "release not found" "$err"; then
        echo not-found
    else
        cat "$err" >&2
        echo unknown
    fi
    rm -f "$err"
}
