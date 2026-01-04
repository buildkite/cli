package output

import (
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-isatty"
	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
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
	minColumnWidth    = 3
	ellipsisWidth     = 3
	defaultTableWidth = 120
)

// ansiPattern strips ANSI/OSC escape sequences
var ansiPattern = regexp.MustCompile(`\x1b(?:\[[0-9;?]*[ -/]*[@-~]|\][^\a]*(?:\a|\x1b\\)|[P_\]^][^\x1b]*\x1b\\)`)

func Table(headers []string, rows [][]string, columnStyles map[string]string) string {
	if len(headers) == 0 {
		return ""
	}

	useColor := ColorEnabled()
	maxWidth := detectedTableWidth()

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

	colWidths := make([]int, len(headers))
	for i, header := range upperHeaders {
		colWidths[i] = displayWidth(header)
	}

	for _, row := range rows {
		for i := 0; i < len(row) && i < len(colWidths); i++ {
			if width := displayWidth(row[i]); width > colWidths[i] {
				colWidths[i] = width
			}
		}
	}

	colWidths = clampColumnWidths(colWidths, len(headers), len(colSeparator), maxWidth)

	totalWidth := 0
	for _, width := range colWidths {
		totalWidth += width + len(colSeparator)
	}
	const maxEstimatedSize = 1 << 20
	estimatedSize := totalWidth * (len(rows) + 1)
	if estimatedSize < 0 || estimatedSize > maxEstimatedSize {
		estimatedSize = maxEstimatedSize
	}
	var builder strings.Builder
	builder.Grow(estimatedSize)

	for i, upperHeader := range upperHeaders {
		if useColor {
			builder.WriteString(ansiDimUnder)
		}
		writePadded(&builder, truncateToWidth(upperHeader, colWidths[i]), colWidths[i])
		if useColor {
			builder.WriteString(ansiReset)
		}
		if i < len(headers)-1 && colWidths[i] > 0 {
			builder.WriteString(colSeparator)
		}
	}
	builder.WriteString("\n")

	for _, row := range rows {
		for i := range headers {
			value := ""
			if i < len(row) {
				value = row[i]
			}

			value = truncateToWidth(value, colWidths[i])

			if colStyles[i] != "" {
				builder.WriteString(colStyles[i])
			}

			writePadded(&builder, value, colWidths[i])

			if colStyles[i] != "" {
				builder.WriteString(ansiReset)
			}

			if i < len(headers)-1 && colWidths[i] > 0 {
				builder.WriteString(colSeparator)
			}
		}
		builder.WriteString("\n")
	}

	return builder.String()
}

func displayWidth(s string) int {
	stripped := ansiPattern.ReplaceAllString(s, "")
	return runewidth.StringWidth(stripped)
}

func writePadded(builder *strings.Builder, s string, width int) {
	visible := displayWidth(s)
	builder.WriteString(s)
	for i := visible; i < width; i++ {
		builder.WriteByte(' ')
	}
}

func truncateToWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}

	if displayWidth(s) <= width {
		return s
	}

	if width <= ellipsisWidth {
		return trimToWidth(s, width)
	}

	trimmed := trimToWidth(s, width-ellipsisWidth)
	return trimmed + "..."
}

func trimToWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}

	stripped := ansiPattern.ReplaceAllString(s, "")
	if runewidth.StringWidth(stripped) <= width {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))

	currentWidth := 0
	i := 0

	for i < len(s) {
		if s[i] == '\x1b' {
			if loc := ansiPattern.FindStringIndex(s[i:]); loc != nil && loc[0] == 0 {
				b.WriteString(s[i : i+loc[1]])
				i += loc[1]
				continue
			}
		}

		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError {
			break
		}

		rw := runewidth.RuneWidth(r)
		if rw == 0 {
			b.WriteString(s[i : i+size])
			i += size
			continue
		}

		if currentWidth+rw > width {
			break
		}

		b.WriteString(s[i : i+size])
		currentWidth += rw
		i += size
	}

	return b.String()
}

func clampColumnWidths(colWidths []int, colCount, separatorWidth, maxWidth int) []int {
	if maxWidth <= 0 || colCount == 0 {
		return colWidths
	}

	sepTotal := (colCount - 1) * separatorWidth
	if sepTotal >= maxWidth {
		clamped := make([]int, len(colWidths))
		return clamped
	}

	available := maxWidth - sepTotal
	sum := 0
	for _, width := range colWidths {
		sum += width
	}

	if sum <= available {
		return colWidths
	}

	clamped := make([]int, len(colWidths))
	if sum == 0 {
		for i := range clamped {
			clamped[i] = minColumnWidth
		}
		return clamped
	}

	effectiveMin := minColumnWidth
	if available < minColumnWidth*colCount {
		effectiveMin = available / colCount
	}
	// First pass: give narrow columns their full width, mark wide columns for proportional allocation
	// A column is "narrow" if it fits within its fair share of space
	fairShare := available / colCount
	narrowThreshold := fairShare * 2

	fixed := make([]bool, len(colWidths))
	remainingSpace := available
	flexSum := 0

	for i, width := range colWidths {
		if width <= narrowThreshold {
			// This column is narrow enough to get its full width
			clamped[i] = width
			fixed[i] = true
			remainingSpace -= width
		} else {
			// This column needs proportional allocation
			flexSum += width
		}
	}

	// Second pass: proportionally allocate remaining space to flexible columns
	for i, width := range colWidths {
		if fixed[i] {
			continue
		}
		if flexSum == 0 {
			clamped[i] = effectiveMin
			continue
		}
		ratio := float64(width) / float64(flexSum)
		alloc := int(math.Floor(ratio * float64(remainingSpace)))
		clamped[i] = max(alloc, effectiveMin)
	}

	currentTotal := 0
	for _, width := range clamped {
		currentTotal += width
	}

	remaining := available - currentTotal

	type columnIndex struct {
		originalWidth int
		index         int
	}

	indices := make([]columnIndex, len(colWidths))
	for i, width := range colWidths {
		indices[i] = columnIndex{originalWidth: width, index: i}
	}

	sort.Slice(indices, func(i, j int) bool {
		return indices[i].originalWidth > indices[j].originalWidth
	})

	if remaining > 0 {
		for remaining > 0 {
			for _, col := range indices {
				if remaining == 0 {
					break
				}
				clamped[col.index]++
				remaining--
			}
		}
	} else if remaining < 0 {
		for _, col := range indices {
			if remaining == 0 {
				break
			}
			reduction := -remaining
			if reduction > clamped[col.index] {
				reduction = clamped[col.index]
			}
			clamped[col.index] -= reduction
			remaining += reduction
		}
	}

	return clamped
}

func detectedTableWidth() int {
	if override := os.Getenv("BUILDKITE_TABLE_MAX_WIDTH"); override != "" {
		if parsed, err := strconv.Atoi(strings.TrimSpace(override)); err == nil && parsed > 0 {
			return parsed
		}
	}

	fd := os.Stdout.Fd()
	if !isatty.IsTerminal(fd) && !isatty.IsCygwinTerminal(fd) {
		return defaultTableWidth
	}

	width, _, err := term.GetSize(int(fd))
	if err != nil || width <= 0 {
		return defaultTableWidth
	}

	return width
}
