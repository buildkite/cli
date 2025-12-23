package output

import (
	"regexp"
	"strings"

	"github.com/mattn/go-runewidth"
)

const (
	ansiReset         = "\033[0m"
	ansiBold          = "\033[1m"
	ansiDim           = "\033[2m"
	ansiItalic        = "\033[3m"
	ansiUnderline     = "\033[4m"
	ansiDimUnder      = "\033[2;4m"
	ansiStrikeThrough = "\033[9m"
	colSeparator      = "    "
)

// ansiPattern strips ANSI/OSC escape sequences
var ansiPattern = regexp.MustCompile(`\x1b(?:\[[0-9;?]*[ -/]*[@-~]|\][^\a]*(?:\a|\x1b\\)|[P_\]^][^\x1b]*\x1b\\)`)

func Table(headers []string, rows [][]string, columnStyles map[string]string) string {
	if len(headers) == 0 {
		return ""
	}

	useColor := ColorEnabled()

	upperHeaders := make([]string, len(headers))
	colStyles := make([]string, len(headers))
	for i, header := range headers {
		upperHeaders[i] = strings.ToUpper(header)

		style := columnStyles[strings.ToLower(header)]
		if style != "" && useColor {
			switch style {
			case "bold":
				colStyles[i] = ansiBold
			case "dim":
				colStyles[i] = ansiDim
			case "italic":
				colStyles[i] = ansiItalic
			case "underline":
				colStyles[i] = ansiUnderline
			case "strikethrough":
				colStyles[i] = ansiStrikeThrough
			default:
				colStyles[i] = ""
			}
		}
	}

	// Start widths from rendered headers
	colWidths := make([]int, len(headers))
	for i, header := range upperHeaders {
		colWidths[i] = displayWidth(header)
	}

	// Ensure widths grow based on content
	for _, row := range rows {
		for i := 0; i < len(row) && i < len(colWidths); i++ {
			if w := displayWidth(row[i]); w > colWidths[i] {
				colWidths[i] = w
			}
		}
	}

	// Roughly size the buffer to avoid extra allocations
	totalWidth := 0
	for _, w := range colWidths {
		totalWidth += w + len(colSeparator) // width + separator
	}
	estimatedSize := totalWidth * (len(rows) + 1) // headers + all rows
	var sb strings.Builder
	sb.Grow(estimatedSize)

	for i, upperHeader := range upperHeaders {
		if useColor {
			sb.WriteString(ansiDimUnder)
		}
		writePadded(&sb, upperHeader, colWidths[i])
		if useColor {
			sb.WriteString(ansiReset)
		}
		if i < len(headers)-1 {
			sb.WriteString(colSeparator)
		}
	}
	sb.WriteString("\n")

	for _, row := range rows {
		for i := range headers {
			value := ""
			if i < len(row) {
				value = row[i]
			}

			if colStyles[i] != "" {
				sb.WriteString(colStyles[i])
			}

			writePadded(&sb, value, colWidths[i])

			if colStyles[i] != "" {
				sb.WriteString(ansiReset)
			}

			if i < len(headers)-1 {
				sb.WriteString(colSeparator)
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// displayWidth returns visible width without escape codes.
func displayWidth(s string) int {
	stripped := ansiPattern.ReplaceAllString(s, "")
	return runewidth.StringWidth(stripped)
}

// writePadded writes s and pads based on visible width.
func writePadded(sb *strings.Builder, s string, width int) {
	visible := displayWidth(s)
	sb.WriteString(s)
	for i := visible; i < width; i++ {
		sb.WriteByte(' ')
	}
}
