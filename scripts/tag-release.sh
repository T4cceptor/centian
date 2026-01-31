#!/usr/bin/env bash

# tag-release.sh - Automatically tag a new release based on semantic versioning
# Usage: ./scripts/tag-release.sh [major|minor|patch]
# Default: patch

set -euo pipefail

usage() {
    echo "Usage: $0 [major|minor|patch]"
    echo "  major - Increment major version (x.0.0)"
    echo "  minor - Increment minor version (x.y.0)"
    echo "  patch - Increment patch version (x.y.z) [default]"
}

# Get bump type from argument, default to patch
BUMP=${1:-patch}

# Validate bump type
case $BUMP in
    major|minor|patch)
        ;;
    *)
        echo "Error: Invalid bump type '$BUMP'"
        usage
        exit 1
        ;;
esac

if ! command -v git >/dev/null 2>&1; then
    echo "Error: git is required to run this script."
    exit 1
fi

if ! git diff --quiet || ! git diff --cached --quiet; then
    echo "Error: working tree is not clean. Commit or stash changes before tagging."
    exit 1
fi

# Get the latest tag, handling case where no tags exist
LATEST_TAG=$(git tag --list 'v*' --sort=-version:refname | head -1)

if [ -z "${LATEST_TAG}" ]; then
    LATEST_VERSION="0.0.0"
    echo "No existing tags found, starting from v0.0.0"
else
    LATEST_VERSION="${LATEST_TAG#v}"
fi

if ! [[ "${LATEST_VERSION}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "Error: latest tag '${LATEST_TAG}' is not a valid semver (vX.Y.Z)."
    exit 1
fi

echo "Current version: v${LATEST_VERSION}"

# Parse version components
IFS='.' read -r MAJOR MINOR PATCH <<< "${LATEST_VERSION}"

# Calculate new version based on bump type
case $BUMP in
    major)
        NEW_VERSION="$((MAJOR + 1)).0.0"
        ;;
    minor)
        NEW_VERSION="$MAJOR.$((MINOR + 1)).0"
        ;;
    patch)
        NEW_VERSION="$MAJOR.$MINOR.$((PATCH + 1))"
        ;;
esac

NEW_TAG="v$NEW_VERSION"

echo "New version: $NEW_TAG ($BUMP bump)"
echo

# Confirm with user
read -r -p "Create and push tag $NEW_TAG? This will trigger the release workflow. (y/N): " confirm

if [ "$confirm" = "y" ] || [ "$confirm" = "Y" ]; then
    REMOTE=${REMOTE:-origin}

    # Create annotated tag
    git tag -a "$NEW_TAG" -m "Release $NEW_TAG"
    echo "✓ Created tag: $NEW_TAG"
    
    # Push the tag to trigger release workflow
    git push "$REMOTE" "$NEW_TAG"
    echo "✓ Pushed tag: $NEW_TAG"
    echo
    echo "Release workflow should now be triggered automatically."
    echo "Check GitHub Actions for build status: https://github.com/T4cceptor/centian/actions"
else
    echo "Tag creation cancelled"
    exit 0
fi
