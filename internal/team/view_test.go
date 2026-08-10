package team

import (
	"strings"
	"testing"
	"time"

	buildkite "github.com/buildkite/go-buildkite/v5"
)

func TestTeamViewTable(t *testing.T) {
	t.Parallel()

	t.Run("empty slice returns no teams message", func(t *testing.T) {
		t.Parallel()

		result := TeamViewTable()
		if result != "No teams found." {
			t.Errorf("expected 'No teams found.', got %q", result)
		}
	})

	t.Run("single team renders detail view", func(t *testing.T) {
		t.Parallel()

		team := buildkite.Team{
			ID:          "team-uuid-123",
			Name:        "Frontend",
			Slug:        "frontend",
			Description: "The frontend team",
			Privacy:     "visible",
			Default:     true,
		}

		result := TeamViewTable(team)

		for _, expected := range []string{"Frontend", "frontend", "The frontend team", "visible", "true", "team-uuid-123"} {
			if !strings.Contains(result, expected) {
				t.Errorf("expected output to contain %q, got:\n%s", expected, result)
			}
		}
	})

	t.Run("multiple teams renders summary table", func(t *testing.T) {
		t.Parallel()

		teams := []buildkite.Team{
			{ID: "team-1", Name: "Frontend", Slug: "frontend", Privacy: "visible", Default: false},
			{ID: "team-2", Name: "Backend", Slug: "backend", Privacy: "secret", Default: true},
		}

		result := TeamViewTable(teams...)

		for _, expected := range []string{
			"NAME", "SLUG", "ID", "PRIVACY", "DEFAULT",
			"Frontend", "Backend", "team-1", "team-2",
		} {
			if !strings.Contains(result, expected) {
				t.Errorf("expected output to contain %q, got:\n%s", expected, result)
			}
		}
	})
}

func TestRenderTeamText(t *testing.T) {
	t.Parallel()

	createdAt := buildkite.Timestamp{Time: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)}

	team := buildkite.Team{
		ID:                         "team-uuid-123",
		Name:                       "Fearless Frontenders",
		Slug:                       "fearless-frontenders",
		Description:                "Frontend engineers",
		Privacy:                    "secret",
		Default:                    true,
		DefaultMemberRole:          "maintainer",
		MembersCanCreatePipelines:  true,
		MembersCanCreateSuites:     false,
		MembersCanCreateRegistries: true,
		CreatedAt:                  &createdAt,
		CreatedBy: &buildkite.User{
			ID:    "user-1",
			Name:  "Peter Pettigrew",
			Email: "pp@hogwarts.co.uk",
		},
	}

	result := RenderTeamText(team)

	for label, value := range map[string]string{
		"Members Can Create Pipelines":   "true",
		"Members Can Create Test Suites": "false",
		"Members Can Create Registries":  "true",
	} {
		matched := false
		for _, line := range strings.Split(result, "\n") {
			if strings.Contains(line, label) && strings.HasSuffix(strings.TrimSpace(line), value) {
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("expected output row %q with value %q, got:\n%s", label, value, result)
		}
	}

	for _, expected := range []string{
		"Fearless Frontenders",
		"fearless-frontenders",
		"Frontend engineers",
		"secret",
		"true",
		"Default Member Role",
		"maintainer",
		"team-uuid-123",
		"Peter Pettigrew",
		"pp@hogwarts.co.uk",
		"user-1",
		"2024-01-15T10:30:00Z",
	} {
		if !strings.Contains(result, expected) {
			t.Errorf("expected output to contain %q, got:\n%s", expected, result)
		}
	}
}

func TestRenderTeamText_NoCreatedBy(t *testing.T) {
	t.Parallel()

	team := buildkite.Team{
		ID:      "team-uuid-123",
		Name:    "Minimal Team",
		Slug:    "minimal-team",
		Privacy: "visible",
	}

	result := RenderTeamText(team)

	if !strings.Contains(result, "Minimal Team") {
		t.Errorf("expected output to contain 'Minimal Team', got:\n%s", result)
	}
	// CreatedBy fields should be absent when not set
	if strings.Contains(result, "Created By") {
		t.Errorf("expected no 'Created By' fields when CreatedBy is nil, got:\n%s", result)
	}
}
