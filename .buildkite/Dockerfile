FROM golang:1.26.2@sha256:fcdb3e42c5544e9682a635771eac76a698b66de79b1b50ec5b9ce5c5f14ad775

COPY --from=goreleaser/goreleaser-pro:v2.15.2@sha256:1eab5a6d8ac8a228fe796e8783d7ab63a64b6b51b3c601c0c38abdf60b22a383 /usr/bin/goreleaser /usr/local/bin/goreleaser
COPY --from=golangci/golangci-lint:v2.11.4@sha256:67dfc9eeeb0eb13fc1a36329c2c378197dc561f1edf1a7792e3f771606bb0e15 /usr/bin/golangci-lint /usr/local/bin/golangci-lint
COPY --from=ghcr.io/ko-build/ko:fc0e4c0582f0801600929b5aaf6981aabdf47148 /ko-app/ko /usr/local/bin/ko
