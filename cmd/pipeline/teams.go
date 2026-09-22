package pipeline

import (
	"context"
	"fmt"
	"strings"

	"github.com/buildkite/cli/v3/internal/graphql"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	buildkite "github.com/buildkite/go-buildkite/v5"
)

func validateTeams(teams map[string]string) error {
	for name, access := range teams {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("--team requires a non-empty team name")
		}
		switch access {
		case "read_only", "build_and_read", "manage_build_and_read":
		default:
			return fmt.Errorf("invalid --team access level %q: use read_only, build_and_read, or manage_build_and_read", access)
		}
	}
	return nil
}

func resolveTeamNames(ctx context.Context, client *buildkite.Client, org string, names map[string]string) (map[string]string, error) {
	if len(names) == 0 {
		return nil, nil
	}
	assignments := make(map[string]string, len(names))
	matched := make(map[string]string, len(names))
	opts := &buildkite.TeamsListOptions{ListOptions: buildkite.ListOptions{Page: 1, PerPage: 100}}
	for {
		teams, resp, err := client.Teams.List(ctx, org, opts)
		if err != nil {
			return nil, fmt.Errorf("could not resolve team names in organization %q: %w", org, err)
		}
		for _, team := range teams {
			access, requested := names[team.Name]
			if !requested {
				continue
			}
			if id, exists := matched[team.Name]; exists && id != team.ID {
				return nil, fmt.Errorf("team name %q is ambiguous in organization %q", team.Name, org)
			}
			matched[team.Name] = team.ID
			assignments[team.ID] = access
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	for name := range names {
		if _, exists := matched[name]; !exists {
			return nil, fmt.Errorf("team %q not found in organization %q; use the exact team name from bk team list", name, org)
		}
	}
	return assignments, nil
}

func (c *CopyCmd) resolveTeams(ctx context.Context, f *factory.Factory, org, slug, targetOrg string) (map[string]string, error) {
	if len(c.Teams) > 0 {
		client := f.RestAPIClient
		if targetOrg != org {
			var err error
			client, err = c.getClientForOrg(f, targetOrg)
			if err != nil {
				return nil, err
			}
		}
		return resolveTeamNames(ctx, client, targetOrg, c.Teams)
	}
	if targetOrg != org {
		return nil, nil
	}

	teams := make(map[string]string)
	var cursor *string
	for {
		result, err := graphql.PipelineTeams(ctx, f.GraphQLClient, org+"/"+slug, cursor)
		if err != nil {
			return nil, fmt.Errorf("could not read source team assignments (use --team NAME=ACCESS_LEVEL to set them explicitly): %w", err)
		}
		if result.Pipeline == nil {
			return nil, fmt.Errorf("could not read team assignments for pipeline %s/%s; use --team NAME=ACCESS_LEVEL to set them explicitly", org, slug)
		}
		connection := result.Pipeline.Teams
		if connection == nil {
			return teams, nil
		}
		for _, edge := range connection.Edges {
			if edge == nil || edge.Node == nil || edge.Node.Team == nil {
				return nil, fmt.Errorf("source team assignment is not accessible; use --team NAME=ACCESS_LEVEL to set teams explicitly")
			}
			teams[edge.Node.Team.Uuid] = strings.ToLower(string(edge.Node.AccessLevel))
		}
		if connection.PageInfo == nil || !connection.PageInfo.HasNextPage {
			return teams, nil
		}
		if connection.PageInfo.EndCursor == nil || (cursor != nil && *cursor == *connection.PageInfo.EndCursor) {
			return nil, fmt.Errorf("could not paginate source team assignments; use --team NAME=ACCESS_LEVEL to set teams explicitly")
		}
		cursor = connection.PageInfo.EndCursor
	}
}
