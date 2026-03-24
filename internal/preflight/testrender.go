package preflight

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/buildkite/cli/v3/pkg/output"
)

const MaxDisplayedTests = 10

// FailedTest represents a test that failed in the build.
type FailedTest struct {
	ID          string
	Scope       string
	Name        string
	Location    string
	Reliability float64
}

// FormatFailedTestsHeader renders the section header line.
func FormatFailedTestsHeader(total int) string {
	return fmt.Sprintf("\n  ── Failed Tests (%d) %s\n", total, strings.Repeat("─", 60))
}

// FormatFailedTestsTable renders the table rows (with header) for the given tests.
// Each line is returned individually so callers can promote them to scrollback.
func FormatFailedTestsTable(tests []FailedTest) []string {
	if len(tests) == 0 {
		return nil
	}

	rows := make([][]string, len(tests))
	for i, t := range tests {
		name := fmt.Sprintf("%s✗%s %s", ansiRed, ansiReset, formatTestName(t))
		loc := shortenLocation(t.Location)
		rel := fmt.Sprintf("%.0f%%", t.Reliability*100)
		rows[i] = []string{name, loc, rel}
	}

	table := output.Table(
		[]string{"Test", "Location", "Reliability"},
		rows,
		map[string]string{"location": "dim", "reliability": "dim"},
	)

	var lines []string
	for _, line := range strings.Split(strings.TrimRight(table, "\n"), "\n") {
		lines = append(lines, "  "+line)
	}
	return lines
}

// FormatFailedTestsOverflow renders the "… and N more" line.
func FormatFailedTestsOverflow(remaining, buildNumber int) string {
	return fmt.Sprintf("  … and %d more — run `bk build test failures %d` to view all", remaining, buildNumber)
}

func formatTestName(t FailedTest) string {
	if t.Scope != "" {
		return t.Scope + " · " + t.Name
	}
	return t.Name
}

func shortenLocation(location string) string {
	if location == "" {
		return ""
	}
	return filepath.Base(location)
}

// MockFailedTests returns hardcoded test data for UI development.
func MockFailedTests() []FailedTest {
	return []FailedTest{
		{ID: "t-001", Scope: "AuthController#authenticate", Name: "rejects expired tokens", Location: "test/controllers/auth_controller_test.rb:91", Reliability: 0},
		{ID: "t-002", Scope: "AuthController#authenticate", Name: "returns 401 for missing credentials", Location: "test/controllers/auth_controller_test.rb:105", Reliability: 0},
		{ID: "t-003", Scope: "AuthController#logout", Name: "clears session on logout", Location: "test/controllers/auth_controller_test.rb:142", Reliability: 0},
		{ID: "t-004", Scope: "TokenService#refresh", Name: "raises on revoked token", Location: "test/services/token_service_test.rb:42", Reliability: 0.12},
		{ID: "t-005", Scope: "TokenService#validate", Name: "rejects tampered signature", Location: "test/services/token_service_test.rb:78", Reliability: 0},
		{ID: "t-006", Scope: "SessionStore#destroy", Name: "handles concurrent deletion", Location: "test/services/session_store_test.rb:63", Reliability: 0.45},
		{ID: "t-007", Scope: "SessionStore#create", Name: "validates TTL bounds", Location: "test/services/session_store_test.rb:88", Reliability: 0},
		{ID: "t-008", Scope: "Worker::RetryHandler#call", Name: "retries on timeout", Location: "app/workers/retry_handler_test.rb:33", Reliability: 0.78},
		{ID: "t-009", Scope: "Worker::RetryHandler#call", Name: "respects max attempts", Location: "app/workers/retry_handler_test.rb:51", Reliability: 0},
		{ID: "t-010", Scope: "Worker::DeadLetterQueue#push", Name: "rejects oversized payload", Location: "app/workers/dead_letter_queue_test.rb:19", Reliability: 0},
		{ID: "t-011", Scope: "CacheWarmer#run", Name: "skips stale entries", Location: "test/services/cache_warmer_test.rb:27", Reliability: 0},
		{ID: "t-012", Scope: "CacheWarmer#run", Name: "respects TTL", Location: "test/services/cache_warmer_test.rb:44", Reliability: 0.33},
	}
}
