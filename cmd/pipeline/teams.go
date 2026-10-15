package pipeline

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	buildkite "github.com/buildkite/go-buildkite/v5"
)

var teamUUIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func isTeamUUID(s string) bool {
	return teamUUIDPattern.MatchString(s)
}

func validateTeams(teams map[string]string) error {
	for key, access := range teams {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("--team requires a non-empty team slug or UUID")
		}
		switch access {
		case "read_only", "build_and_read", "manage_build_and_read":
		default:
			return fmt.Errorf("invalid --team access level %q: use read_only, build_and_read, or manage_build_and_read", access)
		}
	}
	return nil
}

// resolveTeamAssignments converts user-supplied --team entries into the UUID-keyed
// map the REST API expects. UUIDs are passed through unchanged; slugs are looked
// up via the Teams REST API, which requires read_teams access.
func resolveTeamAssignments(ctx context.Context, client *buildkite.Client, org string, teams map[string]string) (map[string]string, error) {
	if len(teams) == 0 {
		return nil, nil
	}
	assignments := make(map[string]string, len(teams))
	slugs := make(map[string]string)
	for key, access := range teams {
		if isTeamUUID(key) {
			assignments[key] = access
			continue
		}
		slugs[key] = access
	}
	if len(slugs) == 0 {
		return assignments, nil
	}
	matched := make(map[string]bool, len(slugs))
	opts := &buildkite.TeamsListOptions{ListOptions: buildkite.ListOptions{Page: 1, PerPage: 100}}
	for {
		results, resp, err := client.Teams.List(ctx, org, opts)
		if err != nil {
			return nil, fmt.Errorf("could not resolve team slugs in organization %q: %w", org, err)
		}
		for _, team := range results {
			access, requested := slugs[team.Slug]
			if !requested {
				continue
			}
			matched[team.Slug] = true
			assignments[team.ID] = access
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	for slug := range slugs {
		if !matched[slug] {
			return nil, fmt.Errorf("team slug %q not found in organization %q; use the exact team slug from bk team list, or pass the team UUID", slug, org)
		}
	}
	return assignments, nil
}

func (c *CopyCmd) resolveTeams(ctx context.Context, f *factory.Factory, sourceOrg, targetOrg string) (map[string]string, error) {
	if len(c.Teams) == 0 {
		return nil, nil
	}
	client := f.RestAPIClient
	if targetOrg != sourceOrg {
		var err error
		client, err = c.getClientForOrg(f, targetOrg)
		if err != nil {
			return nil, err
		}
	}
	return resolveTeamAssignments(ctx, client, targetOrg, c.Teams)
}
