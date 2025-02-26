package ui

import "github.com/charmbracelet/lipgloss"

// Color constants for consistent styling across the application
const (
	// Base colors
	ColorBlack  = lipgloss.Color("0")
	ColorRed    = lipgloss.Color("1")
	ColorGreen  = lipgloss.Color("2")
	ColorYellow = lipgloss.Color("3")
	ColorBlue   = lipgloss.Color("4")
	ColorPurple = lipgloss.Color("5")
	ColorCyan   = lipgloss.Color("6")
	ColorWhite  = lipgloss.Color("7")
	ColorGrey   = lipgloss.Color("8")

	// Semantic colors
	ColorSuccess     = lipgloss.Color("#2ECC40") // Green
	ColorError       = lipgloss.Color("#F45756") // Red
	ColorWarning     = lipgloss.Color("#FF841C") // Orange
	ColorInfo        = lipgloss.Color("#337AB7") // Blue
	ColorDefault     = lipgloss.Color("#DDD")    // Light Grey
	ColorRunning     = lipgloss.Color("#FF6E00") // Orange
	ColorPending     = lipgloss.Color("#5A5A5A") // Grey
	ColorPassedGreen = lipgloss.Color("#9dcc3a") // Bright Green
)

// Icon constants for consistent status representation
const (
	IconSuccess   = "✓"
	IconError     = "✖"
	IconWarning   = "⚠"
	IconInfo      = "ℹ"
	IconRunning   = "▶"
	IconPending   = "⏰"
	IconWaiting   = "⌛"
	IconCanceled  = "🚫"
	IconBlocked   = "🔒"
	IconUnblocked = "🔓"
	IconNote      = "🗒️"
	IconDefault   = "❔"
	IconEllipsis  = "…"
)

// Standard style variants
var (
	// Text styles
	Bold   = lipgloss.NewStyle().Bold(true)
	Italic = lipgloss.NewStyle().Italic(true)
	Faint  = lipgloss.NewStyle().Faint(true)

	// Layout styles
	Padding = lipgloss.NewStyle().Padding(0, 1)
	Header  = Bold.Copy().Padding(0, 1).Underline(true)
	Title   = Bold.Copy().Padding(0, 0)

	// Border styles
	BorderRounded = lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
)

// MaxPreviewLength is the maximum length for content previews
const MaxPreviewLength = 120
