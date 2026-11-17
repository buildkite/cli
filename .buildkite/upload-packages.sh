#!/bin/env bash

#
# This script is used to upload packages to Buildkite registries
#

set -euo pipefail

if [[ "${BUILDKITE_PIPELINE_SLUG:-}" != buildkite-cli-release || "${BUILDKITE_PULL_REQUEST:-false}" != false || "${BUILDKITE_TAG:-}" != "v$(cat VERSION)" ]]; then
    echo "Only version-tag builds in buildkite-cli-release can upload packages"
    exit 1
fi

if [[ -z "${1:-}" ]]; then
    echo "Must pass in the package type: deb, rpm"
    exit 1
fi

PACKAGE=${1}
ORGANIZATION=${2:-buildkite}
REGISTRY=${3:-cli-$PACKAGE}

audience() {
    ORG=$1
    REGISTRY=$2
    echo "https://packages.buildkite.com/${ORG}/${REGISTRY}"
}
upload_url() {
    ORG=$1
    REGISTRY=$2
    echo "https://api.buildkite.com/v2/packages/organizations/${ORG}/registries/${REGISTRY}/packages"
}

AUDIENCE=$(audience "$ORGANIZATION" "$REGISTRY")

# grab a token for pushing packages to buildkite with an expiry of 3 mins
echo "--- Fetching OIDC token for $AUDIENCE"
if ! TOKEN=$(buildkite-agent oidc request-token --audience "$AUDIENCE" --lifetime 180); then
    echo "Failed to retrieve OIDC token"
    exit 1
fi

for FILE in dist/*."${PACKAGE}"; do
    upload_key="uploaded-${FILE##*/}"
    checksum=$(sha256sum "$FILE" | awk '{print $1}')
    if buildkite-agent meta-data exists "$upload_key"; then
        [[ $(buildkite-agent meta-data get "$upload_key") == "$checksum" ]] || {
            echo "Package $FILE changed after publication; refusing to retry"
            exit 1
        }
        echo "Already uploaded $FILE in this build"
        continue
    else
        status=$?
        [[ "$status" == 100 ]] || exit "$status"
    fi
    echo "--- Pushing $FILE"
    if [[ $PACKAGE = "apk" ]]; then
        curl -s -X POST "$(upload_url "$ORGANIZATION" "$REGISTRY")" \
             -H "Authorization: Bearer ${TOKEN}" \
             -F "package[distro_version_id]=alpine/v3" \
             -F "package[package_file]=@${FILE}" \
            --fail-with-body
    else
        curl -s -X POST "$(upload_url "$ORGANIZATION" "$REGISTRY")" \
             -H "Authorization: Bearer ${TOKEN}" \
             -F "file=@${FILE}" \
            --fail-with-body
    fi

    buildkite-agent meta-data set "$upload_key" "$checksum"
done
