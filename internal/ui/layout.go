package ui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

// Section creates a titled section with content underneath
func Section(title string, content string) string {
	titleText := Header.Render(title)
	return lipgloss.JoinVertical(lipgloss.Top, titleText, content)
}

// Row creates a horizontal row of columns with consistent padding
func Row(columns ...string) string {
	var renderedColumns []string
	for _, col := range columns {
		renderedColumns = append(renderedColumns, Padding.Render(col))
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, renderedColumns...)
}

// LabeledValue creates a "Label: Value" formatted string
func LabeledValue(label string, value string) string {
	labelStyle := lipgloss.NewStyle().Width(15).Bold(true)
	return lipgloss.JoinHorizontal(
		lipgloss.Left,
		labelStyle.Render(label+":"),
		Padding.Render(value),
	)
}

// Table creates a table with the given headers and rows
func Table(headers []string, rows [][]string) string {
	t := table.New().
		Border(lipgloss.HiddenBorder()).
		Headers(headers...).
		StyleFunc(func(row, col int) lipgloss.Style {
			return lipgloss.NewStyle().PaddingRight(2)
		})
	
	for _, row := range rows {
		t.Row(row...)
	}
	
	return t.Render()
}

// Card creates a bordered card with title and content
func Card(title, content string, opts ...CardOption) string {
	options := processCardOptions(opts...)
	
	titleText := Title.Render(title)
	
	if options.bordered {
		border := BorderRounded.Copy()
		if options.borderColor != nil {
			border = border.BorderForeground(*options.borderColor)
		}
		return border.Render(lipgloss.JoinVertical(lipgloss.Top, titleText, content))
	}
	
	return lipgloss.JoinVertical(lipgloss.Top, titleText, content)
}

// CardOptions contains options for card rendering
type CardOptions struct {
	bordered    bool
	borderColor *lipgloss.Color
}

// CardOption is a function that modifies CardOptions
type CardOption func(*CardOptions)

// WithBorder adds a border to the card
func WithBorder(bordered bool) CardOption {
	return func(o *CardOptions) {
		o.bordered = bordered
	}
}

// WithBorderColor sets the border color
func WithBorderColor(color lipgloss.Color) CardOption {
	return func(o *CardOptions) {
		o.borderColor = &color
	}
}

// processCardOptions processes card options
func processCardOptions(opts ...CardOption) CardOptions {
	options := CardOptions{
		bordered: false,
	}
	for _, opt := range opts {
		opt(&options)
	}
	return options
}

// Divider creates a horizontal divider
func Divider() string {
	return lipgloss.NewStyle().
		BorderStyle(lipgloss.ThickBorder()).
		BorderTop(true).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		Render("")
}

// SpacedVertical joins strings vertically with a blank line between them
func SpacedVertical(strings ...string) string {
	if len(strings) == 0 {
		return ""
	}
	
	var result []string
	result = append(result, strings[0])
	
	for _, s := range strings[1:] {
		result = append(result, "", s) // Add blank line before each item
	}
	
	return lipgloss.JoinVertical(lipgloss.Top, result...)
}
