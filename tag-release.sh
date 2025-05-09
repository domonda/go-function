#!/bin/bash

SEMVER_REGEX='^v([0-9]+)\.([0-9]+)\.([0-9]+)(-[a-zA-Z0-9]+)?$'

if [[ ! "$1" =~ $SEMVER_REGEX ]]; then
    echo "Usage: $0 <version> [message]"
    exit 1
fi

VERSION=$1
MESSAGE="$VERSION"

# Check if a message was provided as the second argument
if [ -n "$2" ]; then
    MESSAGE="$2"
fi

echo "Tagging $VERSION with message: '$MESSAGE'"

git tag -a $VERSION -m "$MESSAGE"
git tag -a cli/$VERSION -m "$MESSAGE"
git tag -a htmlform/$VERSION -m "$MESSAGE"
git tag -a cmd/gen-func-wrappers/$VERSION -m "$MESSAGE"

echo "Tags to be pushed:"
git push --tags --dry-run

echo "Do you want to push tags to origin? (y/n)"
read CONFIRM
if [[ "$CONFIRM" == "y" || "$CONFIRM" == "Y" ]]; then
    git push origin --tags
else
    git tag -d $VERSION
    git tag -d cli/$VERSION
    git tag -d htmlform/$VERSION
    git tag -d cmd/gen-func-wrappers/$VERSION
    echo "Reverted local $VERSION tags"
fi