#!/usr/bin/env bash
set -euo pipefail

# Release helper for Syl: bumps semver, tags, and pushes to trigger GoReleaser.

# 1. Ensure git working directory is clean
if [[ -n $(git status --porcelain) ]]; then
  echo "❌ Error: Git working tree is dirty. Commit or stash changes before releasing." >&2
  exit 1
fi

# 2. Fetch latest tags
git fetch --tags origin

# 3. Find latest tag or default to v0.1.0
LATEST_TAG=$(git tag -l "v*" | sort -V | tail -n 1 || true)
if [[ -z "$LATEST_TAG" ]]; then
  LATEST_TAG="v0.1.0"
fi

# Strip leading 'v'
VERSION_RAW="${LATEST_TAG#v}"
IFS='.' read -r MAJOR MINOR PATCH <<< "$VERSION_RAW"
MAJOR=${MAJOR:-0}
MINOR=${MINOR:-1}
PATCH=${PATCH:-0}

BUMP_TYPE="${1:-patch}"

case "$BUMP_TYPE" in
  patch)
    PATCH=$((PATCH + 1))
    NEW_TAG="v${MAJOR}.${MINOR}.${PATCH}"
    ;;
  minor)
    MINOR=$((MINOR + 1))
    PATCH=0
    NEW_TAG="v${MAJOR}.${MINOR}.${PATCH}"
    ;;
  major)
    MAJOR=$((MAJOR + 1))
    MINOR=0
    PATCH=0
    NEW_TAG="v${MAJOR}.${MINOR}.${PATCH}"
    ;;
  v*)
    NEW_TAG="$BUMP_TYPE"
    ;;
  [0-9]*)
    NEW_TAG="v$BUMP_TYPE"
    ;;
  *)
    echo "Usage: $0 [patch | minor | major | <version>]"
    echo "Current latest tag: $LATEST_TAG"
    exit 1
    ;;
esac

echo "=========================================="
echo "Syl Release: $LATEST_TAG ➔ $NEW_TAG"
echo "=========================================="

# 4. Run test suite before cutting release
echo "Running tests..."
go test ./...

# 5. Tag and push
echo "Tagging release $NEW_TAG..."
git tag -a "$NEW_TAG" -m "Release $NEW_TAG"

echo "Pushing tag $NEW_TAG to origin..."
git push origin "$NEW_TAG"

echo ""
echo "✓ Successfully tagged and pushed $NEW_TAG!"
echo "GitHub Actions is now compiling cross-platform binaries and publishing the release."
echo "View status: https://github.com/Fhuss/syl/actions"
