package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Khan/genqlient/graphql"
	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
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
			{"multiple assignments", []string{"--team", "readers=read_only", "--team", "platform-engineering=manage_build_and_read"}, false},
			{"empty slug", []string{"--team", " =read_only"}, true},
			{"invalid access", []string{"--team", "readers=admin"}, true},
			{"missing access", []string{"--team", "readers"}, true},
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
				if !maps.Equal(teams, map[string]string{"readers": "read_only", "platform-engineering": "manage_build_and_read"}) {
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
	request.Teams, err = c.resolveTeams(context.Background(), f, "source-org", "source", "source-org")
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
	for _, targetOrg := range []string{"org", "destination"} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || r.URL.Path != "/v2/organizations/"+targetOrg+"/teams" {
				t.Errorf("wrong lookup: %s %s", r.Method, r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `[{"id":%q,"name":"Platform Engineering","slug":"platform-engineering"}]`, ownerTeam)
		}))
		t.Cleanup(s.Close)
		t.Setenv("BUILDKITE_REST_API_ENDPOINT", s.URL)
		t.Setenv("BUILDKITE_API_TOKEN", "test-token")
		client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}
		c := CopyCmd{Teams: map[string]string{"platform-engineering": "build_and_read"}}
		// No GraphQL client: explicit teams must bypass source lookups.
		teams, err := c.resolveTeams(context.Background(), &factory.Factory{RestAPIClient: client, Config: &config.Config{}}, "org", "pipeline", targetOrg)
		if err != nil || !maps.Equal(teams, map[string]string{ownerTeam: "build_and_read"}) {
			t.Fatalf("teams=%v err=%v", teams, err)
		}
	}
	c := CopyCmd{}
	teams, err := c.resolveTeams(context.Background(), &factory.Factory{}, "org", "pipeline", "destination")
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
			teams, err := c.resolveTeams(context.Background(), &factory.Factory{GraphQLClient: graphql.NewClient(s.URL, s.Client())}, "org", "pipeline", "org")
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
		if r.Method == http.MethodGet && r.URL.Path == "/v2/organizations/org/teams" {
			fmt.Fprintf(w, `[{"id":%q,"name":"Readers","slug":"readers"}]`, readerTeam)
			return
		}
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
	c := CreateCmd{Name: "new", Org: "org", Repository: "git@example.com:repo.git", Teams: map[string]string{"readers": "build_and_read"}}
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

func TestResolveTeamSlugs(t *testing.T) {
	for _, tc := range []struct {
		name       string
		slugs      map[string]string
		secondName string
		status     int
		wantErr    string
	}{
		{"paginated slugs", map[string]string{"readers": "read_only", "platform-engineering": "manage_build_and_read"}, "Platform Engineering", 200, ""},
		{"same names with distinct slugs", map[string]string{"readers": "read_only", "platform-engineering": "manage_build_and_read"}, "Readers", 200, ""},
		{"display name is not a slug", map[string]string{"Readers": "read_only"}, "Platform Engineering", 200, "not found"},
		{"UUID is not a slug", map[string]string{readerTeam: "read_only"}, "Platform Engineering", 200, "not found"},
		{"no team read permission", map[string]string{"readers": "read_only"}, "", 403, "could not resolve"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodGet || r.URL.Path != "/v2/organizations/destination/teams" {
					t.Errorf("unexpected lookup: %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				if tc.status != 200 {
					w.WriteHeader(tc.status)
					fmt.Fprint(w, `{"message":"Forbidden"}`)
					return
				}
				if calls == 1 {
					w.Header().Set("Link", fmt.Sprintf(`<http://%s/v2/organizations/destination/teams?page=2>; rel="next"`, r.Host))
					fmt.Fprintf(w, `[{"id":%q,"name":"Readers","slug":"readers"}]`, readerTeam)
				} else {
					if r.URL.Query().Get("page") != "2" {
						t.Error("missing page 2")
					}
					fmt.Fprintf(w, `[{"id":%q,"name":%q,"slug":"platform-engineering"}]`, ownerTeam, tc.secondName)
				}
			}))
			defer s.Close()
			client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
			if err != nil {
				t.Fatal(err)
			}
			teams, err := resolveTeamSlugs(context.Background(), client, "destination", tc.slugs)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) || teams != nil {
					t.Fatalf("teams=%v err=%v", teams, err)
				}
				return
			}
			if err != nil || !maps.Equal(teams, map[string]string{readerTeam: "read_only", ownerTeam: "manage_build_and_read"}) || calls != 2 {
				t.Fatalf("teams=%v calls=%d err=%v", teams, calls, err)
			}
		})
	}
	if teams, err := resolveTeamSlugs(context.Background(), nil, "org", nil); err != nil || teams != nil {
		t.Fatalf("empty assignment should not perform a lookup: %v, %v", teams, err)
	}
}

