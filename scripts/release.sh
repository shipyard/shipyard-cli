#!/usr/bin/env bash
#
# Create a release tag for the shipyard CLI and monitor the release workflow.
#
# The actual build, test, and publish steps are handled by the GitHub Actions
# release workflow (.github/workflows/release.yaml) — this script only manages
# version computation, tagging, and CI monitoring.
#
# Usage: ./scripts/release.sh [flags]
#
# Flags:
#   --rc        Release candidate (e.g. v1.8.1-rc.1)
#   --minor     Bump minor version (v1.9.0)
#   --major     Bump major version (v2.0.0)
#   --dry-run   Print computed tag without side effects
#   --help      Show usage
#
# Prerequisites:
#   - gh CLI authenticated with write access to shipyard/shipyard-cli
#   - git remote "origin" pointing to shipyard/shipyard-cli
#   - Working tree clean and on the main branch
#
set -euo pipefail

# shellcheck source=scripts/release-state.sh
source "$(cd "$(dirname "$0")" && pwd)/release-state.sh"

REPO="shipyard/shipyard-cli"

# ── Flags ──────────────────────────────────────────────────────────────────────

BUMP_MAJOR=false
BUMP_MINOR=false
RC=false
DRY_RUN=false

usage() {
    cat <<EOF
Usage: $0 [flags]

Compute the next version tag, push it, and monitor the release workflow.

Flags:
  --rc        Release candidate (e.g. v1.8.1-rc.1)
  --minor     Bump minor version (v1.9.0)
  --major     Bump major version (v2.0.0)
  --dry-run   Print computed tag without side effects
  --help      Show this help
EOF
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --major)  BUMP_MAJOR=true; shift ;;
        --minor)  BUMP_MINOR=true; shift ;;
        --rc)     RC=true; shift ;;
        --dry-run) DRY_RUN=true; shift ;;
        --help)   usage; exit 0 ;;
        *)
            echo "Unknown flag: $1"
            usage
            exit 1
            ;;
    esac
done

if $BUMP_MAJOR && $BUMP_MINOR; then
    echo "Error: --major and --minor are mutually exclusive."
    exit 1
fi

# ── Cleanup trap ───────────────────────────────────────────────────────────────

TAG_PUSHED=false
NEXT_TAG=""

cleanup() {
    if $TAG_PUSHED && [[ -n "$NEXT_TAG" ]]; then
        echo ""
        echo "Cleaning up: deleting tag $NEXT_TAG..."
        git tag -d "$NEXT_TAG" 2>/dev/null || true
        git push origin --delete "$NEXT_TAG" 2>/dev/null || true
        echo "  Tag $NEXT_TAG removed locally and remotely."
    fi
}
trap cleanup ERR

# ── Preflight checks ──────────────────────────────────────────────────────────

if ! $DRY_RUN; then
    echo "=== Preflight Checks ==="

    # gh CLI authenticated
    if ! gh auth status >/dev/null 2>&1; then
        echo "Error: gh CLI is not authenticated. Run 'gh auth login' first."
        exit 1
    fi
    echo "  gh CLI: authenticated"

    # Working tree clean
    if [[ -n "$(git status --porcelain)" ]]; then
        echo "Error: working tree is not clean. Commit or stash changes first."
        exit 1
    fi
    echo "  Working tree: clean"

    # On main branch
    CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)
    if [[ "$CURRENT_BRANCH" != "main" ]]; then
        echo "Error: must be on the main branch (currently on '$CURRENT_BRANCH')."
        exit 1
    fi
    echo "  Branch: main"

    # Up to date with remote
    git fetch origin main --quiet
    LOCAL_SHA=$(git rev-parse HEAD)
    REMOTE_SHA=$(git rev-parse origin/main)
    if [[ "$LOCAL_SHA" != "$REMOTE_SHA" ]]; then
        echo "Error: local main is not up to date with origin/main."
        echo "  Local:  $LOCAL_SHA"
        echo "  Remote: $REMOTE_SHA"
        echo "  Run 'git pull' first."
        exit 1
    fi
    echo "  origin/main: up to date"
    echo ""
fi

# ── Detect latest tag ─────────────────────────────────────────────────────────

LATEST_STABLE=$(git tag --sort=-v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | head -1)

if [[ -z "$LATEST_STABLE" ]]; then
    echo "Error: no existing stable tags found (expected vX.Y.Z format)."
    exit 1
fi

# ── Compute next version ──────────────────────────────────────────────────────

VERSION="${LATEST_STABLE#v}"
MAJOR=$(echo "$VERSION" | cut -d. -f1)
MINOR=$(echo "$VERSION" | cut -d. -f2)
PATCH=$(echo "$VERSION" | cut -d. -f3)

if $BUMP_MAJOR; then
    NEXT_MAJOR=$((MAJOR + 1))
    NEXT_MINOR=0
    NEXT_PATCH=0
