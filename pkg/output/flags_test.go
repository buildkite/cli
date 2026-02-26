package output

import "testing"

func TestOutputFlags_AfterApply(t *testing.T) {
	tests := []struct {
		name     string
		flags    OutputFlags
		expected string
	}{
		{
			name:     "json flag sets output to json",
			flags:    OutputFlags{JSON: true},
			expected: "json",
		},
		{
			name:     "yaml flag sets output to yaml",
			flags:    OutputFlags{YAML: true},
			expected: "yaml",
		},
		{
			name:     "text flag sets output to text",
			flags:    OutputFlags{Text: true},
			expected: "text",
		},
		{
			name:     "no flags leaves output empty",
			flags:    OutputFlags{},
			expected: "",
		},
		{
			name:     "explicit output value is preserved when no bool flags set",
			flags:    OutputFlags{Output: "yaml"},
			expected: "yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.flags.AfterApply()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.flags.Output != tt.expected {
				t.Errorf("expected Output=%q, got %q", tt.expected, tt.flags.Output)
			}
		})
	}
}
