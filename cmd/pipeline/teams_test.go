package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Khan/genqlient/graphql"
	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v5"
)

const (
	readerTeam = "14e9501c-69fe-4cda-ae07-daea9ca3afd3"
	ownerTeam  = "3f195bcd-28f2-4e1a-bcff-09f3543e5abf"
)

func TestTeamFlags(t *testing.T) {
	for _, command := range []string{"create", "cp"} {
		for _, tc := range []struct {
			name    string
			args    []string
			wantErr bool
		}{
			{"multiple assignments", []string{"--team", readerTeam + "=read_only", "--team", ownerTeam + "=manage_build_and_read"}, false},
			{"invalid UUID", []string{"--team", "my-team=read_only"}, true},
			{"invalid access", []string{"--team", readerTeam + "=admin"}, true},
			{"missing access", []string{"--team", readerTeam}, true},
		} {
			t.Run(command+"/"+tc.name, func(t *testing.T) {
				var cli struct {
					Create CreateCmd `cmd:""`
					Cp     CopyCmd   `cmd:""`
				}
				parser, err := kong.New(&cli, kong.Vars{"output_default_format": ""})
				if err != nil {
					t.Fatal(err)
				}
				_, err = parser.Parse(append([]string{command, "pipeline"}, tc.args...))
				if (err != nil) != tc.wantErr {
					t.Fatalf("error = %v, wantErr %v", err, tc.wantErr)
				}
				if tc.wantErr {
					return
				}
				teams := cli.Create.Teams
				if command == "cp" {
					teams = cli.Cp.Teams
				}
				if !maps.Equal(teams, map[string]string{readerTeam: "read_only", ownerTeam: "manage_build_and_read"}) {
					t.Fatalf("unexpected assignments: %v", teams)
				}
			})
		}
	}
}

func TestCopyTeamsPaginationAndCreation(t *testing.T) {
	want := map[string]string{readerTeam: "read_only", ownerTeam: "manage_build_and_read"}
	pages, posts := 0, 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/graphql" {
			var req struct {
				Variables struct {
					Slug  string
					After *string
				}
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Error(err)
			}
			if req.Variables.Slug != "source-org/source" {
				t.Errorf("wrong source: %s", req.Variables.Slug)
			}
			pages++
			if pages == 1 {
				if req.Variables.After != nil {
					t.Error("first page should not have a cursor")
				}
				fmt.Fprintf(w, `{"data":{"pipeline":{"teams":{"edges":[{"node":{"accessLevel":"READ_ONLY","team":{"uuid":%q}}}],"pageInfo":{"hasNextPage":true,"endCursor":"next"}}}}}`, readerTeam)
			} else {
				if req.Variables.After == nil || *req.Variables.After != "next" {
					t.Error("missing next-page cursor")
				}
				fmt.Fprintf(w, `{"data":{"pipeline":{"teams":{"edges":[{"node":{"accessLevel":"MANAGE_BUILD_AND_READ","team":{"uuid":%q}}}],"pageInfo":{"hasNextPage":false}}}}}`, ownerTeam)
			}
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/v2/organizations/source-org/pipelines" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		posts++
		var body struct {
			Teams map[string]string `json:"teams"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if !maps.Equal(body.Teams, want) {
			t.Errorf("creation must include all teams with original access: %v", body.Teams)
			w.WriteHeader(http.StatusUnprocessableEntity)
		}
		fmt.Fprint(w, `{"name":"copy"}`)
	}))
	defer s.Close()
	client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
	if err != nil {
		t.Fatal(err)
	}
	f := &factory.Factory{Config: &config.Config{}, RestAPIClient: client, GraphQLClient: graphql.NewClient(s.URL+"/graphql", s.Client())}
	c := CopyCmd{OutputFlags: output.OutputFlags{Output: "json"}}
	request := c.buildCreatePipeline(&buildkite.Pipeline{Repository: "git@example.com:repo.git", Configuration: "steps: []"}, "copy", false, "cluster")
	request.Teams, err = c.resolveTeams(context.Background(), f, "source-org", "source", false)
	if err != nil {
		t.Fatal(err)
	}
	var preview bytes.Buffer
	parser, err := kong.New(&c, kong.Writers(&preview, &preview), kong.Vars{"output_default_format": "json"})
	if err != nil {
		t.Fatal(err)
	}
	kongCtx, err := parser.Parse(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.runDryRun(kongCtx, f, request); err != nil {
		t.Fatal(err)
	}
	var dry struct {
		Teams map[string]string `json:"teams"`
	}
	if err := json.Unmarshal(preview.Bytes(), &dry); err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(dry.Teams, want) || posts != 0 {
		t.Fatalf("dry-run teams %v, writes %d", dry.Teams, posts)
	}
	if err := c.runCopy(kongCtx, f, &copyTarget{Org: "source-org", Name: "copy"}, false, request); err != nil {
		t.Fatal(err)
	}
	if pages != 2 || posts != 1 {
		t.Fatalf("pages=%d posts=%d", pages, posts)
	}
}

func TestCopyTeamOverridesAndCrossOrg(t *testing.T) {
	for _, crossOrg := range []bool{false, true} {
		c := CopyCmd{Teams: map[string]string{readerTeam: "build_and_read"}}
		// No GraphQL client: explicit teams must bypass source lookups.
		teams, err := c.resolveTeams(context.Background(), &factory.Factory{}, "org", "pipeline", crossOrg)
		if err != nil || !maps.Equal(teams, c.Teams) {
			t.Fatalf("teams=%v err=%v", teams, err)
		}
	}
	c := CopyCmd{}
	teams, err := c.resolveTeams(context.Background(), &factory.Factory{}, "org", "pipeline", true)
	if err != nil || len(teams) != 0 {
		t.Fatalf("cross-org copy inherited teams: %v, %v", teams, err)
	}
}

func TestCopyTeamLookupFailures(t *testing.T) {
	for _, response := range []string{
		`{"errors":[{"message":"Forbidden"}]}`,
		`{"data":{"pipeline":null}}`,
		`{"data":{"pipeline":{"teams":{"edges":[{"node":{"team":null}}]}}}}`,
	} {
		t.Run(response, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, response)
			}))
			defer s.Close()
			c := CopyCmd{}
			teams, err := c.resolveTeams(context.Background(), &factory.Factory{GraphQLClient: graphql.NewClient(s.URL, s.Client())}, "org", "pipeline", false)
			if err == nil || !strings.Contains(err.Error(), "--team") || teams != nil {
				t.Fatalf("teams=%v err=%v", teams, err)
			}
		})
	}
}

func TestCreateTeamsRequestAndDryRun(t *testing.T) {
	want := map[string]string{readerTeam: "build_and_read"}
	posts := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{}`)
			return
		}
		posts++
		var body struct {
			Teams map[string]string `json:"teams"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if !maps.Equal(body.Teams, want) {
			t.Errorf("teams missing from creation: %v", body.Teams)
		}
		fmt.Fprint(w, `{"name":"new"}`)
	}))
	defer s.Close()
	client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
	if err != nil {
		t.Fatal(err)
	}
	f := &factory.Factory{RestAPIClient: client}
	c := CreateCmd{Name: "new", Org: "org", Repository: "git@example.com:repo.git", Teams: want}
	preview, err := c.createPipelineDryRun(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(preview.Teams, want) || posts != 0 {
		t.Fatalf("dry-run teams=%v writes=%d", preview.Teams, posts)
	}
	if _, err := c.createPipeline(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if posts != 1 {
		t.Fatalf("creation writes=%d", posts)
	}
}
