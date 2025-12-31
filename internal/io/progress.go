package io

import (
	"fmt"
	"strings"
)

func ProgressBar(completed, total, width int) string {
	if width <= 0 {
		return "[]"
	}
	if total <= 0 {
		return "[" + strings.Repeat("░", width) + "]"
	}

	if completed < 0 {
		completed = 0
	}

	filled := min(completed*width/total, width)

	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", width-filled) + "]"
}

func ProgressLine(label string, completed, total, succeeded, failed, barWidth int) string {
	if total == 0 {
		return fmt.Sprintf("%s [no items]", label)
	}

	bar := ProgressBar(completed, total, barWidth)
	percent := min(completed*100/total, 100)

	return fmt.Sprintf("%s %s %3d%% %d/%d succeeded:%d failed:%d", label, bar, percent, completed, total, succeeded, failed)
}
