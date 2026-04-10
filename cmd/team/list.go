package team

import (
	"context"
	"errors"
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
	buildkite "github.com/buildkite/go-buildkite/v4"
)

type ListCmd struct {
	PerPage int `help:"Number of teams per page" default:"30"`
	Limit   int `help:"Maximum number of teams to return" default:"100"`
	output.OutputFlags
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
	var all []buildkite.Team
	var err error
	page := 1
	hasMore := false
	var previousFirstTeamID string

	for len(all) < limit {
		opts := &buildkite.TeamsListOptions{
			ListOptions: buildkite.ListOptions{
				Page:    page,
				PerPage: perPage,
			},
		}

		var pageTeams []buildkite.Team
		spinErr := bkIO.SpinWhile(f, "Loading teams", func() {
			pageTeams, _, err = f.RestAPIClient.Teams.List(ctx, f.Config.OrganizationSlug(), opts)
		})
		if spinErr != nil {
			return nil, false, spinErr
		}
		if err != nil {
			return nil, false, fmt.Errorf("error fetching team list: %v", err)
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

		if len(all) >= limit {
			hasMore = true
			break
		}

		page++
	}

	if len(all) > limit {
		all = all[:limit]
	}

	if len(all) == 0 {
		return nil, false, errors.New("no teams found in organization")
	}

	return all, hasMore, nil
}
