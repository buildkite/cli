#!/bin/env bash

#
# This script is used to push a tag on the current commit
#

set -euo pipefail

TAG="$(buildkite-agent meta-data get "release-tag")"

echo "--- Verifying tag ${TAG}"
echo "Remote tags:"
git ls-remote --tags
echo "Local tags:"
git tag -l
# TODO verify the tag format and that its semver newer than the most previous

echo "--- Downloading gh"
curl -L https://github.com/cli/cli/releases/download/v2.51.0/gh_2.51.0_linux_amd64.tar.gz | tar xvz
echo "--- Logging in to gh"
echo "$GITHUB_TOKEN" | gh_2.51.0_linux_amd64/bin/gh auth login --with-token

echo "+++ Tagging ${BUILDKITE_COMMIT} with ${TAG}"
git tag "${TAG}"

git push origin "${TAG}"
