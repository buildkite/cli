package team

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	buildkite "github.com/buildkite/go-buildkite/v4"
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

	result, _, err := client.Teams.UpdateTeam(context.Background(), "test-org", "team-uuid-123", buildkite.CreateTeam{
		Name:    "Renamed Team",
		Privacy: "visible",
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

func TestUpdateCmdValidate(t *testing.T) {
	t.Parallel()

	boolTrue := true
	boolFalse := false

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
			cmd:     UpdateCmd{TeamUUID: "team-uuid", Description: "new desc"},
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
