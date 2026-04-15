FROM golang:1.26.2@sha256:5f3787b7f902c07c7ec4f3aa91a301a3eda8133aa32661a3b3a3a86ab3a68a36

COPY --from=goreleaser/goreleaser-pro:v2.15.3@sha256:7d24915bf657b184e4478b9ec89c53a7052e4084a37c6d94b84c19769f6d05b1 /usr/bin/goreleaser /usr/local/bin/goreleaser
COPY --from=golangci/golangci-lint:v2.11.4@sha256:67dfc9eeeb0eb13fc1a36329c2c378197dc561f1edf1a7792e3f771606bb0e15 /usr/bin/golangci-lint /usr/local/bin/golangci-lint
COPY --from=ghcr.io/ko-build/ko:fc0e4c0582f0801600929b5aaf6981aabdf47148 /ko-app/ko /usr/local/bin/ko
