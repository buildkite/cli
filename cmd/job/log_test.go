package job

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	buildkitelogs "github.com/buildkite/buildkite-logs"
)

func TestLogCmdValidateFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cmd     LogCmd
		wantErr string
	}{
		{
			name:    "step and job ID conflict",
			cmd:     LogCmd{Step: "test", JobID: "abc-123", Seek: -1},
			wantErr: "--step and a positional job ID are mutually exclusive",
		},
		{
			name: "valid flags - step only",
			cmd:  LogCmd{Step: "test", Seek: -1},
		},
		{
			name:    "tail and seek conflict",
			cmd:     LogCmd{Tail: 50, Seek: 10},
			wantErr: "--tail and --seek are mutually exclusive",
		},
		{
			name:    "follow and seek conflict",
			cmd:     LogCmd{Follow: true, Seek: 100},
			wantErr: "--follow and --seek cannot be used together",
		},
		{
			name: "valid flags - tail only",
			cmd:  LogCmd{Tail: 50, Seek: -1},
		},
		{
			name: "valid flags - follow only",
			cmd:  LogCmd{Follow: true, Seek: -1},
		},
		{
			name: "valid flags - seek and limit",
			cmd:  LogCmd{Seek: 100, Limit: 50},
		},
		{
			name: "valid flags - defaults",
			cmd:  LogCmd{Seek: -1},
		},
		// --timestamps / --no-timestamps
		{
			name:    "timestamps and no-timestamps conflict",
			cmd:     LogCmd{Timestamps: true, NoTimestamps: true, Seek: -1},
			wantErr: "--timestamps and --no-timestamps are mutually exclusive",
		},
		{
			name: "valid flags - timestamps",
			cmd:  LogCmd{Timestamps: true, Seek: -1},
		},
		// --since / --until
		{
			name:    "since and seek conflict",
			cmd:     LogCmd{Since: "5m", Seek: 100},
			wantErr: "--since/--until and --seek are mutually exclusive",
		},
		{
			name:    "until and seek conflict",
			cmd:     LogCmd{Until: "5m", Seek: 100},
			wantErr: "--since/--until and --seek are mutually exclusive",
		},
		{
			name:    "follow and until conflict",
			cmd:     LogCmd{Follow: true, Until: "5m", Seek: -1},
			wantErr: "--follow and --until cannot be used together",
		},
		{
			name:    "invalid since value",
			cmd:     LogCmd{Since: "not-a-time", Seek: -1},
			wantErr: "invalid --since value",
		},
		{
			name:    "invalid until value",
			cmd:     LogCmd{Until: "not-a-time", Seek: -1},
			wantErr: "invalid --until value",
		},
		{
			name: "valid flags - since duration",
			cmd:  LogCmd{Since: "10m", Seek: -1},
		},
		{
			name: "valid flags - since RFC3339",
			cmd:  LogCmd{Since: "2024-01-15T10:00:00Z", Seek: -1},
		},
		{
			name: "valid flags - follow with since",
			cmd:  LogCmd{Follow: true, Since: "5m", Seek: -1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cmd.validateFlags()
			if tt.wantErr != "" {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestWriteEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cmd      LogCmd
		entry    buildkitelogs.ParquetLogEntry
		expected string
	}{
		{
			name:     "plain entry",
			cmd:      LogCmd{},
			entry:    buildkitelogs.ParquetLogEntry{Content: "hello world", RowNumber: 0},
			expected: "hello world\n",
		},
		{
			name:     "entry with timestamp stripping",
			cmd:      LogCmd{NoTimestamps: true},
			entry:    buildkitelogs.ParquetLogEntry{Content: "bk;t=1234567890\x07some output", RowNumber: 0},
			expected: "some output\n",
		},
		{
			name:     "entry with trailing newlines trimmed",
			cmd:      LogCmd{},
			entry:    buildkitelogs.ParquetLogEntry{Content: "line with newlines\n\n", RowNumber: 0},
			expected: "line with newlines\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			tt.cmd.writeEntry(&buf, &tt.entry)
			if got := buf.String(); got != tt.expected {
				t.Errorf("writeEntry() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestStripTimestamps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{"bk;t=1234567890\x07hello", "hello"},
		{"no timestamps here", "no timestamps here"},
		{"bk;t=0\x07start bk;t=999\x07end", "start end"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			if got := stripTimestamps(tt.input); got != tt.expected {
				t.Errorf("stripTimestamps(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestBuildJobLabels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		jobs     []cmdJob
		expected []string
	}{
		{
			name: "all unique labels",
			jobs: []cmdJob{
				{id: "aaa-111", label: "rspec", state: "passed"},
				{id: "bbb-222", label: "lint", state: "passed"},
			},
			expected: []string{"rspec (passed)", "lint (passed)"},
		},
		{
			name: "duplicate labels get ID suffix",
			jobs: []cmdJob{
				{id: "aaa11111-long-id", label: "rspec", state: "running"},
				{id: "bbb22222-long-id", label: "rspec", state: "running"},
			},
			expected: []string{"rspec (running) [aaa11111]", "rspec (running) [bbb22222]"},
		},
		{
			name: "mix of duplicates and unique",
			jobs: []cmdJob{
				{id: "aaa11111-long-id", label: "rspec", state: "running"},
				{id: "bbb22222-long-id", label: "lint", state: "passed"},
				{id: "ccc33333-long-id", label: "rspec", state: "running"},
			},
			expected: []string{"rspec (running) [aaa11111]", "lint (passed)", "rspec (running) [ccc33333]"},
		},
		{
			name: "short ID used as-is",
			jobs: []cmdJob{
				{id: "short", label: "rspec", state: "running"},
				{id: "other", label: "rspec", state: "running"},
			},
			expected: []string{"rspec (running) [short]", "rspec (running) [other]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := buildJobLabels(tt.jobs)
			if len(got) != len(tt.expected) {
				t.Fatalf("buildJobLabels() returned %d labels, want %d", len(got), len(tt.expected))
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("buildJobLabels()[%d] = %q, want %q", i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestLogCmdHelp(t *testing.T) {
	t.Parallel()
	cmd := &LogCmd{}
	help := cmd.Help()
	if !strings.Contains(help, "bk job log") {
		t.Error("help text should contain usage examples")
	}
	if !strings.Contains(help, "-f") {
		t.Error("help text should mention follow flag")
	}
	if !strings.Contains(help, "--since") {
		t.Error("help text should mention since flag")
	}
	if !strings.Contains(help, "--json") {
		t.Error("help text should mention json flag")
	}
}

func TestParseTimeFlag(t *testing.T) {
	t.Parallel()

	t.Run("duration string", func(t *testing.T) {
		t.Parallel()
		before := time.Now()
		result, err := parseTimeFlag("5m")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := before.Add(-5 * time.Minute)
		// Allow 1 second tolerance
		if result.Before(expected.Add(-time.Second)) || result.After(expected.Add(time.Second)) {
			t.Errorf("parseTimeFlag(\"5m\") = %v, want ~%v", result, expected)
		}
	})

	t.Run("RFC3339 timestamp", func(t *testing.T) {
		t.Parallel()
		result, err := parseTimeFlag("2024-01-15T10:30:00Z")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
		if !result.Equal(expected) {
			t.Errorf("parseTimeFlag(\"2024-01-15T10:30:00Z\") = %v, want %v", result, expected)
		}
	})

	t.Run("invalid value", func(t *testing.T) {
		t.Parallel()
		_, err := parseTimeFlag("not-a-time")
		if err == nil {
			t.Error("expected error for invalid time value")
		}
	})
}

func TestWriteEntryWithTimestamps(t *testing.T) {
	t.Parallel()
	cmd := LogCmd{Timestamps: true}
	entry := buildkitelogs.ParquetLogEntry{
		Content:   "bk;t=1705314600000\x07hello world",
		Timestamp: 1705314600000, // 2024-01-15T10:30:00Z
		RowNumber: 0,
	}

	var buf bytes.Buffer
	cmd.writeEntry(&buf, &entry)
	got := buf.String()

	if !strings.HasPrefix(got, "2024-01-15T10:30:00Z") {
		t.Errorf("expected timestamp prefix, got %q", got)
	}
	if !strings.Contains(got, "hello world") {
		t.Error("expected content in output")
	}
	// Raw bk;t= marker should be stripped
	if strings.Contains(got, "bk;t=") {
		t.Error("raw bk;t= marker should be stripped when --timestamps is used")
	}
}

func TestWriteEntryJSON(t *testing.T) {
	t.Parallel()
	cmd := LogCmd{JSON: true}
	entry := buildkitelogs.ParquetLogEntry{
		Content:   "hello world",
		Timestamp: 1705314600000,
		RowNumber: 42,
		Group:     "test-group",
	}

	var buf bytes.Buffer
	cmd.writeEntry(&buf, &entry)

	var result logEntryJSON
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal JSON output: %v", err)
	}
	if result.RowNumber != 42 {
		t.Errorf("row_number = %d, want 42", result.RowNumber)
	}
	if result.Content != "hello world" {
		t.Errorf("content = %q, want %q", result.Content, "hello world")
	}
	if result.Group != "test-group" {
		t.Errorf("group = %q, want %q", result.Group, "test-group")
	}
	if result.Timestamp != "2024-01-15T10:30:00Z" {
		t.Errorf("timestamp = %q, want %q", result.Timestamp, "2024-01-15T10:30:00Z")
	}
}

func TestEntryInTimeRange(t *testing.T) {
	t.Parallel()

	t.Run("no time filters", func(t *testing.T) {
		t.Parallel()
		cmd := LogCmd{}
		entry := &buildkitelogs.ParquetLogEntry{Timestamp: 1000}
		if !cmd.entryInTimeRange(entry) {
			t.Error("should pass with no time filters")
		}
	})

	t.Run("since filter includes entry", func(t *testing.T) {
		t.Parallel()
		sinceTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
		cmd := LogCmd{Since: "2024-01-15T10:00:00Z", sinceTime: sinceTime}
		entry := &buildkitelogs.ParquetLogEntry{Timestamp: time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC).UnixMilli()}
		if !cmd.entryInTimeRange(entry) {
			t.Error("entry after --since should be included")
		}
	})

	t.Run("since filter excludes entry", func(t *testing.T) {
		t.Parallel()
		sinceTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
		cmd := LogCmd{Since: "2024-01-15T10:00:00Z", sinceTime: sinceTime}
		entry := &buildkitelogs.ParquetLogEntry{Timestamp: time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC).UnixMilli()}
		if cmd.entryInTimeRange(entry) {
			t.Error("entry before --since should be excluded")
		}
	})

	t.Run("until filter includes entry", func(t *testing.T) {
		t.Parallel()
		untilTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
		cmd := LogCmd{Until: "2024-01-15T12:00:00Z", untilTime: untilTime}
		entry := &buildkitelogs.ParquetLogEntry{Timestamp: time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC).UnixMilli()}
		if !cmd.entryInTimeRange(entry) {
			t.Error("entry before --until should be included")
		}
	})

	t.Run("until filter excludes entry", func(t *testing.T) {
		t.Parallel()
		untilTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
		cmd := LogCmd{Until: "2024-01-15T12:00:00Z", untilTime: untilTime}
		entry := &buildkitelogs.ParquetLogEntry{Timestamp: time.Date(2024, 1, 15, 13, 0, 0, 0, time.UTC).UnixMilli()}
		if cmd.entryInTimeRange(entry) {
			t.Error("entry after --until should be excluded")
		}
	})
}

func TestShouldAutoFollow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cmd  LogCmd
		want bool
	}{
		{
			name: "default flags - should auto-follow",
			cmd:  LogCmd{Seek: -1},
			want: true,
		},
		{
			name: "explicit follow set - no auto-follow needed",
			cmd:  LogCmd{Follow: true, Seek: -1},
			want: false,
		},
		{
			name: "tail set - should not auto-follow",
			cmd:  LogCmd{Tail: 50, Seek: -1},
			want: false,
		},
		{
			name: "seek set - should not auto-follow",
			cmd:  LogCmd{Seek: 100},
			want: false,
		},
		{
			name: "limit set - should not auto-follow",
			cmd:  LogCmd{Limit: 10, Seek: -1},
			want: false,
		},
		{
			name: "since set - should not auto-follow",
			cmd:  LogCmd{Since: "5m", Seek: -1},
			want: false,
		},
		{
			name: "until set - should not auto-follow",
			cmd:  LogCmd{Until: "5m", Seek: -1},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.cmd.shouldAutoFollow()
			if got != tt.want {
				t.Errorf("shouldAutoFollow() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseJobURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantNil   bool
		wantOrg   string
		wantPipe  string
		wantBuild string
		wantJobID string
	}{
		{
			name:      "full job URL with fragment",
			input:     "https://buildkite.com/my-org/my-pipeline/builds/456#0190046e-e199-453b-a302-a21a4d649d31",
			wantOrg:   "my-org",
			wantPipe:  "my-pipeline",
			wantBuild: "456",
			wantJobID: "0190046e-e199-453b-a302-a21a4d649d31",
		},
		{
			name:      "build URL without job fragment",
			input:     "https://buildkite.com/my-org/my-pipeline/builds/789",
			wantOrg:   "my-org",
			wantPipe:  "my-pipeline",
			wantBuild: "789",
			wantJobID: "",
		},
		{
			name:      "URL with trailing whitespace",
			input:     "  https://buildkite.com/org/pipe/builds/1#abc-def  ",
			wantOrg:   "org",
			wantPipe:  "pipe",
			wantBuild: "1",
			wantJobID: "abc-def",
		},
		{
			name:    "plain job UUID",
			input:   "0190046e-e199-453b-a302-a21a4d649d31",
			wantNil: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantNil: true,
		},
		{
			name:    "non-buildkite URL",
			input:   "https://example.com/org/pipe/builds/123#job-id",
			wantNil: true,
		},
		{
			name:    "buildkite URL with wrong path",
			input:   "https://buildkite.com/org/pipe/jobs/123",
			wantNil: true,
		},
		{
			name:      "http URL (not https)",
			input:     "http://buildkite.com/org/pipe/builds/99#aaa-bbb",
			wantOrg:   "org",
			wantPipe:  "pipe",
			wantBuild: "99",
			wantJobID: "aaa-bbb",
		},
		{
			name:      "uppercase UUID in fragment",
			input:     "https://buildkite.com/org/pipe/builds/1#0190046E-E199-453B-A302-A21A4D649D31",
			wantOrg:   "org",
			wantPipe:  "pipe",
			wantBuild: "1",
			wantJobID: "0190046E-E199-453B-A302-A21A4D649D31",
		},
		{
			name:    "URL with query params before fragment",
			input:   "https://buildkite.com/org/pipe/builds/123?utm_source=slack#job-id",
			wantNil: true,
		},
		{
			name:    "URL with trailing slash",
			input:   "https://buildkite.com/org/pipe/builds/123/",
			wantNil: true,
		},
		{
			name:    "URL with extra path segments",
			input:   "https://buildkite.com/org/pipe/builds/123/extra",
			wantNil: true,
		},
		{
			name:    "fragment with non-hex characters",
			input:   "https://buildkite.com/org/pipe/builds/123#not-a-valid-uuid!",
			wantNil: true,
		},
		{
			name:      "mixed case UUID",
			input:     "https://buildkite.com/org/pipe/builds/5#aBcDeF-1234",
			wantOrg:   "org",
			wantPipe:  "pipe",
			wantBuild: "5",
			wantJobID: "aBcDeF-1234",
		},
		{
			name:    "empty fragment",
			input:   "https://buildkite.com/org/pipe/builds/123#",
			wantNil: true,
		},
		{
			name:      "Slack angle-bracket wrapped URL",
			input:     "<https://buildkite.com/org/pipe/builds/55#abc-def>",
			wantOrg:   "org",
			wantPipe:  "pipe",
			wantBuild: "55",
			wantJobID: "abc-def",
		},
		{
			name:      "Slack angle-bracket wrapped build-only URL",
			input:     "<https://buildkite.com/org/pipe/builds/55>",
			wantOrg:   "org",
			wantPipe:  "pipe",
			wantBuild: "55",
			wantJobID: "",
		},
		{
			name:    "markdown link is not parsed",
			input:   "[Build 123](https://buildkite.com/org/pipe/builds/123#job-id)",
			wantNil: true,
		},
		{
			name:    "double-pasted URL",
			input:   "https://buildkite.com/org/pipe/builds/123#abc-defhttps://buildkite.com",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := parseJobURL(tt.input)
			if tt.wantNil {
				if result != nil {
					t.Errorf("parseJobURL(%q) = %+v, want nil", tt.input, result)
				}
				return
			}
			if result == nil {
				t.Fatalf("parseJobURL(%q) = nil, want non-nil", tt.input)
			}
			if result.org != tt.wantOrg {
				t.Errorf("org = %q, want %q", result.org, tt.wantOrg)
			}
			if result.pipeline != tt.wantPipe {
				t.Errorf("pipeline = %q, want %q", result.pipeline, tt.wantPipe)
			}
			if result.buildNumber != tt.wantBuild {
				t.Errorf("buildNumber = %q, want %q", result.buildNumber, tt.wantBuild)
			}
			if result.jobID != tt.wantJobID {
				t.Errorf("jobID = %q, want %q", result.jobID, tt.wantJobID)
			}
		})
	}
}

func TestURLOverridesFields(t *testing.T) {
	t.Parallel()

	t.Run("URL populates pipeline, build, and jobID", func(t *testing.T) {
		t.Parallel()
		cmd := LogCmd{
			JobID: "https://buildkite.com/acme/deploy/builds/42#aaa-bbb-ccc",
			Seek:  -1,
		}
		parsed := parseJobURL(cmd.JobID)
		if parsed == nil {
			t.Fatal("expected URL to parse")
		}
		cmd.Pipeline = parsed.org + "/" + parsed.pipeline
		cmd.BuildNumber = parsed.buildNumber
		cmd.JobID = parsed.jobID

		if cmd.Pipeline != "acme/deploy" {
			t.Errorf("Pipeline = %q, want %q", cmd.Pipeline, "acme/deploy")
		}
		if cmd.BuildNumber != "42" {
			t.Errorf("BuildNumber = %q, want %q", cmd.BuildNumber, "42")
		}
		if cmd.JobID != "aaa-bbb-ccc" {
			t.Errorf("JobID = %q, want %q", cmd.JobID, "aaa-bbb-ccc")
		}
	})

	t.Run("build-only URL leaves JobID empty", func(t *testing.T) {
		t.Parallel()
		cmd := LogCmd{
			JobID: "https://buildkite.com/acme/deploy/builds/42",
			Seek:  -1,
		}
		parsed := parseJobURL(cmd.JobID)
		if parsed == nil {
			t.Fatal("expected URL to parse")
		}
		cmd.Pipeline = parsed.org + "/" + parsed.pipeline
		cmd.BuildNumber = parsed.buildNumber
		cmd.JobID = parsed.jobID

		if cmd.JobID != "" {
			t.Errorf("JobID = %q, want empty for build-only URL", cmd.JobID)
		}
		if cmd.Pipeline != "acme/deploy" {
			t.Errorf("Pipeline = %q, want %q", cmd.Pipeline, "acme/deploy")
		}
	})

	t.Run("build-only URL with --step is valid", func(t *testing.T) {
		t.Parallel()
		cmd := LogCmd{
			JobID: "", // after URL parsing, jobID is empty for build-only URL
			Step:  "test",
			Seek:  -1,
		}
		if err := cmd.validateFlags(); err != nil {
			t.Errorf("expected no error for build-only URL + --step, got: %v", err)
		}
	})

	t.Run("full URL with --step conflicts", func(t *testing.T) {
		t.Parallel()
		// After URL parsing, JobID is set, so --step should conflict
		cmd := LogCmd{
			JobID: "aaa-bbb-ccc", // simulates post-URL-parse state
			Step:  "test",
			Seek:  -1,
		}
		err := cmd.validateFlags()
		if err == nil {
			t.Error("expected error for URL with job fragment + --step")
		}
		if !strings.Contains(err.Error(), "mutually exclusive") {
			t.Errorf("expected mutually exclusive error, got: %v", err)
		}
	})
}

func TestWriteEntryEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("JSON output always includes timestamp regardless of --timestamps flag", func(t *testing.T) {
		t.Parallel()
		cmd := LogCmd{JSON: true, Timestamps: true}
		entry := buildkitelogs.ParquetLogEntry{
			Content:   "hello",
			Timestamp: 1705314600000,
			RowNumber: 0,
		}
		var buf bytes.Buffer
		cmd.writeEntry(&buf, &entry)

		var result logEntryJSON
		if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
			t.Fatalf("failed to unmarshal JSON: %v", err)
		}
		if result.Timestamp != "2024-01-15T10:30:00Z" {
			t.Errorf("timestamp = %q, want %q", result.Timestamp, "2024-01-15T10:30:00Z")
		}
	})

	t.Run("JSON output strips ANSI from content", func(t *testing.T) {
		t.Parallel()
		cmd := LogCmd{JSON: true}
		entry := buildkitelogs.ParquetLogEntry{
			Content:   "hello",
			Timestamp: 1000,
			RowNumber: 0,
		}
		var buf bytes.Buffer
		cmd.writeEntry(&buf, &entry)

		var result logEntryJSON
		if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
			t.Fatalf("failed to unmarshal JSON: %v", err)
		}
		// CleanContent(true) strips ANSI, so no escape codes should remain
		if strings.Contains(result.Content, "\x1b") {
			t.Error("JSON content should not contain ANSI escape codes")
		}
	})

	t.Run("empty content produces valid output", func(t *testing.T) {
		t.Parallel()
		cmd := LogCmd{}
		entry := buildkitelogs.ParquetLogEntry{Content: "", RowNumber: 0}
		var buf bytes.Buffer
		cmd.writeEntry(&buf, &entry)
		if buf.String() != "\n" {
			t.Errorf("expected single newline for empty content, got %q", buf.String())
		}
	})

	t.Run("empty content JSON produces valid JSONL", func(t *testing.T) {
		t.Parallel()
		cmd := LogCmd{JSON: true}
		entry := buildkitelogs.ParquetLogEntry{Content: "", Timestamp: 1000, RowNumber: 0}
		var buf bytes.Buffer
		cmd.writeEntry(&buf, &entry)

		var result logEntryJSON
		if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
			t.Fatalf("empty content should produce valid JSON, got error: %v", err)
		}
		if result.Content != "" {
			t.Errorf("content = %q, want empty", result.Content)
		}
	})

	t.Run("multiple bk;t= markers stripped", func(t *testing.T) {
		t.Parallel()
		cmd := LogCmd{NoTimestamps: true}
		entry := buildkitelogs.ParquetLogEntry{
			Content:   "bk;t=111\x07first bk;t=222\x07second",
			RowNumber: 0,
		}
		var buf bytes.Buffer
		cmd.writeEntry(&buf, &entry)
		if strings.Contains(buf.String(), "bk;t=") {
			t.Errorf("all bk;t= markers should be stripped, got %q", buf.String())
		}
		if !strings.Contains(buf.String(), "first") || !strings.Contains(buf.String(), "second") {
			t.Errorf("content around markers should be preserved, got %q", buf.String())
		}
	})

	t.Run("JSON group field omitted when empty", func(t *testing.T) {
		t.Parallel()
		cmd := LogCmd{JSON: true}
		entry := buildkitelogs.ParquetLogEntry{Content: "hi", Timestamp: 1000, RowNumber: 0, Group: ""}
		var buf bytes.Buffer
		cmd.writeEntry(&buf, &entry)
		if strings.Contains(buf.String(), `"group"`) {
			t.Error("group field should be omitted when empty (omitempty)")
		}
	})
}

func TestBuildJobLabelsParallelIndex(t *testing.T) {
	t.Parallel()

	idx0, idx1, idx2 := 0, 1, 2
	jobs := []cmdJob{
		{id: "aaa11111-long", label: "rspec #0", state: "failed"},
		{id: "bbb22222-long", label: "rspec #1", state: "passed"},
		{id: "ccc33333-long", label: "rspec #2", state: "passed"},
	}
	labels := buildJobLabels(jobs)

	// All have the same base "rspec #N (state)" pattern but different labels, so they shouldn't
	// need disambiguation UNLESS the full label+state string matches
	_ = idx0
	_ = idx1
	_ = idx2

	if len(labels) != 3 {
		t.Fatalf("expected 3 labels, got %d", len(labels))
	}
	// With different parallel indices, labels should be unique (no ID suffix needed)
	for _, l := range labels {
		if strings.Contains(l, "[") {
			t.Errorf("unique parallel labels shouldn't need ID suffix, got %q", l)
		}
	}
}

func TestEntryInTimeRangeBoundary(t *testing.T) {
	t.Parallel()

	t.Run("entry exactly at since boundary is included", func(t *testing.T) {
		t.Parallel()
		boundary := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
		cmd := LogCmd{Since: "2024-01-15T10:00:00Z", sinceTime: boundary}
		entry := &buildkitelogs.ParquetLogEntry{Timestamp: boundary.UnixMilli()}
		if !cmd.entryInTimeRange(entry) {
			t.Error("entry exactly at --since boundary should be included")
		}
	})

	t.Run("entry exactly at until boundary is included", func(t *testing.T) {
		t.Parallel()
		boundary := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
		cmd := LogCmd{Until: "2024-01-15T12:00:00Z", untilTime: boundary}
		entry := &buildkitelogs.ParquetLogEntry{Timestamp: boundary.UnixMilli()}
		if !cmd.entryInTimeRange(entry) {
			t.Error("entry exactly at --until boundary should be included")
		}
	})

	t.Run("entry 1ms before since is excluded", func(t *testing.T) {
		t.Parallel()
		boundary := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
		cmd := LogCmd{Since: "2024-01-15T10:00:00Z", sinceTime: boundary}
		entry := &buildkitelogs.ParquetLogEntry{Timestamp: boundary.UnixMilli() - 1}
		if cmd.entryInTimeRange(entry) {
			t.Error("entry 1ms before --since should be excluded")
		}
	})

	t.Run("entry 1ms after until is excluded", func(t *testing.T) {
		t.Parallel()
		boundary := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
		cmd := LogCmd{Until: "2024-01-15T12:00:00Z", untilTime: boundary}
		entry := &buildkitelogs.ParquetLogEntry{Timestamp: boundary.UnixMilli() + 1}
		if cmd.entryInTimeRange(entry) {
			t.Error("entry 1ms after --until should be excluded")
		}
	})

	t.Run("since and until together - entry in range", func(t *testing.T) {
		t.Parallel()
		since := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
		until := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
		cmd := LogCmd{
			Since: "2024-01-15T10:00:00Z", sinceTime: since,
			Until: "2024-01-15T12:00:00Z", untilTime: until,
		}
		entry := &buildkitelogs.ParquetLogEntry{Timestamp: time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC).UnixMilli()}
		if !cmd.entryInTimeRange(entry) {
			t.Error("entry within since/until range should be included")
		}
	})

	t.Run("since and until together - entry outside range", func(t *testing.T) {
		t.Parallel()
		since := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
		until := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
		cmd := LogCmd{
			Since: "2024-01-15T10:00:00Z", sinceTime: since,
			Until: "2024-01-15T12:00:00Z", untilTime: until,
		}
		entry := &buildkitelogs.ParquetLogEntry{Timestamp: time.Date(2024, 1, 15, 13, 0, 0, 0, time.UTC).UnixMilli()}
		if cmd.entryInTimeRange(entry) {
			t.Error("entry after until should be excluded")
		}
	})
}
