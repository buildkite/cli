package agent

import (
	"strings"
	"testing"
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
