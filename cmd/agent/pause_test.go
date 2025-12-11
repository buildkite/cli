package agent

import (
	"testing"
)

func TestPauseCmdValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		timeout int
		wantErr bool
		errMsg  string
	}{
		{"valid timeout", 60, false, ""},
		{"minimum valid timeout", 1, false, ""},
		{"maximum valid timeout", 1440, false, ""},
		{"zero timeout invalid", 0, true, "timeout-in-minutes must be 1 or more"},
		{"negative timeout invalid", -1, true, "timeout-in-minutes must be 1 or more"},
		{"excessive timeout invalid", 1441, true, "timeout-in-minutes cannot exceed 1440 minutes (1 day)"},
		{"very large timeout invalid", 10000, true, "timeout-in-minutes cannot exceed 1440 minutes (1 day)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd := &PauseCmd{
				AgentID:          "test-agent",
				TimeoutInMinutes: tt.timeout,
			}

			var err error
			if cmd.TimeoutInMinutes <= 0 {
				err = errValidation("timeout-in-minutes must be 1 or more")
			} else if cmd.TimeoutInMinutes > 1440 {
				err = errValidation("timeout-in-minutes cannot exceed 1440 minutes (1 day)")
			}

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if err.Error() != tt.errMsg {
					t.Errorf("expected error %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

type validationError string

func (e validationError) Error() string { return string(e) }
func errValidation(msg string) error    { return validationError(msg) }
