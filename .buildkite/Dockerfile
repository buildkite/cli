FROM golang:1.26.0@sha256:c83e68f3ebb6943a2904fa66348867d108119890a2c6a2e6f07b38d0eb6c25c5

COPY --from=goreleaser/goreleaser-pro:v2.13.3@sha256:34eed2b04907d777ed443e4190262f0d9882b4b03c61a9783eee996ac8b7c657 /usr/bin/goreleaser /usr/local/bin/goreleaser
COPY --from=golangci/golangci-lint:v2.9.0@sha256:dbe337da33f7d649126107cd2cf0375e932c156c1e20e32a2d5205a79ba8c7a8 /usr/bin/golangci-lint /usr/local/bin/golangci-lint
COPY --from=ghcr.io/ko-build/ko:fc0e4c0582f0801600929b5aaf6981aabdf47148 /ko-app/ko /usr/local/bin/ko
