package ui

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/buildkite/go-buildkite/v4"
)

// stripANSI removes ANSI escape sequences from a string
// This is used to remove styling from lipgloss-styled strings for testing
func stripANSI(str string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return re.ReplaceAllString(str, "")
}

func TestStatusRendering(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		state    string
		blocked  bool
		contains string
	}{
		{
			name:     "success state",
			state:    "success",
			contains: IconSuccess,
		},
		{
			name:     "error state",
			state:    "error",
			contains: IconError,
		},
		{
			name:     "running state",
			state:    "running",
			contains: IconRunning,
		},
		{
			name:     "blocked success",
			state:    "passed",
			blocked:  true,
			contains: "blocked",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := StatusIcon(tc.state, WithBlocked(tc.blocked))
			if !strings.Contains(result, tc.contains) {
				t.Errorf("Expected result to contain %q, but got: %q", tc.contains, result)
			}

			// Also test the RenderStatus function
			rendered := RenderStatus(tc.state, WithBlocked(tc.blocked))
			strippedRendered := stripANSI(rendered)
			if !strings.Contains(strippedRendered, tc.contains) {
				t.Errorf("Expected rendered result to contain %q, but got: %q",
					tc.contains, strippedRendered)
			}
		})
	}
}

func TestTextFormatting(t *testing.T) {
	t.Parallel()

	t.Run("truncate text", func(t *testing.T) {
		t.Parallel()
		long := "This is a long text that should be truncated"
		result := TruncateText(long, 10)
		if len(result) != 13 { // 10 + 3 for ellipsis
			t.Errorf("Expected truncated text to be 11 chars, got %d: %q", len(result), result)
		}
		if !strings.HasSuffix(result, IconEllipsis) {
			t.Errorf("Expected truncated text to end with ellipsis, got: %q", result)
		}
	})

	t.Run("strip html tags", func(t *testing.T) {
		t.Parallel()
		html := "<p>This is <strong>HTML</strong> content</p>"
		result := StripHTMLTags(html)
		expected := "This is HTML content"
		if result != expected {
			t.Errorf("Expected stripped HTML to be %q, got: %q", expected, result)
		}
	})

	t.Run("format bytes", func(t *testing.T) {
		t.Parallel()
		testCases := []struct {
			bytes    int64
			expected string
		}{
			{500, "500B"},
			{1536, "1.5KB"},
			{1500000, "1.4MB"},
			{1500000000, "1.4GB"},
		}

		for _, tc := range testCases {
			result := FormatBytes(tc.bytes)
			if result != tc.expected {
				t.Errorf("Expected %d bytes to format as %q, got: %q", tc.bytes, tc.expected, result)
			}
		}
	})
}

func TestLayoutComponents(t *testing.T) {
	t.Parallel()

	t.Run("section", func(t *testing.T) {
		t.Parallel()
		result := Section("Title", "Content")
		strippedResult := stripANSI(result)
		if !strings.Contains(strippedResult, "Title") {
			t.Errorf("Expected section to contain title")
		}
		if !strings.Contains(strippedResult, "Content") {
			t.Errorf("Expected section to contain content")
		}
	})

	t.Run("labeled value", func(t *testing.T) {
		t.Parallel()
		result := LabeledValue("Label", "Value")
		strippedResult := stripANSI(result)
		if !strings.Contains(strippedResult, "Label:") {
			t.Errorf("Expected labeled value to contain 'Label:'")
		}
		if !strings.Contains(strippedResult, "Value") {
			t.Errorf("Expected labeled value to contain 'Value'")
		}
	})

	t.Run("table", func(t *testing.T) {
		t.Parallel()
		headers := []string{"Col1", "Col2"}
		rows := [][]string{
			{"A", "B"},
			{"C", "D"},
		}
		result := Table(headers, rows)
		strippedResult := stripANSI(result)
		for _, h := range headers {
			if !strings.Contains(strippedResult, h) {
				t.Errorf("Expected table to contain header %q", h)
			}
		}
		for _, row := range rows {
			for _, cell := range row {
				if !strings.Contains(strippedResult, cell) {
					t.Errorf("Expected table to contain cell %q", cell)
				}
			}
		}
	})

	t.Run("card", func(t *testing.T) {
		t.Parallel()
		result := Card("Card Title", "Card Content", WithBorder(true))
		strippedResult := stripANSI(result)
		if !strings.Contains(strippedResult, "Card Title") {
			t.Errorf("Expected card to contain title")
		}
		if !strings.Contains(strippedResult, "Card Content") {
			t.Errorf("Expected card to contain content")
		}
	})
}

func TestBuildkiteComponents(t *testing.T) {
	t.Parallel()

	t.Run("render build number", func(t *testing.T) {
		t.Parallel()
		result := RenderBuildNumber("success", 123)
		strippedResult := stripANSI(result)
		if !strings.Contains(strippedResult, "Build #123") {
			t.Errorf("Expected build number to contain 'Build #123', got: %q", strippedResult)
		}
	})

	t.Run("render build summary", func(t *testing.T) {
		t.Parallel()
		now := time.Now()
		build := &buildkite.Build{
			Number:  42,
			State:   "passed",
			Message: "Test build",
			Source:  "api",
			Branch:  "main",
			Commit:  "abc123",
			Creator: buildkite.Creator{
				Name: "Test User",
			},
			CreatedAt: &buildkite.Timestamp{Time: now},
		}
		result := RenderBuildSummary(build)
		strippedResult := stripANSI(result)

		// Check for key pieces of information
		expectations := []string{
			"Build #42",
			"Test build",
			"Triggered via api by Test User",
			"Branch: main",
			"Commit: abc123",
		}

		for _, exp := range expectations {
			if !strings.Contains(strippedResult, exp) {
				t.Errorf("Expected build summary to contain %q, got: %q", exp, strippedResult)
			}
		}
	})

	t.Run("render job summary", func(t *testing.T) {
		t.Parallel()
		start := time.Now().Add(-time.Minute)
		finish := time.Now()
		job := buildkite.Job{
			Type:       "script",
			Name:       "Test Job",
			State:      "passed",
			Command:    "echo hello",
			StartedAt:  &buildkite.Timestamp{Time: start},
			FinishedAt: &buildkite.Timestamp{Time: finish},
		}
		result := RenderJobSummary(job)
		strippedResult := stripANSI(result)

		if !strings.Contains(strippedResult, "Test Job") {
			t.Errorf("Expected job summary to contain job name")
		}

		// Should contain a duration close to 1 minute
		if !strings.Contains(strippedResult, "1m0s") && !strings.Contains(strippedResult, "1m") {
			t.Errorf("Expected job summary to contain duration, got: %q", strippedResult)
		}
	})

	t.Run("render artifact", func(t *testing.T) {
		t.Parallel()
		artifact := &buildkite.Artifact{
			ID:       "art-123",
			Path:     "path/to/artifact.txt",
			FileSize: 1500,
		}
		result := RenderArtifact(artifact)
		strippedResult := stripANSI(result)

		if !strings.Contains(strippedResult, "art-123") {
			t.Errorf("Expected artifact render to contain ID")
		}
		if !strings.Contains(strippedResult, "path/to/artifact.txt") {
			t.Errorf("Expected artifact render to contain path")
		}
		if !strings.Contains(strippedResult, "1.5KB") {
			t.Errorf("Expected artifact render to contain formatted file size")
		}
	})
}
