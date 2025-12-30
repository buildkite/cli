package agent

import "testing"

func TestParseMetadata(t *testing.T) {
	cases := []struct {
		name     string
		input    []string
		metadata string
		queue    string
	}{
		{
			name:     "single queue entry",
			input:    []string{"queue=production"},
			metadata: "~",
			queue:    "production",
		},
		{
			name:     "single non-queue entry",
			input:    []string{"os=linux"},
			metadata: "os=linux",
			queue:    "default",
		},
		{
			name:     "multiple entries with queue",
			input:    []string{"queue=deploy", "os=linux", "region=us"},
			metadata: "os=linux, region=us",
			queue:    "deploy",
		},
		{
			name:     "no entries",
			input:    nil,
			metadata: "",
			queue:    "default",
		},
		{
			name:     "multiple entries without queue",
			input:    []string{"os=linux", "region=us"},
			metadata: "os=linux, region=us",
			queue:    "default",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			metadata, queue := parseMetadata(tc.input)
			if metadata != tc.metadata {
				t.Fatalf("metadata mismatch: got %q want %q", metadata, tc.metadata)
			}
			if queue != tc.queue {
				t.Fatalf("queue mismatch: got %q want %q", queue, tc.queue)
			}
		})
	}
}
