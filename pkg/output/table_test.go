package output

import (
	"strings"
	"testing"
	"unicode/utf8"
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

func TestTrimToWidthPreservesANSICodes(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		width       int
		shouldMatch func(string) bool
	}{
		{
			name:  "ANSI code at start",
			input: "\x1b[31mHello World\x1b[0m",
			width: 5,
			shouldMatch: func(result string) bool {
				return strings.HasPrefix(result, "\x1b[31m") && strings.Contains(result, "Hello")
			},
		},
		{
			name:  "ANSI code in middle - preserves codes before truncation",
			input: "Hello \x1b[31mWorld\x1b[0m",
			width: 8,
			shouldMatch: func(result string) bool {
				// Should contain "Hello " and the color code, with "Wo"
				return strings.Contains(result, "Hello") && strings.Contains(result, "\x1b[31m") && strings.Contains(result, "Wo")
			},
		},
		{
			name:  "Multiple ANSI codes",
			input: "\x1b[1m\x1b[31mBold Red\x1b[0m",
			width: 4,
			shouldMatch: func(result string) bool {
				return strings.Contains(result, "\x1b[1m") && strings.Contains(result, "\x1b[31m") && strings.Contains(result, "Bold")
			},
		},
		{
			name:  "ANSI code after truncation point - not included",
			input: "Hello\x1b[31m World\x1b[0m",
			width: 5,
			shouldMatch: func(result string) bool {
				return result == "Hello" || result == "Hello\x1b[31m"
			},
		},
		{
			name:  "Wide characters with ANSI",
			input: "\x1b[32m你好世界\x1b[0m",
			width: 4,
			shouldMatch: func(result string) bool {
				return strings.HasPrefix(result, "\x1b[32m") && strings.Contains(result, "你好")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := trimToWidth(tt.input, tt.width)

			if !tt.shouldMatch(result) {
				t.Errorf("trimToWidth(%q, %d) = %q failed validation", tt.input, tt.width, result)
			}

			if strings.Contains(result, "\x1b") {
				matches := ansiPattern.FindAllString(result, -1)

				if strings.Count(result, "\x1b") != len(matches) {
					t.Errorf("Result contains broken ANSI sequences: %q (found %d escape chars but %d complete sequences)",
						result, strings.Count(result, "\x1b"), len(matches))
				}
			}

			actualWidth := displayWidth(result)
			if actualWidth > tt.width {
				t.Errorf("Result width %d exceeds requested width %d", actualWidth, tt.width)
			}
		})
	}
}

