FROM golang:1.26.1@sha256:595c7847cff97c9a9e76f015083c481d26078f961c9c8dca3923132f51fe12f1

COPY --from=goreleaser/goreleaser-pro:v2.14.3@sha256:086a121392cc9995517316ad6502a75c7b5a55d95a521938a556c7d3c352ec7e /usr/bin/goreleaser /usr/local/bin/goreleaser
COPY --from=golangci/golangci-lint:v2.11.4@sha256:67dfc9eeeb0eb13fc1a36329c2c378197dc561f1edf1a7792e3f771606bb0e15 /usr/bin/golangci-lint /usr/local/bin/golangci-lint
COPY --from=ghcr.io/ko-build/ko:fc0e4c0582f0801600929b5aaf6981aabdf47148 /ko-app/ko /usr/local/bin/ko
