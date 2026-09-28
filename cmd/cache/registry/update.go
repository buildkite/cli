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

type UpdateCmd struct {
	ClusterUUID      string  `arg:"" help:"Cluster UUID the cache registry belongs to" name:"cluster-uuid"`
	RegistryUUID     string  `arg:"" help:"Cache registry UUID to update" name:"registry-uuid"`
	Name             *string `help:"New name for the cache registry (regenerates its slug)" optional:""`
	Description      *string `help:"New description for the cache registry" optional:""`
	Emoji            *string `help:"New emoji for the cache registry" optional:""`
	Color            *string `help:"New color for the cache registry" optional:""`
	PolicyFile       string  `help:"Read the new policy from a YAML or JSON file, or from stdin with -" optional:"" name:"policy-file"`
	ClearDescription bool    `help:"Clear the cache registry description" name:"clear-description"`
	ClearEmoji       bool    `help:"Clear the cache registry emoji" name:"clear-emoji"`
	ClearColor       bool    `help:"Clear the cache registry color" name:"clear-color"`
	ClearPolicy      bool    `help:"Clear the cache registry policy" name:"clear-policy"`
	output.OutputFlags
}

func (c *UpdateCmd) Help() string {
	return `
At least one setter or clear flag must be provided. A metadata setter cannot be
combined with its corresponding clear flag. The policy file may contain YAML or
JSON and must have an object at its root. Policies are normalized by the API, so
YAML comments and formatting are not preserved.

Changing the name regenerates the registry slug. Jobs that explicitly select
the old slug must be updated to use the new one.

Examples:
  $ bk cache registry update my-cluster-uuid my-registry-uuid --name "Ruby gems"
  $ bk cache registry update my-cluster-uuid my-registry-uuid --policy-file policy.yml
  $ bk cache registry update my-cluster-uuid my-registry-uuid --clear-description
`
}

func (c *UpdateCmd) Validate() error {
	if c.Description != nil && c.ClearDescription {
		return fmt.Errorf("--description and --clear-description cannot be used together")
	}
	if c.Emoji != nil && c.ClearEmoji {
		return fmt.Errorf("--emoji and --clear-emoji cannot be used together")
	}
	if c.Color != nil && c.ClearColor {
		return fmt.Errorf("--color and --clear-color cannot be used together")
	}
	if c.PolicyFile != "" && c.ClearPolicy {
		return fmt.Errorf("--policy-file and --clear-policy cannot be used together")
	}
	if c.Name == nil && c.Description == nil && c.Emoji == nil && c.Color == nil && c.PolicyFile == "" &&
		!c.ClearDescription && !c.ClearEmoji && !c.ClearColor && !c.ClearPolicy {
		return fmt.Errorf("at least one update or clear flag must be provided")
	}
	return nil
}

func (c *UpdateCmd) input(stdin io.Reader) (buildkite.CacheRegistryUpdate, error) {
	var input buildkite.CacheRegistryUpdate
	if c.Name != nil {
		input.Name = buildkite.Some(*c.Name)
	}
	if c.Description != nil {
		input.Description = buildkite.Some(c.Description)
	} else if c.ClearDescription {
		input.Description = buildkite.Some[*string](nil)
	}
	if c.Emoji != nil {
		input.Emoji = buildkite.Some(c.Emoji)
	} else if c.ClearEmoji {
		input.Emoji = buildkite.Some[*string](nil)
	}
	if c.Color != nil {
		input.Color = buildkite.Some(c.Color)
	} else if c.ClearColor {
		input.Color = buildkite.Some[*string](nil)
	}
	if c.PolicyFile != "" {
		policy, err := readPolicy(c.PolicyFile, stdin)
		if err != nil {
			return input, err
		}
		input.Policy = buildkite.Some(policy)
	} else if c.ClearPolicy {
		input.Policy = buildkite.Some[buildkite.CacheRegistryPolicy](nil)
	}
	return input, nil
}

func (c *UpdateCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
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

func (c *UpdateCmd) run(ctx context.Context, f *factory.Factory, org string, stdin io.Reader, writer io.Writer, format output.Format) error {
	input, err := c.input(stdin)
	if err != nil {
		return err
	}

	var registry buildkite.CacheRegistry
	if err := bkIO.SpinWhile(f, "Updating cache registry", func() error {
		var apiErr error
		registry, _, apiErr = f.RestAPIClient.CacheRegistries.Update(ctx, org, c.ClusterUUID, c.RegistryUUID, input)
		return apiErr
	}); err != nil {
		return fmt.Errorf("error updating cache registry: %w", err)
	}

	return output.Write(writer, output.Viewable[buildkite.CacheRegistry]{Data: registry, Render: renderRegistryText}, format)
}
