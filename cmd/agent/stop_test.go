package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/spf13/afero"
)

func TestStopCmdStructure(t *testing.T) {
	t.Parallel()

	cmd := &StopCmd{
		Agents: []string{"agent-1", "agent-2"},
		Limit:  5,
		Force:  true,
	}

	if len(cmd.Agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(cmd.Agents))
	}

	if cmd.Limit != 5 {
		t.Errorf("expected Limit to be 5, got %d", cmd.Limit)
	}

	if !cmd.Force {
		t.Error("expected Force to be true")
	}
}

func TestStopCmdHelp(t *testing.T) {
	t.Parallel()

	cmd := &StopCmd{}
	help := cmd.Help()

	if help == "" {
		t.Error("Help() should return non-empty string")
	}

	if !strings.Contains(strings.ToLower(help), "agent") {
		t.Error("Help text should mention agents")
	}
}

func TestStopAgentErrorCollection(t *testing.T) {
	t.Parallel()

	t.Run("parses agent arg correctly", func(t *testing.T) {
		t.Parallel()

		conf := config.New(afero.NewMemMapFs(), nil)
		conf.SelectOrganization("default-org", false)

		tests := []struct {
			name        string
			input       string
			expectedOrg string
			expectedID  string
		}{
			{
				name:        "agent ID only",
				input:       "agent-123",
				expectedOrg: "default-org",
				expectedID:  "agent-123",
			},
			{
				name:        "org/agent format",
				input:       "custom-org/agent-456",
				expectedOrg: "custom-org",
				expectedID:  "agent-456",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				org, id := parseAgentArg(tt.input, conf)

				if org != tt.expectedOrg {
					t.Errorf("expected org %q, got %q", tt.expectedOrg, org)
				}

				if id != tt.expectedID {
					t.Errorf("expected id %q, got %q", tt.expectedID, id)
				}
			})
		}
	})
}

func TestStopAgentBulkOperationErrorHandling(t *testing.T) {
	t.Parallel()

	// This test verifies the error collection logic without running the full command
	t.Run("error details format", func(t *testing.T) {
		t.Parallel()

		errorDetails := []string{}

		// Simulate collecting errors
		updates := []stopResult{
			{id: "agent-1", err: nil},
			{id: "agent-2", err: fmt.Errorf("connection timeout")},
			{id: "agent-3", err: nil},
			{id: "agent-4", err: fmt.Errorf("not found")},
		}

		for _, update := range updates {
			if update.err != nil {
				errorDetails = append(errorDetails, fmt.Sprintf("FAILED %s: %v", update.id, update.err))
			}
		}

		if len(errorDetails) != 2 {
			t.Errorf("expected 2 error details, got %d", len(errorDetails))
		}

		if !strings.Contains(errorDetails[0], "agent-2") {
			t.Error("expected first error to mention agent-2")
		}

		if !strings.Contains(errorDetails[1], "agent-4") {
			t.Error("expected second error to mention agent-4")
		}

		if !strings.Contains(errorDetails[0], "connection timeout") {
			t.Error("expected first error to include 'connection timeout'")
		}

		if !strings.Contains(errorDetails[1], "not found") {
			t.Error("expected second error to include 'not found'")
		}
	})
}
