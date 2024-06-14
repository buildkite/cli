#!/bin/env bash

#
# This script is used to push a tag on the current commit
#

set -euo pipefail

TAG=$(buildkite-agent meta-data get "release-tag")

echo "Tagging ${BUILDKITE_COMMIT} with ${TAG}"

git tag "${TAG}"

echo "Local tags:"
git tag -l

git push origin "${TAG}"
echo "Remote tags:"
git ls-remote --tags
