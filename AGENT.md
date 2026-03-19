This project is the Buildkite CLI (`bk`)

## Commands
- Bootstrap: `mise install`
- Hooks: `mise run hooks`
- Format: `mise run format`
- Test: `mise run test`
- Lint: `mise run lint`
- Generate: `mise run generate` (required after GraphQL changes)
- Run: `go run main.go`

## Environment
- `BUILDKITE_GRAPHQL_TOKEN` required for development

## Project Structure
- Main binary: `main.go`
- GraphQL schema: `schema.graphql`
- CLI commands: `pkg/cmd/`

## Notes
- `mise.toml` pins the local Go/tool versions
- CI: https://buildkite.com/buildkite/buildkite-cli
- Always format after changing code
