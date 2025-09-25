This project is the Buildkite CLI (`bk`)

## Commands
- Test: `go test ./...`
- Lint: `docker-compose -f .buildkite/docker-compose.yaml run golangci-lint golangci-lint run`
- Generate: `go generate` (required after GraphQL changes)
- Run: `go run main.go`

## Environment
- `BUILDKITE_GRAPHQL_TOKEN` required for development

## Project Structure
- Main binary: `main.go`
- GraphQL schema: `schema.graphql`
- CLI commands: `pkg/cmd/`

## Notes
- CI: https://buildkite.com/buildkite/buildkite-cli
- Always format after changing code
