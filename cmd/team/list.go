package team

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/internal/team"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v5"
)

type ListCmd struct {
	PerPage int `help:"Number of teams per page" default:"30"`
	Limit   int `help:"Maximum number of teams to return" default:"100"`
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
List the teams for an organization. By default, shows up to 100 teams.

Examples:
  # List all teams
  $ bk team list

  # List teams in JSON format
  $ bk team list -o json

  # List up to 200 teams
  $ bk team list --limit 200
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

	format := output.ResolveFormat(c.Output, f.Config.OutputFormat())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	teams, hasMore, err := listTeams(ctx, f, c.PerPage, c.Limit)
	if err != nil {
		return err
	}

	if format != output.FormatText {
		return output.Write(os.Stdout, teams, format)
	}

	if len(teams) == 0 {
		fmt.Fprintln(os.Stdout, "No teams found.")
		return nil
	}

	summary := team.TeamViewTable(teams...)

	writer, cleanup := bkIO.Pager(f.NoPager, f.Config.Pager())
	defer func() { _ = cleanup() }()

	totalDisplay := fmt.Sprintf("%d", len(teams))
	if hasMore {
		totalDisplay = fmt.Sprintf("%d+", len(teams))
	}
	fmt.Fprintf(writer, "Showing %s teams in %s\n\n", totalDisplay, f.Config.OrganizationSlug())
	fmt.Fprintf(writer, "%v\n", summary)

	return nil
}

func listTeams(ctx context.Context, f *factory.Factory, perPage, limit int) ([]buildkite.Team, bool, error) {
	if limit == 0 {
		return []buildkite.Team{}, false, nil
	}

	all := []buildkite.Team{}
	page := 1
	var previousFirstTeamID string

	// Fetch until more than limit results are seen, so an exact boundary can
	// be distinguished from truncation when reporting hasMore.
	for len(all) <= limit {
		opts := &buildkite.TeamsListOptions{
			ListOptions: buildkite.ListOptions{
				Page:    page,
				PerPage: perPage,
			},
		}

		var pageTeams []buildkite.Team
		spinErr := bkIO.SpinWhile(f, "Loading teams", func() error {
			var err error
			pageTeams, _, err = f.RestAPIClient.Teams.List(ctx, f.Config.OrganizationSlug(), opts)
			return err
		})
		if spinErr != nil {
			return nil, false, fmt.Errorf("error fetching team list: %v", spinErr)
		}

		if len(pageTeams) == 0 {
			break
		}

		if page > 1 && pageTeams[0].ID == previousFirstTeamID {
			return nil, false, fmt.Errorf("API returned duplicate page content at page %d, stopping pagination to prevent infinite loop", page)
		}
		previousFirstTeamID = pageTeams[0].ID

		all = append(all, pageTeams...)

		if len(pageTeams) < perPage {
			break
		}

		page++
	}

	hasMore := len(all) > limit
	if hasMore {
		all = all[:limit]
	}

	return all, hasMore, nil
}
