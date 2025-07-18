This project is the Buildkite CLI (`bk`)

## Commands
- Test: `go test ./...`
- Lint: `docker-compose -f .buildkite/docker-compose.yaml run golangci-lint golangci-lint run`
- Generate: `go generate` (required after GraphQL changes)
- Run: `go run cmd/bk/main.go`

## Environment
- `BUILDKITE_GRAPHQL_TOKEN` required for development

## Project Structure
- Main binary: `cmd/bk/main.go`
- GraphQL schema: `schema.graphql`
- CLI commands: `pkg/cli/`

## CLI Framework
- Uses Kong CLI framework (github.com/alecthomas/kong)

## Go Dependencies
- When investigating Go library behavior, check local source instead of web searches
- Use `go list -m -f "{{.Dir}}" <module-name>` to find local module source

## Notes
- CI: https://buildkite.com/buildkite/buildkite-cli
- Always format after changing code
