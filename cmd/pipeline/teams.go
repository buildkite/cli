package pipeline

import (
	"context"
	"fmt"
	"strings"

	"github.com/buildkite/cli/v3/internal/graphql"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/google/uuid"
)

func validateTeams(teams map[string]string) error {
	for id, access := range teams {
		if _, err := uuid.Parse(id); err != nil {
			return fmt.Errorf("invalid --team UUID %q: %w", id, err)
		}
		switch access {
		case "read_only", "build_and_read", "manage_build_and_read":
		default:
			return fmt.Errorf("invalid --team access level %q: use read_only, build_and_read, or manage_build_and_read", access)
		}
	}
	return nil
}

func (c *CopyCmd) resolveTeams(ctx context.Context, f *factory.Factory, org, slug string, isCrossOrg bool) (map[string]string, error) {
	if len(c.Teams) > 0 || isCrossOrg {
		return c.Teams, nil
	}

	teams := make(map[string]string)
	var cursor *string
	for {
		result, err := graphql.PipelineTeams(ctx, f.GraphQLClient, org+"/"+slug, cursor)
		if err != nil {
			return nil, fmt.Errorf("could not read source team assignments (use --team UUID=ACCESS_LEVEL to set them explicitly): %w", err)
		}
		if result.Pipeline == nil {
			return nil, fmt.Errorf("could not read team assignments for pipeline %s/%s; use --team UUID=ACCESS_LEVEL to set them explicitly", org, slug)
		}
		connection := result.Pipeline.Teams
		if connection == nil {
			return teams, nil
		}
		for _, edge := range connection.Edges {
			if edge == nil || edge.Node == nil || edge.Node.Team == nil {
				return nil, fmt.Errorf("source team assignment is not accessible; use --team UUID=ACCESS_LEVEL to set teams explicitly")
			}
			teams[edge.Node.Team.Uuid] = strings.ToLower(string(edge.Node.AccessLevel))
		}
		if connection.PageInfo == nil || !connection.PageInfo.HasNextPage {
			return teams, nil
		}
		if connection.PageInfo.EndCursor == nil || (cursor != nil && *cursor == *connection.PageInfo.EndCursor) {
			return nil, fmt.Errorf("could not paginate source team assignments; use --team UUID=ACCESS_LEVEL to set teams explicitly")
		}
		cursor = connection.PageInfo.EndCursor
	}
}
