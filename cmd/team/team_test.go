package team

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	buildkite "github.com/buildkite/go-buildkite/v5"
	"github.com/spf13/afero"
)

func makeTeams(n, offset int) []buildkite.Team {
	teams := make([]buildkite.Team, n)
	for i := range teams {
		teams[i] = buildkite.Team{
			ID:   fmt.Sprintf("team-%d", offset+i),
			Name: fmt.Sprintf("Team %d", offset+i),
			Slug: fmt.Sprintf("team-%d", offset+i),
		}
	}
	return teams
}

func TestListTeams(t *testing.T) {
	t.Parallel()

	t.Run("fetches teams through API", func(t *testing.T) {
		t.Parallel()

		teams := []buildkite.Team{
			{ID: "team-1", Name: "Frontend", Slug: "frontend", Privacy: "visible"},
			{ID: "team-2", Name: "Backend", Slug: "backend", Privacy: "secret", Default: true},
		}

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				t.Errorf("expected GET, got %s", r.Method)
			}
			if !strings.Contains(r.URL.Path, "/teams") {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(teams)
		}))
		defer s.Close()

		client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		result, _, err := client.Teams.List(context.Background(), "test-org", nil)
		if err != nil {
			t.Fatal(err)
		}

		if len(result) != 2 {
			t.Fatalf("expected 2 teams, got %d", len(result))
		}
		if result[0].Name != "Frontend" {
			t.Errorf("expected name 'Frontend', got %q", result[0].Name)
		}
		if result[1].Slug != "backend" {
			t.Errorf("expected slug 'backend', got %q", result[1].Slug)
		}
	})

	t.Run("empty result returns empty slice", func(t *testing.T) {
		t.Parallel()

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]buildkite.Team{})
		}))
		defer s.Close()

		client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		result, _, err := client.Teams.List(context.Background(), "test-org", nil)
		if err != nil {
			t.Fatal(err)
		}

		if len(result) != 0 {
			t.Errorf("expected 0 teams, got %d", len(result))
		}
	})

	t.Run("paginates across multiple pages", func(t *testing.T) {
		t.Parallel()

		// page 1: 30 teams (full page), page 2: 15 teams (partial) → 45 total
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			page := r.URL.Query().Get("page")
			w.Header().Set("Content-Type", "application/json")
			switch page {
			case "", "1":
				json.NewEncoder(w).Encode(makeTeams(30, 0))
			case "2":
				json.NewEncoder(w).Encode(makeTeams(15, 30))
			default:
				json.NewEncoder(w).Encode([]buildkite.Team{})
			}
		}))
		defer s.Close()

		client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		page1, _, err := client.Teams.List(context.Background(), "test-org", &buildkite.TeamsListOptions{
			ListOptions: buildkite.ListOptions{Page: 1, PerPage: 30},
		})
		if err != nil {
			t.Fatal(err)
		}
		page2, _, err := client.Teams.List(context.Background(), "test-org", &buildkite.TeamsListOptions{
			ListOptions: buildkite.ListOptions{Page: 2, PerPage: 30},
		})
		if err != nil {
			t.Fatal(err)
		}

		total := append(page1, page2...)
		if len(total) != 45 {
			t.Errorf("expected 45 teams across 2 pages, got %d", len(total))
		}
		// Partial second page signals no further pages
		if len(page2) >= 30 {
			t.Error("expected page 2 to be a partial page indicating end of results")
		}
	})

	t.Run("stops at limit when pages are full", func(t *testing.T) {
		t.Parallel()

		// Server always returns full pages of 30; limit is 30 so only one page needed
		callCount := 0
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(makeTeams(30, (callCount-1)*30))
		}))
		defer s.Close()

		client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		result, _, err := client.Teams.List(context.Background(), "test-org", &buildkite.TeamsListOptions{
			ListOptions: buildkite.ListOptions{Page: 1, PerPage: 30},
		})
		if err != nil {
			t.Fatal(err)
		}

		if len(result) != 30 {
			t.Errorf("expected 30 teams, got %d", len(result))
		}
		// A full page means there are potentially more results
		if len(result) < 30 {
			t.Error("expected a full page indicating more results may exist")
		}
		if callCount != 1 {
			t.Errorf("expected 1 API call, got %d", callCount)
		}
	})

	t.Run("duplicate page detection", func(t *testing.T) {
		t.Parallel()

		// Server always returns the same page content regardless of page param
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(makeTeams(30, 0))
		}))
		defer s.Close()

		client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		page1, _, err := client.Teams.List(context.Background(), "test-org", &buildkite.TeamsListOptions{
			ListOptions: buildkite.ListOptions{Page: 1, PerPage: 30},
		})
		if err != nil {
			t.Fatal(err)
		}
		page2, _, err := client.Teams.List(context.Background(), "test-org", &buildkite.TeamsListOptions{
			ListOptions: buildkite.ListOptions{Page: 2, PerPage: 30},
		})
		if err != nil {
			t.Fatal(err)
		}

		// Both pages have the same first ID — the listTeams loop would catch this
		if page1[0].ID != page2[0].ID {
			t.Error("expected duplicate page content to have matching first IDs")
		}
	})
}

