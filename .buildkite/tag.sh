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

echo "+++ Tagging ${BUILDKITE_COMMIT} with ${TAG}"
git tag "${TAG}"
git push origin "${TAG}"