func TestCreateWithoutTeamsReturnsValidationError(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("BUILDKITE_API_TOKEN", "test-token")
	posts, lists := 0, 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/v2/organizations/org/pipelines" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		switch r.Method {
		case http.MethodPost:
			posts++
			var body map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if _, present := body["teams"]; present {
				t.Error("creation without --team must omit teams")
			}
			// Model a non-admin creation rejected by a Teams-enabled organization.
			w.WriteHeader(http.StatusUnprocessableEntity)
			fmt.Fprint(w, `{"message":"Team assignments are required"}`)
		case http.MethodGet:
			lists++
			// The duplicate-name check must not mask the original validation error.
			fmt.Fprint(w, `[]`)
		default:
			t.Errorf("unexpected method: %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer s.Close()
	t.Setenv("BUILDKITE_REST_API_ENDPOINT", s.URL)
	t.Setenv("BUILDKITE_GRAPHQL_ENDPOINT", s.URL+"/graphql")
	var c CreateCmd
	var stdout bytes.Buffer
	parser, err := kong.New(&c, kong.Writers(&stdout, &stdout), kong.Vars{"output_default_format": "json"})
	if err != nil {
		t.Fatal(err)
	}
	kongCtx, err := parser.Parse([]string{"new", "--org", "org", "--repository", "git@example.com:repo.git", "--cluster-uuid", "cluster"})
	if err != nil {
		t.Fatal(err)
	}
	err = c.Run(kongCtx, cli.Globals{NoInput: true, Quiet: true})
	var apiErr *buildkite.ErrorResponse
	if !errors.As(err, &apiErr) || apiErr.Response.StatusCode != http.StatusUnprocessableEntity || apiErr.Message != "Team assignments are required" {
		t.Fatalf("expected original missing-teams 422, got %v", err)
	}
	if posts != 1 || lists != 1 || stdout.Len() != 0 {
		t.Fatalf("posts=%d lists=%d output=%q", posts, lists, stdout.String())
	}
}

func TestCopySourceWithoutTeams(t *testing.T) {
	for _, dryRun := range []bool{true, false} {
		t.Run(fmt.Sprintf("dry-run=%t", dryRun), func(t *testing.T) {
			t.Chdir(t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("BUILDKITE_API_TOKEN", "test-token")
			lookups, posts := 0, 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/v2/organizations/org/pipelines/source":
					fmt.Fprint(w, `{"name":"Source","slug":"source","repository":"git@example.com:repo.git","configuration":"steps: []","cluster_id":"cluster"}`)
				case r.Method == http.MethodPost && r.URL.Path == "/graphql":
					lookups++
					fmt.Fprint(w, `{"data":{"pipeline":{"teams":{"edges":[],"pageInfo":{"hasNextPage":false}}}}}`)
				case r.Method == http.MethodPost && r.URL.Path == "/v2/organizations/org/pipelines":
					posts++
					var body map[string]json.RawMessage
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if _, present := body["teams"]; present {
						t.Error("copy of a source without teams must omit teams")
					}
					if string(body["name"]) != `"copy"` || string(body["cluster_id"]) != `"cluster"` {
						t.Errorf("unexpected copy payload: %v", body)
					}
					// Model an authorized caller that may create without team assignments.
					w.WriteHeader(http.StatusCreated)
					fmt.Fprint(w, `{"name":"copy","cluster_id":"cluster"}`)
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer s.Close()
			t.Setenv("BUILDKITE_REST_API_ENDPOINT", s.URL)
			t.Setenv("BUILDKITE_GRAPHQL_ENDPOINT", s.URL+"/graphql")
			var c CopyCmd
			var stdout bytes.Buffer
			parser, err := kong.New(&c, kong.Writers(&stdout, &stdout), kong.Vars{"output_default_format": "json"})
			if err != nil {
				t.Fatal(err)
			}
			args := []string{"org/source", "--org", "org", "--target", "org/copy"}
			if dryRun {
				args = append(args, "--dry-run")
			}
			kongCtx, err := parser.Parse(args)
			if err != nil {
				t.Fatal(err)
			}
			if err := c.Run(kongCtx, cli.Globals{NoInput: true, Quiet: true}); err != nil {
				t.Fatal(err)
			}
			var result map[string]json.RawMessage
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if _, present := result["teams"]; present {
				t.Error("output should not invent team assignments")
			}
			if string(result["name"]) != `"copy"` || string(result["cluster_id"]) != `"cluster"` {
				t.Fatalf("unexpected output: %s", stdout.String())
			}
			wantPosts := 1
			if dryRun {
				wantPosts = 0
			}
			if lookups != 1 || posts != wantPosts {
				t.Fatalf("lookups=%d posts=%d, want 1 and %d", lookups, posts, wantPosts)
			}
		})
	}
}
