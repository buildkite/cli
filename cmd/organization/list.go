package organization

import (
	"fmt"
	"os"
	"slices"

	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/output"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

type ListCmd struct {
	Output string `help:"Output format. One of: json, yaml, text" short:"o" default:"${output_default_format}"`
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
	f, err := factory.New()
	if err != nil {
		return err
	}

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

	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("238"))).
		Headers("ORGANIZATION", "SELECTED")

	for _, org := range organizations {
		selected := ""
		if org.Selected {
			selected = "*"
		}
		t.Row(org.Slug, selected)
	}

	fmt.Fprintln(os.Stdout, t)

	return nil
}
