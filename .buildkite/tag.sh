#!/usr/bin/env bash

#
# This script calculates the next semantic version and pushes a tag
#

set -euo pipefail

RELEASE_TYPE="$(buildkite-agent meta-data get "release-type")"

if [[ "${RELEASE_TYPE}" == "major" ]]; then
  echo "🚨 Major releases require manual tagging to prevent accidents."
  echo "Please run: git tag vX.0.0 && git push origin vX.0.0"
  exit 1
fi

# Get latest tag matching v*.*.* pattern
LATEST_TAG=$(git describe --tags --match "v[0-9]*" --abbrev=0 2>/dev/null) || {
  echo "Error: No existing version tags found. Cannot calculate next version."
  exit 1
}
echo "Latest tag: ${LATEST_TAG}"

# Parse version (strip 'v' prefix and any pre-release suffix)
VERSION="${LATEST_TAG#v}"
IFS='.' read -r MAJOR MINOR PATCH <<< "${VERSION%%-*}"

# Calculate new version
case "${RELEASE_TYPE}" in
  minor)
    TAG="v${MAJOR}.$((MINOR + 1)).0"
    ;;
  patch)
    TAG="v${MAJOR}.${MINOR}.$((PATCH + 1))"
    ;;
  *)
    echo "Error: Unknown release type: ${RELEASE_TYPE}"
    exit 1
    ;;
esac

echo "New tag: ${TAG}"

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
