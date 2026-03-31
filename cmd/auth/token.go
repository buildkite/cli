package auth

import (
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
)

type TokenCmd struct{}

func (c *TokenCmd) Help() string {
	return `
Prints the stored API token for the currently selected organization to stdout.

The token is retrieved from the system keychain (or the BUILDKITE_API_TOKEN
environment variable if set). This is useful for passing the token to other
tools, for example:

Examples:
	# Print the current token
	$ bk auth token

	# Use the token in a curl request
	$ curl -H "Authorization: Bearer $(bk auth token)" https://api.buildkite.com/v2/user
`
}

func (c *TokenCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}

	token := f.Config.APIToken()
	if token == "" {
		return fmt.Errorf("no token found; run `bk auth login` to authenticate")
	}

	fmt.Fprintln(os.Stdout, token)
	return nil
}