elif $BUMP_MINOR; then
    NEXT_MAJOR=$MAJOR
    NEXT_MINOR=$((MINOR + 1))
    NEXT_PATCH=0
else
    NEXT_MAJOR=$MAJOR
    NEXT_MINOR=$MINOR
    NEXT_PATCH=$((PATCH + 1))
fi

BASE="v${NEXT_MAJOR}.${NEXT_MINOR}.${NEXT_PATCH}"

if $RC; then
    # Find existing RCs for this base version and auto-increment
    LATEST_RC=$(git tag --sort=-v:refname | grep "^${BASE}-rc\." | head -1 || true)
    if [[ -n "$LATEST_RC" ]]; then
        RC_NUM=$(echo "$LATEST_RC" | sed "s/^${BASE}-rc\.//")
        NEXT_RC=$((RC_NUM + 1))
    else
        NEXT_RC=1
    fi
    NEXT_TAG="${BASE}-rc.${NEXT_RC}"
else
    NEXT_TAG="$BASE"
fi

# Check tag doesn't already exist
if git tag -l "$NEXT_TAG" | grep -q "$NEXT_TAG"; then
    echo "Error: tag $NEXT_TAG already exists."
    exit 1
fi

# ── Dry run ────────────────────────────────────────────────────────────────────

RELEASE_TYPE="stable"
$RC && RELEASE_TYPE="release candidate"

if $DRY_RUN; then
    echo "Dry run — no changes will be made."
    echo ""
    echo "  Current tag:  $LATEST_STABLE"
    echo "  Next tag:     $NEXT_TAG"
    echo "  Release type: $RELEASE_TYPE"
    exit 0
fi

# ── Confirm ────────────────────────────────────────────────────────────────────

echo "=== Release Summary ==="
echo "  Current tag:  $LATEST_STABLE"
echo "  Next tag:     $NEXT_TAG"
echo "  Release type: $RELEASE_TYPE"
echo ""
read -r -p "Proceed? [y/N] " CONFIRM
if [[ "$CONFIRM" != "y" && "$CONFIRM" != "Y" ]]; then
    echo "Aborted."
    exit 0
fi

# ── Tag and push ───────────────────────────────────────────────────────────────

echo ""
# release_state reads drafts from the releases list, which hides them from a
# token that can't push.
if ! can_push "$REPO"; then
    echo "Error: gh's token can't push to $REPO (check 'gh auth status')." >&2
    exit 1
fi
# The tag goes on HEAD. Note the runs that already exist for it, so the watch
# below can't mistake an earlier failed release of this tag and commit for the
# run this push starts. A failed lookup stops here, before anything is pushed.
TAG_SHA=$(git rev-parse HEAD)
KNOWN_RUNS=$(tag_runs "$NEXT_TAG" "$TAG_SHA" "$REPO" | tr '\n' ' ')

echo "Step 1: Creating tag $NEXT_TAG..."
git tag "$NEXT_TAG"
echo "  Tag created locally."

echo "Step 2: Pushing tag to origin..."
git push origin "$NEXT_TAG"
TAG_PUSHED=true
echo "  Tag pushed."

# ── Monitor CI ─────────────────────────────────────────────────────────────────

echo ""
echo "Step 3: Waiting for release workflow to start..."

# Poll for the workflow run to appear (may take a few seconds)
RUN_ID=""
for i in $(seq 1 30); do
    RUN_ID=$(find_release_run "$NEXT_TAG" "$TAG_SHA" "$REPO" "$KNOWN_RUNS")
    if [[ -n "$RUN_ID" ]]; then
        break
    fi
    sleep 2
done

if [[ -z "$RUN_ID" ]]; then
    echo "  Warning: could not find workflow run after 60s."
    echo "  Check manually: https://github.com/$REPO/actions"
    echo ""
    echo "  Tag $NEXT_TAG was pushed successfully."
    # Don't trigger cleanup — the tag is valid, CI just hasn't appeared yet.
    TAG_PUSHED=false
    exit 0
fi

echo "  Workflow run found: https://github.com/$REPO/actions/runs/$RUN_ID"
echo "  Watching run (Ctrl+C to stop watching — tag will remain)..."
echo ""

# Disable cleanup trap before watching — if the user Ctrl+C's, we don't want
# to delete the tag. Only CI failure should trigger cleanup.
trap - ERR

if gh run watch "$RUN_ID" --repo "$REPO" --exit-status; then
    echo ""
    echo "=== Release $NEXT_TAG succeeded ==="
    echo "  https://github.com/$REPO/releases/tag/$NEXT_TAG"
