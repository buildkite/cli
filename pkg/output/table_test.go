package output

import (
	"strings"
	"testing"
)

func TestTableTruncatesWhenWidthExceeded(t *testing.T) {
	t.Setenv("BUILDKITE_TABLE_MAX_WIDTH", "20")

	headers := []string{"Col1", "Col2"}
	rows := [][]string{{"this-is-a-very-long-value", "short"}}

	table := Table(headers, rows, nil)

	lines := strings.Split(strings.TrimSuffix(table, "\n"), "\n")
	for i, line := range lines {
		if displayWidth(line) > 20 {
			t.Fatalf("line %d exceeds max width: %d > 20", i, displayWidth(line))
		}
	}

	if !strings.Contains(table, "...") {
		t.Fatalf("expected truncated output to contain ellipsis")
	}
}

func TestTableProportionalClampPreservesShortColumn(t *testing.T) {
	t.Setenv("BUILDKITE_TABLE_MAX_WIDTH", "30")

	headers := []string{"Short", "Longer"}
	rows := [][]string{{"ok", "this-is-a-very-long-value-that-should-truncate"}}

	table := Table(headers, rows, nil)

	lines := strings.Split(strings.TrimSuffix(table, "\n"), "\n")
	for i, line := range lines {
		if displayWidth(line) > 30 {
			t.Fatalf("line %d exceeds max width: %d > 30", i, displayWidth(line))
		}
	}

	if strings.Contains(lines[1], "ok...") {
		t.Fatalf("short column should not be truncated: %q", lines[1])
	}

	if !strings.Contains(lines[1], "...") {
		t.Fatalf("long column should be truncated with ellipsis")
	}
}

func TestTableRespectsNoTruncationWhenWidthIsLarge(t *testing.T) {
	t.Setenv("BUILDKITE_TABLE_MAX_WIDTH", "200")

	headers := []string{"Col1", "Col2"}
	rows := [][]string{{"alpha", "beta"}}

	table := Table(headers, rows, nil)

	if !strings.Contains(table, "alpha") || !strings.Contains(table, "beta") {
		t.Fatalf("expected table to contain original values")
	}

	for _, line := range strings.Split(strings.TrimSuffix(table, "\n"), "\n") {
		if displayWidth(line) > 200 {
			t.Fatalf("line exceeds large width guard")
		}
	}
}

func TestTableFitsWhenMaxWidthSmallerThanColumnCount(t *testing.T) {
	t.Setenv("BUILDKITE_TABLE_MAX_WIDTH", "20")

	headers := []string{"A", "B", "C", "D"}
	rows := [][]string{{"val1", "val2", "val3", "val4"}}

	table := Table(headers, rows, nil)

	lines := strings.Split(strings.TrimSuffix(table, "\n"), "\n")
	for i, line := range lines {
		width := displayWidth(line)
		if width > 20 {
			t.Fatalf("line %d exceeds max width: %d > 20 (content: %q)", i, width, line)
		}
	}

	if len(table) == 0 {
		t.Fatalf("table should not be empty even with severe constraints")
	}
}

func TestTableFitsWhenSeparatorsExceedMaxWidth(t *testing.T) {
	t.Setenv("BUILDKITE_TABLE_MAX_WIDTH", "5")

	headers := []string{"A", "B", "C"}
	rows := [][]string{{"x", "y", "z"}}

	table := Table(headers, rows, nil)

	lines := strings.Split(strings.TrimSuffix(table, "\n"), "\n")
	for i, line := range lines {
		width := displayWidth(line)
		if width > 5 {
			t.Fatalf("line %d exceeds max width: %d > 5 (content: %q)", i, width, line)
		}
	}

	if len(table) == 0 {
		t.Fatalf("table should render even when separators exceed width")
	}
}