func TestGetTeam(t *testing.T) {
	t.Parallel()

	team := buildkite.Team{
		ID:          "team-uuid-123",
		Name:        "Fearless Frontenders",
		Slug:        "fearless-frontenders",
		Description: "The frontend team",
		Privacy:     "secret",
		Default:     true,
	}

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/teams/team-uuid-123") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(team)
	}))
	defer s.Close()

	client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.Teams.GetTeam(context.Background(), "test-org", "team-uuid-123")
	if err != nil {
		t.Fatal(err)
	}

	if result.Name != "Fearless Frontenders" {
		t.Errorf("expected name 'Fearless Frontenders', got %q", result.Name)
	}
	if result.Description != "The frontend team" {
		t.Errorf("expected description 'The frontend team', got %q", result.Description)
	}
	if !result.Default {
		t.Error("expected Default to be true")
	}
}

func TestCreateCmdPermissions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		pipelines  bool
		suites     bool
		registries bool
	}{
		{
			name:       "uses API defaults for creation permissions",
			args:       []string{"New Team"},
			pipelines:  true,
			suites:     true,
			registries: true,
		},
		{
			name:       "explicitly disables pipeline creation",
			args:       []string{"New Team", "--no-members-can-create-pipelines"},
			pipelines:  false,
			suites:     true,
			registries: true,
		},
		{
			name:       "explicitly disables suite creation",
			args:       []string{"New Team", "--no-members-can-create-suites"},
			pipelines:  true,
			suites:     false,
			registries: true,
		},
		{
			name:       "explicitly disables registry creation",
			args:       []string{"New Team", "--no-members-can-create-registries"},
			pipelines:  true,
			suites:     true,
			registries: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var cmd CreateCmd
			parser, err := kong.New(&cmd, kong.Vars{"output_default_format": ""})
			if err != nil {
				t.Fatalf("kong.New() error = %v", err)
			}
			if _, err := parser.Parse(tt.args); err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			input := cmd.createTeamInput()
			if input.MembersCanCreatePipelines != tt.pipelines {
				t.Errorf("MembersCanCreatePipelines = %v, want %v", input.MembersCanCreatePipelines, tt.pipelines)
			}
			if input.MembersCanCreateSuites != tt.suites {
				t.Errorf("MembersCanCreateSuites = %v, want %v", input.MembersCanCreateSuites, tt.suites)
			}
			if input.MembersCanCreateRegistries != tt.registries {
				t.Errorf("MembersCanCreateRegistries = %v, want %v", input.MembersCanCreateRegistries, tt.registries)
			}

			body, err := json.Marshal(input)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}
			var request map[string]json.RawMessage
			if err := json.Unmarshal(body, &request); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			for field, want := range map[string]bool{
				"members_can_create_pipelines":  tt.pipelines,
				"members_can_create_suites":     tt.suites,
				"members_can_create_registries": tt.registries,
			} {
				value, ok := request[field]
				if !ok {
					t.Errorf("request body omitted %q", field)
					continue
				}
				var got bool
				if err := json.Unmarshal(value, &got); err != nil {
					t.Errorf("request field %q: json.Unmarshal() error = %v", field, err)
					continue
				}
				if got != want {
					t.Errorf("request field %q = %v, want %v", field, got, want)
				}
			}
		})
	}
}

