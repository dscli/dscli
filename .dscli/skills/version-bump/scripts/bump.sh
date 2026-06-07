#!/bin/bash
# version-bump: update version.go, commit, tag & push with changelog
set -euo pipefail

PUSH=true
NEW_VER=""

# Parse flags
for arg in "$@"; do
    case "$arg" in
        --no-push) PUSH=false ;;
        --push)    PUSH=true ;;
        -h|--help)
            echo "Usage: bump.sh [--no-push] <version>"
            echo "  --no-push  Skip git push (tag created locally only)"
            echo "Example: bump.sh 0.7.12"
            exit 0
            ;;
        *) NEW_VER="$arg" ;;
    esac
done

if [ -z "$NEW_VER" ]; then
    echo "Usage: bump.sh [--no-push] <version>"
    echo "Example: bump.sh 0.7.12"
    exit 1
fi

# Validate semver-like format: X.Y.Z
if ! echo "$NEW_VER" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$'; then
    echo "ERROR: version must be X.Y.Z format (e.g. 0.7.12), got: $NEW_VER"
    exit 1
fi

cd "$(git rev-parse --show-toplevel)"

# Safety: ensure workspace is clean
if ! git diff-index --quiet HEAD --; then
    echo "ERROR: workspace is dirty. Please commit or stash changes first."
    exit 1
fi

# Safety: ensure we're on main branch
CURRENT_BRANCH=$(git branch --show-current)
if [ "$CURRENT_BRANCH" != "main" ]; then
    echo "ERROR: not on main branch (current: $CURRENT_BRANCH)."
    echo "  Version bumps must be done on main."
    exit 1
fi

echo "=== Bumping version to v$NEW_VER ==="

# Capture changelog BEFORE commit (changes since last tag)
LAST_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "")
if [ -n "$LAST_TAG" ]; then
    CHANGELOG=$(git log --oneline "$LAST_TAG..HEAD" --format="- %s" | sed -n "1,20p")
else
    CHANGELOG=$(git log --oneline -20 --format="- %s")
fi

# Check README.md — warn if untouched this release cycle
if [ -f README.md ]; then
    if [ -n "$LAST_TAG" ]; then
        if git diff --quiet "$LAST_TAG..HEAD" -- README.md; then
            echo "⚠️  WARNING: README.md has NOT been modified since $LAST_TAG."
            echo "   Please review if feature descriptions are up to date."
            echo "   (hint: update README.md → commit → then re-run bump)"
            echo ""
        fi
    fi
fi

# 1. Update version.go
sed -i 's/^\tVersion = ".*"/\tVersion = "'"$NEW_VER"'"/' version.go
echo "  version.go → $NEW_VER"

# 2. Stage version change
git add version.go

# 3. Commit
git commit -m "version: bump to v$NEW_VER"

# 4. Tag with changelog summary
TAG_MSG="v$NEW_VER
${CHANGELOG:-  (first tag)}"

git tag -a "v$NEW_VER" -m "$TAG_MSG"

# 5. Verify tag was created locally
if ! git rev-parse "refs/tags/v$NEW_VER" >/dev/null 2>&1; then
    echo "ERROR: tag v$NEW_VER was not created! This is unexpected." >&2
    exit 1
fi

echo ""
echo "=== Done ==="
echo "  Version:  v$NEW_VER"
echo "  Tag:      $(git describe --tags --abbrev=0)"
echo "  Commit:   $(git rev-parse --short HEAD)"

# 6. Push (if not disabled)
if $PUSH; then
    echo ""
    echo "--- Pushing commit and tag ---"
    git push || { echo "ERROR: git push failed" >&2; exit 1; }
    git push origin "refs/tags/v$NEW_VER" || { echo "ERROR: tag push failed" >&2; exit 1; }

    # Verify remote tag arrived
    sleep 1  # brief wait for remote sync
    if git ls-remote --tags origin "refs/tags/v$NEW_VER" | grep -q .; then
        echo "✅ Tag v$NEW_VER confirmed on remote."
    else
        echo "⚠️  WARNING: Remote tag verification failed."
        echo "   Check manually: git ls-remote --tags origin | grep v$NEW_VER"
    fi
else
    echo ""
    echo "Next (manual push):"
    echo "  git push && git push origin v$NEW_VER"
fi
