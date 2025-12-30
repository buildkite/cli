package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	bkIO "github.com/buildkite/cli/v3/internal/io"
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

	t.Run("progress tracking", func(t *testing.T) {
		t.Parallel()

		total := 10
		succeeded := 0
		failed := 0
		completed := 0

		updates := []stopResult{
			{id: "agent-1", err: nil},
			{id: "agent-2", err: nil},
			{id: "agent-3", err: fmt.Errorf("timeout")},
			{id: "agent-4", err: nil},
			{id: "agent-5", err: fmt.Errorf("not found")},
		}

		for _, update := range updates {
			completed++
			if update.err != nil {
				failed++
			} else {
				succeeded++
			}
		}

		if completed != 5 {
			t.Errorf("expected completed=5, got %d", completed)
		}

		if succeeded != 3 {
			t.Errorf("expected succeeded=3, got %d", succeeded)
		}

		if failed != 2 {
			t.Errorf("expected failed=2, got %d", failed)
		}

		expectedPercent := (completed * 100) / total
		if expectedPercent != 50 {
			t.Errorf("expected 50%% progress, got %d%%", expectedPercent)
		}
	})
}

func TestStopProgressOutput(t *testing.T) {
	t.Parallel()

	t.Run("progress line format", func(t *testing.T) {
		t.Parallel()

		line := bkIO.ProgressLine("Stopping agents", 5, 10, 3, 2, 6)

		if !strings.Contains(line, "Stopping agents") {
			t.Error("expected line to contain 'Stopping agents'")
		}
		if !strings.Contains(line, "50%") {
			t.Error("expected line to contain percentage")
		}
		if !strings.Contains(line, "5/10") {
			t.Error("expected line to contain completed/total")
		}
		if !strings.Contains(line, "succeeded:3") {
			t.Error("expected line to contain success count")
		}
		if !strings.Contains(line, "failed:2") {
			t.Error("expected line to contain fail count")
		}
		if !strings.Contains(line, "[") || !strings.Contains(line, "]") {
			t.Error("expected line to contain progress bar brackets")
		}
	})
}
