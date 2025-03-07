package testutil

import (
	"io"
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v4"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

// CommandInput contains the configuration for a test command
type CommandInput struct {
	TestServerURL string
	Flags         map[string]string
	Args          []string
	Stdin         io.Reader
	Factory       *factory.Factory
	NewCmd        func(*factory.Factory) *cobra.Command
}

// CreateCommand creates a test command with the given configuration
func CreateCommand(t *testing.T, input CommandInput) (*cobra.Command, error) {
	t.Helper()

	if input.Factory == nil {
		// Create default factory if none provided
		client, err := buildkite.NewOpts(buildkite.WithBaseURL(input.TestServerURL))
		if err != nil {
			return nil, err
		}

		conf := config.New(afero.NewMemMapFs(), nil)
		err = conf.SelectOrganization("test")
		if err != nil {
			t.Errorf("Error selecting organization: %s", err)
		}

		input.Factory = &factory.Factory{Config: conf, RestAPIClient: client}
	}

	cmd := input.NewCmd(input.Factory)

	args := []string{}
	for k, v := range input.Flags {
		args = append(args, "--"+k, v)
	}

	args = append(args, input.Args...)
	cmd.SetArgs(args)

	if input.Stdin != nil {
		cmd.SetIn(input.Stdin)
	}

	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	return cmd, nil
}
