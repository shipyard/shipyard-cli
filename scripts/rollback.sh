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

# 1. Delete the GitHub release
echo "Step 1: Deleting GitHub release $TAG..."
if gh release view "$TAG" --repo "$REPO" >/dev/null 2>&1; then
    gh release delete "$TAG" --repo "$REPO" --yes
    echo "  Deleted GitHub release."
else
    echo "  No GitHub release found for $TAG (skipping)."
fi

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
echo "[x] GitHub release $TAG deleted"
echo "[x] Git tag $TAG deleted (remote + local)"
echo "[ ] Verify: gh release view --repo $REPO"
echo "[ ] Verify: curl -sI https://github.com/$REPO/releases/latest | grep location"
echo "[ ] Revert Homebrew cask commit if not done above"
echo "[ ] Notify team of rollback"