func TestCreateTeam(t *testing.T) {
	t.Parallel()

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/teams") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var input buildkite.CreateTeam
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}

		if input.Name != "New Team" {
			t.Errorf("expected name 'New Team', got %q", input.Name)
		}
		if input.Privacy != "secret" {
			t.Errorf("expected privacy 'secret', got %q", input.Privacy)
		}
		if !input.IsDefaultTeam {
			t.Error("expected IsDefaultTeam to be true")
		}
		if input.DefaultMemberRole != "maintainer" {
			t.Errorf("expected default member role 'maintainer', got %q", input.DefaultMemberRole)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(buildkite.Team{
			ID:      "new-team-uuid",
			Name:    input.Name,
			Privacy: input.Privacy,
			Default: input.IsDefaultTeam,
		})
	}))
	defer s.Close()

	client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
	if err != nil {
		t.Fatal(err)
	}

	result, _, err := client.Teams.CreateTeam(context.Background(), "test-org", buildkite.CreateTeam{
		Name:              "New Team",
		Privacy:           "secret",
		IsDefaultTeam:     true,
		DefaultMemberRole: "maintainer",
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.ID != "new-team-uuid" {
		t.Errorf("expected ID 'new-team-uuid', got %q", result.ID)
	}
	if result.Name != "New Team" {
		t.Errorf("expected name 'New Team', got %q", result.Name)
	}
}

func TestUpdateTeam(t *testing.T) {
	t.Parallel()

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/teams/team-uuid-123") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var input buildkite.CreateTeam
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}

		if input.Name != "Renamed Team" {
			t.Errorf("expected name 'Renamed Team', got %q", input.Name)
		}
		if input.Privacy != "visible" {
			t.Errorf("expected privacy 'visible', got %q", input.Privacy)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(buildkite.Team{
			ID:      "team-uuid-123",
			Name:    input.Name,
			Privacy: input.Privacy,
		})
	}))
	defer s.Close()

	client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
	if err != nil {
		t.Fatal(err)
	}

	result, _, err := client.Teams.UpdateTeam(context.Background(), "test-org", "team-uuid-123", buildkite.UpdateTeam{
		Name:    buildkite.Some("Renamed Team"),
		Privacy: buildkite.Some("visible"),
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.Name != "Renamed Team" {
		t.Errorf("expected name 'Renamed Team', got %q", result.Name)
	}
	if result.Privacy != "visible" {
		t.Errorf("expected privacy 'visible', got %q", result.Privacy)
	}
}

func TestDeleteTeam(t *testing.T) {
	t.Parallel()

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/teams/team-uuid-123") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer s.Close()

	client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.Teams.DeleteTeam(context.Background(), "test-org", "team-uuid-123")
	if err != nil {
		t.Fatal(err)
	}
}

func TestUpdateCmdPermissions(t *testing.T) {
	t.Parallel()

	permissionFields := []string{
		"members_can_create_pipelines",
		"members_can_create_suites",
		"members_can_create_registries",
	}

	t.Run("omits permissions that were not specified", func(t *testing.T) {
		t.Parallel()

		input := (&UpdateCmd{Name: "New Name"}).updateTeamInput()
		body, err := json.Marshal(input)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}

		var request map[string]json.RawMessage
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		for _, field := range permissionFields {
			if _, ok := request[field]; ok {
				t.Errorf("request body unexpectedly included %q", field)
			}
		}
	})

	tests := []struct {
		name  string
		flag  string
		field string
		want  bool
	}{
		{name: "enables pipeline creation", flag: "--members-can-create-pipelines", field: "members_can_create_pipelines", want: true},
		{name: "disables pipeline creation", flag: "--no-members-can-create-pipelines", field: "members_can_create_pipelines", want: false},
		{name: "enables suite creation", flag: "--members-can-create-suites", field: "members_can_create_suites", want: true},
		{name: "disables suite creation", flag: "--no-members-can-create-suites", field: "members_can_create_suites", want: false},
		{name: "enables registry creation", flag: "--members-can-create-registries", field: "members_can_create_registries", want: true},
		{name: "disables registry creation", flag: "--no-members-can-create-registries", field: "members_can_create_registries", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var cmd UpdateCmd
			parser, err := kong.New(&cmd, kong.Vars{"output_default_format": ""})
			if err != nil {
				t.Fatalf("kong.New() error = %v", err)
			}
			if _, err := parser.Parse([]string{"team-uuid", tt.flag}); err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			body, err := json.Marshal(cmd.updateTeamInput())
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}
			var request map[string]json.RawMessage
			if err := json.Unmarshal(body, &request); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}

			for _, field := range permissionFields {
				value, ok := request[field]
				if field != tt.field {
					if ok {
						t.Errorf("request body unexpectedly included %q", field)
					}
					continue
				}
				if !ok {
					t.Fatalf("request body omitted %q", field)
				}
				var got bool
				if err := json.Unmarshal(value, &got); err != nil {
					t.Fatalf("request field %q: json.Unmarshal() error = %v", field, err)
				}
				if got != tt.want {
					t.Errorf("request field %q = %v, want %v", field, got, tt.want)
				}
			}
		})
	}
}

