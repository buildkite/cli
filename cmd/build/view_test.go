package build

import (
	"testing"
)

func TestViewCmd_BuildGetOptions_WithJobStates(t *testing.T) {
	cmd := &ViewCmd{
		JobStates: []string{"failed", "broken"},
	}

	opts := cmd.buildGetOptions()
	if opts == nil {
		t.Fatal("Expected non-nil BuildGetOptions")
	}

	if len(opts.JobStates) != 2 {
		t.Fatalf("Expected 2 job states, got %d", len(opts.JobStates))
	}

	if opts.JobStates[0] != "failed" {
		t.Errorf("Expected first state to be 'failed', got %q", opts.JobStates[0])
	}

	if opts.JobStates[1] != "broken" {
		t.Errorf("Expected second state to be 'broken', got %q", opts.JobStates[1])
	}
}

func TestViewCmd_BuildGetOptions_Empty(t *testing.T) {
	cmd := &ViewCmd{}

	opts := cmd.buildGetOptions()
	if opts != nil {
		t.Errorf("Expected nil BuildGetOptions when no job states, got %+v", opts)
	}
}

func TestViewCmd_BuildGetOptions_SingleState(t *testing.T) {
	cmd := &ViewCmd{
		JobStates: []string{"running"},
	}

	opts := cmd.buildGetOptions()
	if opts == nil {
		t.Fatal("Expected non-nil BuildGetOptions")
	}

	if len(opts.JobStates) != 1 {
		t.Fatalf("Expected 1 job state, got %d", len(opts.JobStates))
	}

	if opts.JobStates[0] != "running" {
		t.Errorf("Expected state to be 'running', got %q", opts.JobStates[0])
	}
}
