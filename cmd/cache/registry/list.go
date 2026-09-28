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

type ListCmd struct {
	ClusterUUID string `arg:"" help:"Cluster UUID to list cache registries for" name:"cluster-uuid"`
	PerPage     int    `help:"Number of cache registries per page" default:"30" name:"per-page"`
	Limit       int    `help:"Maximum number of cache registries to return" default:"100"`
	output.OutputFlags
}

func (c *ListCmd) Validate() error {
	if c.PerPage < 1 || c.PerPage > 100 {
		return fmt.Errorf("invalid --per-page %d: must be between 1 and 100", c.PerPage)
	}
	if c.Limit < 0 {
		return fmt.Errorf("invalid --limit %d: must be greater than or equal to 0", c.Limit)
	}
	return nil
}

func (c *ListCmd) Help() string {
	return `
Examples:
  $ bk cache registry list my-cluster-uuid
  $ bk cache registry list my-cluster-uuid --limit 200
  $ bk cache registry list my-cluster-uuid --output json
`
}

func (c *ListCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
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
	return c.run(ctx, f, f.Config.OrganizationSlug(), writer, format)
}

func (c *ListCmd) run(ctx context.Context, f *factory.Factory, org string, writer io.Writer, format output.Format) error {
	registries, err := c.fetch(ctx, f, org)
	if err != nil {
		return err
	}
	if format != output.FormatText {
		return output.Write(writer, registries, format)
	}
	if len(registries) == 0 {
		_, err := fmt.Fprintln(writer, "No cache registries found")
		return err
	}

	rows := make([][]string, 0, len(registries))
	for _, registry := range registries {
		rows = append(rows, []string{
			registry.Slug,
			registry.Name,
			optionalString(registry.Description),
			registry.UUID,
		})
	}
	_, err = fmt.Fprintf(writer, "Cache Registries (%d)\n\n%s\n", len(registries), output.Table(
		[]string{"Slug", "Name", "Description", "UUID"},
		rows,
		map[string]string{"slug": "bold"},
	))
	return err
}

func (c *ListCmd) fetch(ctx context.Context, f *factory.Factory, org string) ([]buildkite.CacheRegistry, error) {
	registries := make([]buildkite.CacheRegistry, 0)
	if c.Limit == 0 {
		return registries, nil
	}

	opts := &buildkite.CacheRegistriesListOptions{PerPage: min(c.PerPage, c.Limit)}
	seenCursors := make(map[string]struct{})
	for len(registries) < c.Limit {
		var page buildkite.CacheRegistriesList
		if err := bkIO.SpinWhile(f, "Fetching cache registries", func() error {
			var apiErr error
			page, _, apiErr = f.RestAPIClient.CacheRegistries.List(ctx, org, c.ClusterUUID, opts)
			return apiErr
		}); err != nil {
			return nil, fmt.Errorf("error fetching cache registries: %w", err)
		}

		remaining := c.Limit - len(registries)
		if len(page.Items) > remaining {
			page.Items = page.Items[:remaining]
		}
		registries = append(registries, page.Items...)
		if len(registries) >= c.Limit || page.Links.Next == "" {
			break
		}

		next, err := page.Links.Next.ToOptions()
		if err != nil {
			return nil, fmt.Errorf("error parsing next cache registries page: %w", err)
		}
		cursor := next.After
		if cursor == "" {
			cursor = next.Before
		}
		if cursor == "" {
			return nil, fmt.Errorf("API returned a next cache registries page without a cursor")
		}
		if _, exists := seenCursors[cursor]; exists {
			return nil, fmt.Errorf("API returned repeated cache registries cursor %q, stopping pagination to prevent an infinite loop", cursor)
		}
		seenCursors[cursor] = struct{}{}
		next.PerPage = min(c.PerPage, c.Limit-len(registries))
		opts = next
	}
	return registries, nil
}
