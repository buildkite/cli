This project is the Buildkite CLI (`bk`)

## Commands
- Test: `go test ./...`
- Lint: `docker-compose -f .buildkite/docker-compose.yaml run lint`
- Generate: `go generate` (required after GraphQL changes)
- Run: `go run cmd/bk/main.go`

## Environment
- `BUILDKITE_GRAPHQL_TOKEN` required for development

## Project Structure
- Main binary: `cmd/bk/main.go`
- GraphQL schema: `schema.graphql`
- CLI commands: `pkg/cmd/`

## Notes
- CI: https://buildkite.com/buildkite/buildkite-cli
