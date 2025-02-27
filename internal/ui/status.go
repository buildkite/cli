package ui

import (
	"github.com/charmbracelet/lipgloss"
)

// StatusStyle returns the appropriate styling for a status state
func StatusStyle(state string) lipgloss.Style {
	switch state {
	case "passed", "success":
		return lipgloss.NewStyle().Foreground(ColorSuccess)
	case "running", "scheduled":
		return lipgloss.NewStyle().Foreground(ColorRunning)
	case "failed", "failing", "error":
		return lipgloss.NewStyle().Foreground(ColorError)
	case "warning":
		return lipgloss.NewStyle().Foreground(ColorWarning)
	case "info":
		return lipgloss.NewStyle().Foreground(ColorInfo)
	default:
		return lipgloss.NewStyle().Foreground(ColorPending)
	}
}

// StatusIcon returns the appropriate icon for a status state
func StatusIcon(state string, opts ...StatusOption) string {
	options := processStatusOptions(opts...)

	var icon string
	switch state {
	case "passed", "success":
		icon = IconSuccess
	case "failed", "failing", "error":
		icon = IconError
	case "warning":
		icon = IconWarning
	case "info":
		icon = IconInfo
	case "running":
		icon = IconRunning
	case "scheduled", "pending":
		icon = IconPending
	case "canceled":
		icon = IconCanceled
	case "canceling":
		return IconCanceled + "(cancelling...)"
	case "blocked":
		return IconBlocked + "(Blocked)"
	case "unblocked":
		return IconUnblocked + "(Unblocked)"
	default:
		icon = IconDefault
	}

	if options.blocked && state == "passed" {
		return icon + "(blocked)"
	}

	return icon
}

// RenderStatus renders a status with the appropriate icon and styling
func RenderStatus(state string, opts ...StatusOption) string {
	style := StatusStyle(state)
	icon := StatusIcon(state, opts...)
	return style.Render(icon)
}

// StatusOptions contains options for status rendering
type StatusOptions struct {
	blocked bool
}

// StatusOption is a function that modifies StatusOptions
type StatusOption func(*StatusOptions)

// WithBlocked indicates the status is blocked
func WithBlocked(blocked bool) StatusOption {
	return func(o *StatusOptions) {
		o.blocked = blocked
	}
}

// processStatusOptions processes status options
func processStatusOptions(opts ...StatusOption) StatusOptions {
	options := StatusOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	return options
}
