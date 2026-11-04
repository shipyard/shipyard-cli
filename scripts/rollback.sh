#!/usr/bin/env bash
#
# Rollback a bad release of the shipyard CLI.
#
# Deletes the GitHub release and git tag, then reverts the Homebrew cask
# commit so all distribution channels serve the previous stable version.
#
# Usage: ./scripts/rollback.sh <version-tag>
#   e.g. ./scripts/rollback.sh v1.9.0
#
# Prerequisites:
#   - gh CLI authenticated with write access to shipyard/shipyard-cli
#     and shipyard/homebrew-tap
#   - git remote "origin" pointing to shipyard/shipyard-cli
#
set -euo pipefail

# shellcheck source=scripts/release-state.sh
source "$(cd "$(dirname "$0")" && pwd)/release-state.sh"

TAG="${1:-}"
if [[ -z "$TAG" ]]; then
    echo "Usage: $0 <version-tag>"
    echo "  e.g. $0 v1.9.0"
    exit 1
fi

REPO="shipyard/shipyard-cli"
TAP_REPO="shipyard/homebrew-tap"

echo "=== Rolling back release $TAG ==="
echo ""

# 1. Delete the GitHub release. The tag is deleted only once the release is
# known to be gone: deleting it first would leave a live release with no tag.
echo "Step 1: Deleting GitHub release $TAG..."
if ! can_push "$REPO"; then
    echo "  gh's token can't push to $REPO (check 'gh auth status'); nothing changed." >&2
    exit 1
fi
RELEASE_DELETED=false
case "$(release_state "$TAG" "$REPO")" in
    published | draft)
        gh release delete "$TAG" --repo "$REPO" --yes
        RELEASE_DELETED=true
        echo "  Deleted GitHub release."
        ;;
    not-found)
        echo "  No GitHub release found for $TAG (skipping)."
        ;;
    *)
        echo "  Couldn't tell whether release $TAG exists (gh's error is above); nothing changed." >&2
        exit 1
        ;;
esac

# 2. Delete the git tag (remote and local)
echo "Step 2: Deleting git tag $TAG..."
if git ls-remote --tags origin "$TAG" | grep -q "$TAG"; then
    git push origin --delete "$TAG"
    echo "  Deleted remote tag."
else
    echo "  Remote tag $TAG not found (skipping)."
fi
if git tag -l "$TAG" | grep -q "$TAG"; then
    git tag -d "$TAG"
    echo "  Deleted local tag."
fi

# 3. Verify /releases/latest resolves to the previous version
echo "Step 3: Verifying /releases/latest..."
LATEST=$(gh release view --repo "$REPO" --json tagName --jq '.tagName' 2>/dev/null || echo "NONE")
echo "  /releases/latest now points to: $LATEST"
if [[ "$LATEST" == "$TAG" ]]; then
    echo "  WARNING: /releases/latest still shows the rolled-back tag."
    echo "  GitHub may take a moment to update. Check again shortly."
fi

# 4. Revert the Homebrew cask commit
echo "Step 4: Reverting Homebrew cask in $TAP_REPO..."
CASK_COMMIT=$(gh api "repos/$TAP_REPO/commits?path=Casks&per_page=5" \
    --jq ".[].sha" 2>/dev/null | head -1)

if [[ -n "$CASK_COMMIT" ]]; then
    CASK_MSG=$(gh api "repos/$TAP_REPO/commits/$CASK_COMMIT" \
        --jq '.commit.message' 2>/dev/null || echo "")
    if echo "$CASK_MSG" | grep -q "$TAG"; then
        echo "  Found cask commit: $CASK_COMMIT"
        echo "  Message: $CASK_MSG"
        echo ""
        echo "  To revert, run:"
        echo "    cd /tmp && git clone git@github.com:$TAP_REPO.git && cd homebrew-tap"
        echo "    git revert --no-edit $CASK_COMMIT"
        echo "    git push origin main"
    else
        echo "  Latest cask commit does not match $TAG (skipping)."
        echo "  Commit: $CASK_MSG"
    fi
else
    echo "  Could not find cask commits (skipping)."
fi

echo ""
echo "=== Rollback Checklist ==="
if $RELEASE_DELETED; then
    echo "[x] GitHub release $TAG deleted"
else
    echo "[x] GitHub release $TAG: none found"
fi
echo "[x] Git tag $TAG deleted (remote + local)"
echo "[ ] Verify: gh release view --repo $REPO"
echo "[ ] Verify: curl -sI https://github.com/$REPO/releases/latest | grep location"
echo "[ ] Revert Homebrew cask commit if not done above"
echo "[ ] Notify team of rollback"
echo ""
# gstack-shortcut(dec-90477c5c): reuse is warned against, not blocked; upgrade when release.sh refuses a tag whose earlier run reached publishing
echo "Release the fix as a new version, not $TAG again: $TAG's attestations aren't"
echo "deleted, so the withdrawn binaries would still verify as $TAG."
