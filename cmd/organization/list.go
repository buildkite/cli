package organization

import (
	"fmt"
	"os"
	"slices"
	"strconv"

	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/output"
)

type ListCmd struct {
	Output string `help:"Output format. One of: json, yaml, text" short:"o" default:"${output_default_format}" enum:"json,yaml,text"`
}

type Organization struct {
	Slug     string `json:"slug" yaml:"slug"`
	Selected bool   `json:"selected" yaml:"selected"`
}

func (c *ListCmd) Help() string {
	return `List configured organizations.

Examples:
  # List all configured organizations (JSON by default)
  $ bk organization list

  # List organizations in text format
  $ bk organization list -o text
`
}

func (c *ListCmd) Run(globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}

	f.NoPager = f.NoPager || globals.DisablePager()

	orgs := f.Config.ConfiguredOrganizations()
	if len(orgs) == 0 {
		fmt.Println("No organizations configured. Run `bk configure` to add one.")
		return nil
	}

	slices.Sort(orgs)
	selectedOrg := f.Config.OrganizationSlug()

	organizations := make([]Organization, len(orgs))
	for i, org := range orgs {
		organizations[i] = Organization{
			Slug:     org,
			Selected: org == selectedOrg,
		}
	}

	format := output.Format(c.Output)
	if format != output.FormatJSON && format != output.FormatYAML && format != output.FormatText {
		return fmt.Errorf("invalid output format: %s", c.Output)
	}

	if format != output.FormatText {
		return output.Write(os.Stdout, organizations, format)
	}

	rows := make([][]string, 0, len(organizations))
	for _, org := range organizations {
		rows = append(rows, []string{org.Slug, strconv.FormatBool(org.Selected)})
	}

	table := output.Table(
		[]string{"Organization Slug", "Selected"},
		rows,
		map[string]string{"organization slug": "bold", "selected": "italic"},
	)

	writer, cleanup := bkIO.Pager(f.NoPager)
	defer func() { _ = cleanup() }()

	fmt.Fprintf(writer, "Showing configured organization(s)\n\n%s\n", table)

	return nil
}
