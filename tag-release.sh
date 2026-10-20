#!/bin/bash

set -euo pipefail

# Just in case the script is run from another directory
SCRIPT_DIR=$(cd -P -- "$(dirname -- "$0")" && pwd -P)
cd "$SCRIPT_DIR"

# All released modules of go.work are tagged in lockstep with the same version,
# the tools module only pins development tools and is never tagged
MODULE_PATHS=("" "cli/" "cmd/gen-func-wrappers/")
RELEASE_BRANCH="main"

VERSION_FILE_CONTENT=""
if [ -f VERSION ]; then
    VERSION_FILE_CONTENT=$(tr -d '[:space:]' < VERSION)
fi

# Show current tags and usage if no arguments provided
if [ -z "${1:-}" ]; then
    echo "Current release tags:"
    echo ""

    for PREFIX in "${MODULE_PATHS[@]}"; do
        if [ -z "$PREFIX" ]; then
            MODULE_NAME="(root module)"
        else
            MODULE_NAME="${PREFIX%/}"
        fi
        echo "  Module: $MODULE_NAME"

        # Get the latest tag for this module
        LATEST_TAG=$(git tag -l "${PREFIX}v*" --sort=-v:refname | head -1)
        if [ -n "$LATEST_TAG" ]; then
            echo "    Latest: $LATEST_TAG"
            # Show last 3 tags for this module
            echo "    Recent:"
            git tag -l "${PREFIX}v*" --sort=-v:refname | head -3 | sed 's/^/      /'
        else
            echo "    No tags yet"
        fi
        echo ""
    done

    if [ -n "$VERSION_FILE_CONTENT" ]; then
        echo "VERSION file: $VERSION_FILE_CONTENT"
        echo ""
    fi
    echo "Usage: $0 <version> [message]"
    echo ""
    echo "Creates tags for all modules with the specified version"
    echo "at a clean HEAD that equals origin/$RELEASE_BRANCH."
    echo "The version must match the VERSION file."
    echo ""
    EXAMPLE_VERSION="${VERSION_FILE_CONTENT:-v1.2.3}"
    if [ -n "$VERSION_FILE_CONTENT" ]; then
        echo "Examples for the VERSION file $EXAMPLE_VERSION:"
    else
        echo "Examples:"
    fi
    echo "  $0 $EXAMPLE_VERSION               # Creates $EXAMPLE_VERSION, cli/$EXAMPLE_VERSION, cmd/gen-func-wrappers/$EXAMPLE_VERSION"
    echo "  $0 $EXAMPLE_VERSION \"bug fixes\"   # Same with custom message"
    echo ""
    echo "Version format: vMAJOR.MINOR.PATCH[-PRERELEASE]"
    exit 0
fi

# No leading zeros, the go command ignores such tags.
# The pre-release has dot-separated identifiers like -rc.1,
# numeric identifiers without leading zeros.
SEMVER_IDENT='(0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)'
SEMVER_REGEX="^v(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)(-${SEMVER_IDENT}(\\.${SEMVER_IDENT})*)?\$"
if [[ ! "$1" =~ $SEMVER_REGEX ]]; then
    echo "Error: Invalid version format: $1"
    echo ""
    echo "Usage: $0 <version> [message]"
    echo "Version must be in format: vMAJOR.MINOR.PATCH[-PRERELEASE]"
    echo "Examples: v0.99.1, v1.0.0, v2.1.3-beta1, v2.0.0-rc.1"
    exit 1
fi

VERSION=$1
MESSAGE="$VERSION"
if [ -n "${2:-}" ]; then
    MESSAGE="$2" # provided as the second argument
fi

if [ -n "$VERSION_FILE_CONTENT" ] && [ "$VERSION" != "$VERSION_FILE_CONTENT" ]; then
    echo "Error: Version $VERSION does not match the VERSION file: $VERSION_FILE_CONTENT"
    exit 1
fi

if ! git diff --quiet || ! git diff --cached --quiet; then
    echo "Error: The working tree has uncommitted changes"
    exit 1
fi

git fetch --quiet --tags origin "$RELEASE_BRANCH"
if [ "$(git rev-parse HEAD)" != "$(git rev-parse "origin/$RELEASE_BRANCH")" ]; then
    echo "Error: HEAD is not origin/$RELEASE_BRANCH, check out and pull $RELEASE_BRANCH first"
    exit 1
fi

TAGS=()
for PREFIX in "${MODULE_PATHS[@]}"; do
    TAG="${PREFIX}${VERSION}"
    if git rev-parse --quiet --verify "refs/tags/$TAG" > /dev/null; then
        echo "Error: Tag $TAG already exists"
        exit 1
    fi
    TAGS+=("$TAG")
done

# Delete the tags created so far if anything fails before they are pushed
CREATED_TAGS=()
delete_created_tags() {
    # ${array[@]+...} avoids an unbound variable error for an empty array in bash 3.2 with set -u
    for TAG in ${CREATED_TAGS[@]+"${CREATED_TAGS[@]}"}; do
        git tag -d "$TAG"
    done
}
trap delete_created_tags ERR
trap 'delete_created_tags; exit 130' INT TERM

echo "Tagging $VERSION with message: '$MESSAGE'"

for TAG in "${TAGS[@]}"; do
    echo "  tag $TAG"
    git tag -a "$TAG" -m "$MESSAGE"
    CREATED_TAGS+=("$TAG")
done

# Full ref names, because a branch with the name of a tag makes the short name ambiguous
TAG_REFS=("${TAGS[@]/#/refs/tags/}")

echo "Tags to be pushed"
git push --dry-run --atomic origin "${TAG_REFS[@]}"

echo "Do you want to push tags to origin? (y/n)"
read -r CONFIRM
if [[ "$CONFIRM" == "y" || "$CONFIRM" == "Y" ]]; then
    # An interrupted push could have pushed the tags, so keep them,
    # but a failed atomic push pushed none, so the ERR trap deletes them
    trap - INT TERM
    # All tags or none, so the module proxy never sees a partial release
    git push --atomic origin "${TAG_REFS[@]}"
else
    delete_created_tags
    echo "Reverted local $VERSION tags"
fi
