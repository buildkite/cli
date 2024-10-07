#!/bin/env bash

#
# This script is used to push a tag on the current commit
#

set -euo pipefail

TAG="$(buildkite-agent meta-data get "release-tag")"

if git ls-remote --exit-code --tags origin "refs/tags/${TAG}" >/dev/null 2>&1; then
  echo "Error: Tag ${TAG} already exists at origin"
  exit 1
fi

echo "${TAG} does not exist at origin. Proceeding... 🚀"

echo "--- Downloading gh"
curl -sL https://github.com/cli/cli/releases/download/v2.57.0/gh_2.57.0_linux_amd64.tar.gz | tar xz
echo "--- Logging in to gh"
gh_2.57.0_linux_amd64/bin/gh auth setup-git

echo "+++ Tagging ${BUILDKITE_COMMIT} with ${TAG}"
git tag "${TAG}"
git push origin "${TAG}"
