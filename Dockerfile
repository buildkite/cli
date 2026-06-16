FROM golang:1.26-alpine AS base

RUN apk add --no-cache git ca-certificates

WORKDIR /base

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o bk .

FROM alpine:3.24.0

RUN apk --no-cache add ca-certificates \
    && addgroup -S buildkite \
    && adduser -S -D -h /home/buildkite -s /sbin/nologin -G buildkite buildkite \
    && mkdir -p /cli \
    && chown buildkite:buildkite /cli

WORKDIR /cli

COPY --chown=buildkite:buildkite --from=base /base/bk .

ENV HOME="/home/buildkite" \
    PATH="/cli:${PATH}"

USER buildkite

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 CMD bk --version >/dev/null || exit 1

ENTRYPOINT ["bk"]
