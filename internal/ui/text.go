package ui

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// TruncateText truncates text to the specified length and adds an ellipsis
func TruncateText(text string, maxLength int) string {
	if len(text) <= maxLength {
		return text
	}
	return text[:maxLength] + IconEllipsis
}

// TrimMessage trims a multi-line message to the first line
func TrimMessage(msg string) string {
	if idx := strings.Index(msg, "\n"); idx != -1 {
		return msg[:idx] + IconEllipsis
	}
	return msg
}

// StripHTMLTags removes HTML tags from a string
func StripHTMLTags(html string) string {
	// Remove closing tags first
	re := regexp.MustCompile(`</[^>]+>`)
	html = re.ReplaceAllString(html, "")

	// Then remove opening tags
	re = regexp.MustCompile(`<[^>]*>`)
	html = re.ReplaceAllString(html, "")

	return html
}

// TruncateAndStripTags strips HTML tags and truncates text
func TruncateAndStripTags(html string, maxLength int) string {
	text := StripHTMLTags(html)
	return TruncateText(text, maxLength)
}

// FormatBytes formats bytes into human-readable format (KB, MB, GB, etc.)
func FormatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
		TB = 1024 * GB
	)

	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.1fTB", float64(bytes)/TB)
	case bytes >= GB:
		return fmt.Sprintf("%.1fGB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.1fMB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.1fKB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

// FormatDuration formats a duration in a human-readable way
func FormatDuration(d time.Duration) string {
	if d == 0 {
		return ""
	}
	return d.String()
}

// FormatDate formats a time in RFC3339 format
func FormatDate(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}
