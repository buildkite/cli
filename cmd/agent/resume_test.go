package agent

import (
	"testing"
)

func TestResumeCmdStructure(t *testing.T) {
	t.Parallel()

	cmd := &ResumeCmd{
		AgentID: "test-agent-123",
	}

	if cmd.AgentID != "test-agent-123" {
		t.Errorf("expected AgentID to be %q, got %q", "test-agent-123", cmd.AgentID)
	}
}

func TestResumeCmdHelp(t *testing.T) {
	t.Parallel()

	cmd := &ResumeCmd{}
	help := cmd.Help()

	if help == "" {
		t.Error("Help() should return non-empty string")
	}

	if len(help) < 10 {
		t.Errorf("Help text seems too short: %q", help)
	}
}