func TestTableHandlesComplexUnicode(t *testing.T) {
	tests := []struct {
		name    string
		headers []string
		rows    [][]string
		verify  func(*testing.T, string)
	}{
		{
			name:    "Emoji with zero-width joiners",
			headers: []string{"Family", "Description"},
			rows: [][]string{
				{"👨‍👩‍👧‍👦", "Family with kids"},
				{"👩‍💻", "Woman technologist"},
			},
			verify: func(t *testing.T, result string) {
				if !strings.Contains(result, "👨‍👩‍👧‍👦") {
					t.Error("Family emoji with ZWJ missing from output")
				}
				if !strings.Contains(result, "👩‍💻") {
					t.Error("Woman technologist emoji missing from output")
				}
			},
		},
		{
			name:    "Right-to-left text (Hebrew)",
			headers: []string{"Hebrew", "English"},
			rows: [][]string{
				{"שלום", "Hello"},
				{"עברית", "Hebrew"},
			},
			verify: func(t *testing.T, result string) {
				if !strings.Contains(result, "שלום") {
					t.Error("Hebrew text missing from output")
				}
				if !strings.Contains(result, "עברית") {
					t.Error("Hebrew word for 'Hebrew' missing from output")
				}
			},
		},
		{
			name:    "Right-to-left text (Arabic)",
			headers: []string{"Arabic", "English"},
			rows: [][]string{
				{"مرحبا", "Hello"},
				{"العربية", "Arabic"},
			},
			verify: func(t *testing.T, result string) {
				if !strings.Contains(result, "مرحبا") {
					t.Error("Arabic text missing from output")
				}
				if !strings.Contains(result, "العربية") {
					t.Error("Arabic word for 'Arabic' missing from output")
				}
			},
		},
		{
			name:    "Combining diacritical marks",
			headers: []string{"Text", "Type"},
			rows: [][]string{
				{"café", "Precomposed"},
				{"café", "Combining marks"},
				{"ñ vs ñ", "Different forms"},
			},
			verify: func(t *testing.T, result string) {
				// Both forms of café should be present
				cafeCount := strings.Count(result, "café")
				if cafeCount < 1 {
					t.Errorf("Expected at least one 'café', got %d occurrences", cafeCount)
				}
			},
		},
		{
			name:    "Mixed emoji and text",
			headers: []string{"Status", "Message"},
			rows: [][]string{
				{"✅", "Success"},
				{"❌", "Failed"},
				{"⚠️", "Warning"},
			},
			verify: func(t *testing.T, result string) {
				if !strings.Contains(result, "✅") {
					t.Error("Check mark emoji missing")
				}
				if !strings.Contains(result, "❌") {
					t.Error("Cross mark emoji missing")
				}
				if !strings.Contains(result, "⚠") {
					t.Error("Warning emoji missing")
				}
			},
		},
		{
			name:    "Skin tone modifiers",
			headers: []string{"Emoji", "Description"},
			rows: [][]string{
				{"👋", "Wave (default)"},
				{"👋🏻", "Wave (light skin)"},
				{"👋🏿", "Wave (dark skin)"},
			},
			verify: func(t *testing.T, result string) {
				if !strings.Contains(result, "👋") {
					t.Error("Wave emoji missing")
				}
			},
		},
		{
			name:    "Regional indicator symbols (flags)",
			headers: []string{"Flag", "Country"},
			rows: [][]string{
				{"🇺🇸", "United States"},
				{"🇬🇧", "United Kingdom"},
				{"🇯🇵", "Japan"},
			},
			verify: func(t *testing.T, result string) {
				// Flags are made of regional indicator pairs
				if !strings.Contains(result, "🇺🇸") {
					t.Error("US flag emoji missing")
				}
			},
		},
		{
			name:    "Variation selectors",
			headers: []string{"Char", "Type"},
			rows: [][]string{
				{"♠", "Text style"},
				{"♠️", "Emoji style"},
			},
			verify: func(t *testing.T, result string) {
				if !strings.Contains(result, "♠") {
					t.Error("Spade symbol missing")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Table(tt.headers, tt.rows, nil)

			// Verify the table was generated
			if len(result) == 0 {
				t.Fatal("Table output is empty")
			}

			// Verify headers are present
			for _, header := range tt.headers {
				upperHeader := strings.ToUpper(header)
				if !strings.Contains(result, upperHeader) {
					t.Errorf("Header %q not found in output", upperHeader)
				}
			}

			// Run custom verification
			tt.verify(t, result)

			// Verify the output doesn't have broken formatting
			lines := strings.Split(strings.TrimSuffix(result, "\n"), "\n")
			if len(lines) != len(tt.rows)+1 {
				t.Errorf("Expected %d lines (1 header + %d rows), got %d", len(tt.rows)+1, len(tt.rows), len(lines))
			}
		})
	}
}

func TestTrimToWidthWithComplexUnicode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		width    int
		minWidth int // minimum acceptable width of result
	}{
		{
			name:     "Emoji with ZWJ truncation",
			input:    "Hello 👨‍👩‍👧‍👦 World",
			width:    10,
			minWidth: 0,
		},
		{
			name:     "Arabic text truncation",
			input:    "مرحبا بك في العالم",
			width:    8,
			minWidth: 0,
		},
		{
			name:     "Hebrew text truncation",
			input:    "שלום עולם מקסים",
			width:    6,
			minWidth: 0,
		},
		{
			name:     "Combined diacritics truncation",
			input:    "Café résumé naïve",
			width:    8,
			minWidth: 0,
		},
		{
			name:     "Flag emoji truncation",
			input:    "USA: 🇺🇸 UK: 🇬🇧 JP: 🇯🇵",
			width:    12,
			minWidth: 0,
		},
		{
			name:     "Skin tone modifiers truncation",
			input:    "👋 👋🏻 👋🏿",
			width:    6,
			minWidth: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := trimToWidth(tt.input, tt.width)

			actualWidth := displayWidth(result)
			if actualWidth > tt.width {
				t.Errorf("Result width %d exceeds max width %d (input: %q, result: %q)",
					actualWidth, tt.width, tt.input, result)
			}

			if actualWidth < tt.minWidth {
				t.Errorf("Result width %d is less than min width %d (result: %q)",
					actualWidth, tt.minWidth, result)
			}

			// Verify we didn't create invalid UTF-8
			if !utf8.ValidString(result) {
				t.Errorf("Result contains invalid UTF-8: %q", result)
			}
		})
	}
}
