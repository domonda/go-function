#!/bin/bash

# Just in case the script is run from another directory
SCRIPT_DIR=$(cd -P -- $(dirname -- "$0") && pwd -P)
cd $SCRIPT_DIR

MODULE_PATHS=("" "cli/" "htmlform/" "cmd/gen-func-wrappers/")

SEMVER_REGEX='^v([0-9]+)\.([0-9]+)\.([0-9]+)(-[a-zA-Z0-9]+)?$'
if [[ ! "$1" =~ $SEMVER_REGEX ]]; then
    echo "Usage: $0 <version> [message]"
    exit 1
fi

VERSION=$1
MESSAGE="$VERSION"
if [ -n "$2" ]; then
    MESSAGE="$2" # provided as the second argument
fi

echo "Tagging $VERSION with message: '$MESSAGE'"

for PREFIX in "${MODULE_PATHS[@]}"; do
    git tag -a "${PREFIX}${VERSION}" -m "$MESSAGE"
done

echo "Tags to be pushed:"
git push --tags --dry-run

echo "Do you want to push tags to origin? (y/n)"
read CONFIRM
if [[ "$CONFIRM" == "y" || "$CONFIRM" == "Y" ]]; then
    git push origin --tags
else
    for PREFIX in "${MODULE_PATHS[@]}"; do
        git tag -d "${PREFIX}${VERSION}"
    done
    echo "Reverted local $VERSION tags"
fi