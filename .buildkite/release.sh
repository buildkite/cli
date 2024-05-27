#!/bin/env bash

# NOTE: do not exit on non-zero returns codes
set -uo pipefail

AUDIENCE=$1

goreleaser release --clean --snapshot

if [ ! $? ]; then
    echo "Failed to build a release"
    exit 1
fi

# grab a token for pushing packages to buildkite with an expiry of 3 mins
TOKEN=$(buildkite-agent oidc request-token --audience "$AUDIENCE" --lifetime 180)

if [ ! $? ]; then
    echo "Failed to retrieve OIDC token"
    exit 1
fi

for FILE in dist/*.rpm; do
    curl -X POST https://api.buildkite.com/v2/packages/organizations/jradtilbrook/registries/cli-rpm/packages \
         -H "Authorization: Bearer ${TOKEN}" \
         -F "file=@${FILE}"
        --fail-with-body

    if [ ! $? ]; then
        echo "Failed to push RPM package $file"
        exit 1
    fi
done