else
    # gh run watch also fails when it loses its connection, so only a run that
    # finished without succeeding leads to the recovery steps below.
    CONCLUSION=$(run_conclusion "$RUN_ID" "$REPO")
    if [[ -z "$CONCLUSION" ]]; then
        echo ""
        echo "  Stopped watching, but run $RUN_ID hasn't finished (or gh couldn't say)."
        echo "  Tag $NEXT_TAG left in place. Follow the run at"
        echo "  https://github.com/$REPO/actions/runs/$RUN_ID"
        exit 1
    fi
    if [[ "$CONCLUSION" == success ]]; then
        echo ""
        echo "=== Release $NEXT_TAG succeeded ==="
        echo "  https://github.com/$REPO/releases/tag/$NEXT_TAG"
        exit 0
    fi
    echo ""
    echo "=== Release workflow failed ==="
    echo ""
    # GoReleaser publishes before the attest step, so a late failure leaves a
    # live release. Don't offer to delete its tag; point at the recovery path.
    # Only a definite not-found reaches the delete prompt.
    RELEASE_STATE=$(release_state "$NEXT_TAG" "$REPO")
    if [[ "$RELEASE_STATE" == unknown ]]; then
        echo "  Couldn't tell whether release $NEXT_TAG exists (gh's error is above)."
        echo "  Tag $NEXT_TAG left in place. Check https://github.com/$REPO/releases: if the"
        echo "  release exists, don't rerun the workflow; re-attest it or withdraw it with"
        echo "  scripts/rollback.sh. If it doesn't, delete the tag here and on origin"
        echo "  (git tag -d $NEXT_TAG && git push origin --delete $NEXT_TAG) and run this"
        echo "  script again."
        exit 1
    fi
    # Re-attesting wouldn't finish a draft: its upload failed partway.
    if [[ "$RELEASE_STATE" == draft ]]; then
        echo "  Release $NEXT_TAG is still a draft: the upload failed partway and some assets"
        echo "  may be missing. Withdraw it with scripts/rollback.sh and release again."
        exit 1
    fi
    if [[ "$RELEASE_STATE" == published ]]; then
        echo "  Release $NEXT_TAG is already published, so the failure came after publishing"
        echo "  (attestation, or GoReleaser's Homebrew tap push). Check the run log."
        # Re-attesting needs the checksums the run saved; without them it can't work.
        case "$(saved_checksums "$RUN_ID" "$REPO")" in
            yes)
                echo "  If the release has no attestation, re-attest with:"
                echo "    gh workflow run release.yaml --repo $REPO --ref $NEXT_TAG -f run_id=$RUN_ID"
                echo "  To withdraw the release instead, use scripts/rollback.sh."
                ;;
            no)
                echo "  The run saved no checksums, so it can't be re-attested. Withdraw it with"
                echo "  scripts/rollback.sh and release again."
                ;;
            *)
                echo "  Couldn't check whether run $RUN_ID saved its checksums (gh's error is above)."
                echo "  If https://github.com/$REPO/actions/runs/$RUN_ID lists a checksums artifact"
                echo "  and the release has no attestation, re-attest with:"
                echo "    gh workflow run release.yaml --repo $REPO --ref $NEXT_TAG -f run_id=$RUN_ID"
                ;;
        esac
        exit 1
    fi
    # Only a definite not-found gets the prompt; anything else keeps the tag.
    if [[ "$RELEASE_STATE" != not-found ]]; then
        echo "  Unexpected release state '$RELEASE_STATE'; tag $NEXT_TAG left in place." >&2
        exit 1
    fi
    read -r -p "Delete tag $NEXT_TAG? [y/N] " DELETE_CONFIRM
    if [[ "$DELETE_CONFIRM" == "y" || "$DELETE_CONFIRM" == "Y" ]]; then
        # The prompt can wait a while: look again, in case a run of the tag
        # (re-run, new or on another commit) is going or a release appeared.
        # Anything but a definite answer keeps the tag.
        if [[ "$(release_state "$NEXT_TAG" "$REPO")" != not-found ]] \
            || [[ "$(runs_in_progress "$NEXT_TAG" "$REPO")" != no ]]; then
            echo "  Release $NEXT_TAG or a run of it appeared while waiting (or gh couldn't say);" >&2
            echo "  tag left in place." >&2
            exit 1
        fi
        echo "Cleaning up tag $NEXT_TAG..."
        # Only if origin's tag is still the commit this script tagged: someone
        # may have deleted it and pushed it again elsewhere meanwhile.
        if ! git push --force-with-lease="refs/tags/${NEXT_TAG}:${TAG_SHA}" origin ":refs/tags/${NEXT_TAG}"; then
            echo "  Origin's $NEXT_TAG is no longer $TAG_SHA, or the push failed; tag left in place." >&2
            exit 1
        fi
        git tag -d "$NEXT_TAG" 2>/dev/null || true
        echo "  Tag deleted. Fix the issue and re-run this script."
    else
        echo "  Tag $NEXT_TAG left in place. Clean up manually or use scripts/rollback.sh"
    fi
    exit 1
fi