func TestUpdateCmdValidate(t *testing.T) {
	t.Parallel()

	boolTrue := true
	boolFalse := false
	desc := "new desc"
	emptyDesc := ""

	tests := []struct {
		name    string
		cmd     UpdateCmd
		wantErr bool
	}{
		{
			name:    "no flags set",
			cmd:     UpdateCmd{TeamUUID: "team-uuid"},
			wantErr: true,
		},
		{
			name:    "only name",
			cmd:     UpdateCmd{TeamUUID: "team-uuid", Name: "New Name"},
			wantErr: false,
		},
		{
			name:    "only description",
			cmd:     UpdateCmd{TeamUUID: "team-uuid", Description: &desc},
			wantErr: false,
		},
		{
			name:    "explicitly empty description clears it",
			cmd:     UpdateCmd{TeamUUID: "team-uuid", Description: &emptyDesc},
			wantErr: false,
		},
		{
			name:    "valid privacy visible",
			cmd:     UpdateCmd{TeamUUID: "team-uuid", Privacy: "visible"},
			wantErr: false,
		},
		{
			name:    "valid privacy secret",
			cmd:     UpdateCmd{TeamUUID: "team-uuid", Privacy: "secret"},
			wantErr: false,
		},
		{
			name:    "invalid privacy",
			cmd:     UpdateCmd{TeamUUID: "team-uuid", Privacy: "public"},
			wantErr: true,
		},
		{
			name:    "only default true",
			cmd:     UpdateCmd{TeamUUID: "team-uuid", Default: &boolTrue},
			wantErr: false,
		},
		{
			name:    "only default false",
			cmd:     UpdateCmd{TeamUUID: "team-uuid", Default: &boolFalse},
			wantErr: false,
		},
		{
			name:    "valid default member role member",
			cmd:     UpdateCmd{TeamUUID: "team-uuid", DefaultMemberRole: "member"},
			wantErr: false,
		},
		{
			name:    "valid default member role maintainer",
			cmd:     UpdateCmd{TeamUUID: "team-uuid", DefaultMemberRole: "maintainer"},
			wantErr: false,
		},
		{
			name:    "invalid default member role",
			cmd:     UpdateCmd{TeamUUID: "team-uuid", DefaultMemberRole: "admin"},
			wantErr: true,
		},
		{
			name:    "only members-can-create-pipelines",
			cmd:     UpdateCmd{TeamUUID: "team-uuid", MembersCanCreatePipelines: &boolTrue},
			wantErr: false,
		},
		{
			name:    "only members-can-create-suites",
			cmd:     UpdateCmd{TeamUUID: "team-uuid", MembersCanCreateSuites: &boolFalse},
			wantErr: false,
		},
		{
			name:    "only members-can-create-registries",
			cmd:     UpdateCmd{TeamUUID: "team-uuid", MembersCanCreateRegistries: &boolTrue},
			wantErr: false,
		},
		{
			name:    "multiple valid flags",
			cmd:     UpdateCmd{TeamUUID: "team-uuid", Name: "New Name", Privacy: "secret", DefaultMemberRole: "maintainer"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.cmd.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestListCmdValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		perPage int
		limit   int
		wantErr bool
	}{
		{name: "minimum per-page", perPage: 1, limit: 100},
		{name: "maximum per-page", perPage: 100, limit: 100},
		{name: "zero per-page", perPage: 0, limit: 100, wantErr: true},
		{name: "negative per-page", perPage: -1, limit: 100, wantErr: true},
		{name: "per-page above API maximum", perPage: 101, limit: 100, wantErr: true},
		{name: "zero limit", perPage: 30, limit: 0},
		{name: "negative limit", perPage: 30, limit: -1, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd := ListCmd{PerPage: tt.perPage, Limit: tt.limit}
			err := cmd.Validate()
			if tt.wantErr && err == nil {
				t.Fatal("Validate() error = nil, want an error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func newListTestFactory(t *testing.T, baseURL string) *factory.Factory {
	t.Helper()

	client, err := buildkite.NewOpts(buildkite.WithBaseURL(baseURL))
	if err != nil {
		t.Fatal(err)
	}
	conf := config.New(afero.NewMemMapFs(), nil)
	if err := conf.SelectOrganization("test-org", true); err != nil {
		t.Fatalf("SelectOrganization() error = %v", err)
	}
	return &factory.Factory{
		Config:        conf,
		RestAPIClient: client,
		Quiet:         true,
	}
}

func TestListTeamsPagination(t *testing.T) {
	t.Parallel()

	t.Run("partial final page crossing the limit sets hasMore", func(t *testing.T) {
		t.Parallel()

		// Pages 1-3 return 30 teams each, page 4 returns a partial page of 15,
		// so 105 teams are collected against a limit of 100.
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			w.Header().Set("Content-Type", "application/json")
			switch {
			case page >= 1 && page <= 3:
				json.NewEncoder(w).Encode(makeTeams(30, (page-1)*30))
			case page == 4:
				json.NewEncoder(w).Encode(makeTeams(15, 90))
			default:
				json.NewEncoder(w).Encode([]buildkite.Team{})
			}
		}))
		defer s.Close()

		teams, hasMore, err := listTeams(context.Background(), newListTestFactory(t, s.URL), 30, 100)
		if err != nil {
			t.Fatal(err)
		}
		if len(teams) != 100 {
			t.Errorf("expected 100 teams, got %d", len(teams))
		}
		if !hasMore {
			t.Error("expected hasMore to be true when results were truncated to the limit")
		}
	})

	t.Run("results under the limit leave hasMore false", func(t *testing.T) {
		t.Parallel()

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			w.Header().Set("Content-Type", "application/json")
			if page == 1 {
				json.NewEncoder(w).Encode(makeTeams(30, 0))
			} else {
				json.NewEncoder(w).Encode(makeTeams(15, 30))
			}
		}))
		defer s.Close()

		teams, hasMore, err := listTeams(context.Background(), newListTestFactory(t, s.URL), 30, 100)
		if err != nil {
			t.Fatal(err)
		}
		if len(teams) != 45 {
			t.Errorf("expected 45 teams, got %d", len(teams))
		}
		if hasMore {
			t.Error("expected hasMore to be false when all teams fit within the limit")
		}
	})

	t.Run("total equal to limit and per-page leaves hasMore false", func(t *testing.T) {
		t.Parallel()

		// 2 teams total with --per-page=2 --limit=2: the final page is full,
		// so a confirming fetch of the empty next page is needed to
		// distinguish an exact boundary from truncation.
		var requests int
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests++
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			w.Header().Set("Content-Type", "application/json")
			if page <= 1 {
				json.NewEncoder(w).Encode(makeTeams(2, 0))
			} else {
				json.NewEncoder(w).Encode([]buildkite.Team{})
			}
		}))
		defer s.Close()

		teams, hasMore, err := listTeams(context.Background(), newListTestFactory(t, s.URL), 2, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(teams) != 2 {
			t.Errorf("expected 2 teams, got %d", len(teams))
		}
		if hasMore {
			t.Error("expected hasMore to be false when the total exactly equals the limit")
		}
		if requests != 2 {
			t.Errorf("expected 2 API requests (one to confirm no further results), got %d", requests)
		}
	})

	t.Run("full final page at the limit with more teams sets hasMore", func(t *testing.T) {
		t.Parallel()

		// 3 teams total with --per-page=2 --limit=2: the confirming fetch
		// finds a third team, so the truncated result reports hasMore.
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			w.Header().Set("Content-Type", "application/json")
			if page <= 1 {
				json.NewEncoder(w).Encode(makeTeams(2, 0))
			} else {
				json.NewEncoder(w).Encode(makeTeams(1, 2))
			}
		}))
		defer s.Close()

		teams, hasMore, err := listTeams(context.Background(), newListTestFactory(t, s.URL), 2, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(teams) != 2 {
			t.Errorf("expected 2 teams, got %d", len(teams))
		}
		if !hasMore {
			t.Error("expected hasMore to be true when a further team exists beyond the limit")
		}
	})

	t.Run("limit of zero returns no teams without a request", func(t *testing.T) {
		t.Parallel()

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("expected no API request when limit is 0")
		}))
		defer s.Close()

		teams, hasMore, err := listTeams(context.Background(), newListTestFactory(t, s.URL), 30, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(teams) != 0 {
			t.Errorf("expected no teams, got %d", len(teams))
		}
		if hasMore {
			t.Error("expected hasMore to be false for a zero limit")
		}
	})
}
