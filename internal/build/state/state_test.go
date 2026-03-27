package state

import "testing"

func TestIsTerminal(t *testing.T) {
	tests := []struct {
		name  string
		state State
		want  bool
	}{
		{name: "scheduled", state: Scheduled, want: false},
		{name: "running", state: Running, want: false},
		{name: "blocked", state: Blocked, want: false},
		{name: "canceling", state: Canceling, want: false},
		{name: "failing", state: Failing, want: false},
		{name: "passed", state: Passed, want: true},
		{name: "failed", state: Failed, want: true},
		{name: "canceled", state: Canceled, want: true},
		{name: "skipped", state: Skipped, want: true},
		{name: "not run", state: NotRun, want: true},
		{name: "unknown", state: State("mystery"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTerminal(tt.state); got != tt.want {
				t.Fatalf("IsTerminal(%q) = %v, want %v", tt.state, got, tt.want)
			}
		})
	}
}

func TestIsIncomplete(t *testing.T) {
	tests := []struct {
		name  string
		state State
		want  bool
	}{
		{name: "scheduled", state: Scheduled, want: true},
		{name: "running", state: Running, want: true},
		{name: "blocked", state: Blocked, want: true},
		{name: "canceling", state: Canceling, want: true},
		{name: "failing", state: Failing, want: true},
		{name: "passed", state: Passed, want: false},
		{name: "failed", state: Failed, want: false},
		{name: "canceled", state: Canceled, want: false},
		{name: "skipped", state: Skipped, want: false},
		{name: "not run", state: NotRun, want: false},
		{name: "unknown", state: State("mystery"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsIncomplete(tt.state); got != tt.want {
				t.Fatalf("IsIncomplete(%q) = %v, want %v", tt.state, got, tt.want)
			}
		})
	}
}
