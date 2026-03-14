FROM golang:1.26.1@sha256:c7e98cc0fd4dfb71ee7465fee6c9a5f079163307e4bf141b336bb9dae00159a5

COPY --from=goreleaser/goreleaser-pro:v2.14.3@sha256:086a121392cc9995517316ad6502a75c7b5a55d95a521938a556c7d3c352ec7e /usr/bin/goreleaser /usr/local/bin/goreleaser
COPY --from=golangci/golangci-lint:v2.11.3@sha256:e838e8ab68aaefe83e2408691510867ade9329c0e0b895a3fb35eb93d1c2a4ba /usr/bin/golangci-lint /usr/local/bin/golangci-lint
COPY --from=ghcr.io/ko-build/ko:fc0e4c0582f0801600929b5aaf6981aabdf47148 /ko-app/ko /usr/local/bin/ko
