#!/bin/env bash

#
# This script is used to build a release of the CLI and publish it to multiple registries on Buildkite
#

# NOTE: do not exit on non-zero returns codes
set -uo pipefail

# should we publish the build packages
PUBLISH=${PUBLISH:-false}

audience() {
    ORG=$1
    REGISTRY=$2
    echo "https://packages.buildkite.com/organizations/${ORG}/packages/registries/${REGISTRY}"
}
upload_url() {
    ORG=$1
    REGISTRY=$2
    echo "https://api.buildkite.com/v2/packages/organizations/${ORG}/registries/${REGISTRY}/packages"
}

GORELEASER_KEY=$(buildkite-agent secret get goreleaser_key)
if [[ $? -ne 0 ]]; then
    echo "Failed to retrieve GoReleaser Pro key"
    exit 1
fi

GITHUB_TOKEN=$(buildkite-agent secret get github_token)
if [[ $? -ne 0 ]]; then
    echo "Failed to retrieve GitHub token"
    echo "Will continue to allow building (without releasing)"
fi

if ! goreleaser "$@"; then
    echo "Failed to build a release"
    exit 1
fi

if [[ $PUBLISH == false ]]; then
    echo "Not publishing package to registries"
    exit 0
fi

AUDIENCE=$(audience $ORGANIZATION $REGISTRY)

# grab a token for pushing packages to buildkite with an expiry of 3 mins
TOKEN=$(buildkite-agent oidc request-token --audience "$AUDIENCE" --lifetime 180)

if [[ $? -ne 0 ]]; then
    echo "Failed to retrieve OIDC token"
    exit 1
fi

for FILE in dist/*.${PACKAGE}; do
    curl -X POST $(upload_url $ORGANIZATION $REGISTRY) \
         -H "Authorization: Bearer ${TOKEN}" \
         -F "file=@${FILE}" \
        --fail-with-body

    if [[ $? -ne 0 ]]; then
        echo "Failed to push RPM package $file"
        exit 1
    fi
done
