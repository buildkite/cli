#!/bin/env bash

#
# This script is used to upload packages to Buildkite registries
#

set -uo pipefail

if [[ -z "${1}" ]]; then
    echo "Must pass in the package type: apk, deb, rpm"
    exit 1
fi

PACKAGE=${1}
ORGANIZATION=${2:-buildkite}
REGISTRY=${3:-cli-$PACKAGE}

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

AUDIENCE=$(audience $ORGANIZATION $REGISTRY)

# grab a token for pushing packages to buildkite with an expiry of 3 mins
echo "--- Fetching OIDC token for $AUDIENCE"
TOKEN=$(buildkite-agent oidc request-token --audience "$AUDIENCE" --lifetime 180)

if [[ $? -ne 0 ]]; then
    echo "Failed to retrieve OIDC token"
    exit 1
fi

for FILE in dist/linux/*.${PACKAGE}; do
    echo "--- Pushing $FILE"
    curl -s -X POST $(upload_url $ORGANIZATION $REGISTRY) \
         -H "Authorization: Bearer ${TOKEN}" \
         -F "file=@${FILE}" \
        --fail-with-body

    if [[ $? -ne 0 ]]; then
        echo "Failed to push package $file"
        exit 1
    fi
done
