package registry

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v5"
)

type CreateCmd struct {
	ClusterUUID string  `arg:"" help:"Cluster UUID to create the cache registry in" name:"cluster-uuid"`
	Name        string  `help:"Name for the cache registry" required:""`
	Description *string `help:"Description for the cache registry" optional:""`
	Emoji       *string `help:"Emoji for the cache registry" optional:""`
	Color       *string `help:"Color for the cache registry" optional:""`
	PolicyFile  string  `help:"Read the policy from a YAML or JSON file, or from stdin with -" optional:"" name:"policy-file"`
	output.OutputFlags
}

func (c *CreateCmd) Help() string {
	return `
The policy file may contain YAML or JSON and must have an object at its root.
Use --policy-file - to read from stdin. Policies are normalized by the API, so
YAML comments and formatting are not preserved.

Omitting --policy-file creates the registry with the default unrestricted
policy. Only use an unrestricted registry with builds you trust.

Examples:
  # Create with the default unrestricted policy (trusted builds only)
  $ bk cache registry create my-cluster-uuid --name "Ruby gems"

  # Create with an explicit policy
  $ bk cache registry create my-cluster-uuid --name "Ruby gems" --policy-file policy.yml
  $ cat policy.json | bk cache registry create my-cluster-uuid --name "Ruby gems" --policy-file -
`
}

func (c *CreateCmd) input(stdin io.Reader) (buildkite.CacheRegistryCreate, error) {
	input := buildkite.CacheRegistryCreate{Name: c.Name}
	if c.Description != nil {
		input.Description = buildkite.Some(c.Description)
	}
	if c.Emoji != nil {
		input.Emoji = buildkite.Some(c.Emoji)
	}
	if c.Color != nil {
		input.Color = buildkite.Some(c.Color)
	}
	if c.PolicyFile != "" {
		policy, err := readPolicy(c.PolicyFile, stdin)
		if err != nil {
			return input, err
		}
		input.Policy = buildkite.Some(policy)
	}
	return input, nil
}

func (c *CreateCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}
	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()
	f.NoPager = f.NoPager || globals.DisablePager()
	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	format := output.ResolveFormat(c.Output, f.Config.OutputFormat())
	writer, cleanup := commandWriter(f, format)
	defer func() { _ = cleanup() }()
	return c.run(ctx, f, f.Config.OrganizationSlug(), os.Stdin, writer, format)
}

func (c *CreateCmd) run(ctx context.Context, f *factory.Factory, org string, stdin io.Reader, writer io.Writer, format output.Format) error {
	input, err := c.input(stdin)
	if err != nil {
		return err
	}

	var registry buildkite.CacheRegistry
	if err := bkIO.SpinWhile(f, "Creating cache registry", func() error {
		var apiErr error
		registry, _, apiErr = f.RestAPIClient.CacheRegistries.Create(ctx, org, c.ClusterUUID, input)
		return apiErr
	}); err != nil {
		return fmt.Errorf("error creating cache registry: %w", err)
	}

	return output.Write(writer, output.Viewable[buildkite.CacheRegistry]{Data: registry, Render: renderRegistryText}, format)
}
