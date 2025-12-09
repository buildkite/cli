FROM golang:1.24-alpine AS base

RUN apk add --no-cache git ca-certificates

WORKDIR /base

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o bk .

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /cli

COPY --from=base /base/bk .

ENV PATH="/cli:${PATH}"

ENTRYPOINT ["bk"]
