// Package cursor provides ANSI Escape sequences
// introduced in VT-100 terminals for cursor and screen
// manipulation operations.
package cursor

import "fmt"

var Esc = "\x1b"

func escape(format string, args ...interface{}) string {
	return fmt.Sprintf("%s%s", Esc, fmt.Sprintf(format, args...))
}

// Show returns ANSI escape sequence to show the cursor
func Show() string {
	return escape("[?25h")
}

// Hide returns ANSI escape sequence to hide the cursor
func Hide() string {
	return escape("[?25l")
}

// MoveTo returns ANSI escape sequence to move cursor
// to specified position on screen.
func MoveTo(line, col int) string {
	return escape("[%d;%dH", line, col)
}

// MoveUp returns ANSI escape sequence to move cursor
// up n lines.
func MoveUp(n int) string {
	return escape("[%dA", n)
}

// MoveDown returns ANSI escape sequence to move cursor
// down n lines.
func MoveDown(n int) string {
	return escape("[%dB", n)
}

// MoveRight returns ANSI escape sequence to move cursor
// right n columns.
func MoveRight(n int) string {
	return escape("[%dC", n)
}

// MoveLeft returns ANSI escape sequence to move cursor
// left n columns.
func MoveLeft(n int) string {
	return escape("[%dD", n)
}

// MoveUpperLeft returns ANSI escape sequence to move cursor
// to upper left corner of screen.
func MoveUpperLeft(n int) string {
	return escape("[%dH", n)
}

// MoveNextLine returns ANSI escape sequence to move cursor
// to next line.
func MoveNextLine() string {
	return escape("E")
}

// ClearLineRight returns ANSI escape sequence to clear line
// from right of the cursor.
func ClearLineRight() string {
	return escape("[0K")
}

// ClearLineLeft returns ANSI escape sequence to clear line
// from left of the cursor.
func ClearLineLeft() string {
	return escape("[1K")
}

// ClearEntireLine returns ANSI escape sequence to clear current line.
func ClearEntireLine() string {
	return escape("[2K")
}

// ClearScreenDown returns ANSI escape sequence to clear screen below.
// the cursor.
func ClearScreenDown() string {
	return escape("[0J")
}

// ClearScreenUp returns ANSI escape sequence to clear screen above.
// the cursor.
func ClearScreenUp() string {
	return escape("[1J")
}

// ClearEntireScreen returns ANSI escape sequence to clear the screen.
func ClearEntireScreen() string {
	return escape("[2J")
}

// SaveAttributes returns ANSI escape sequence to save current position
// and attributes of the cursor.
func SaveAttributes() string {
	return escape("7")
}

// RestoreAttributes returns ANSI escape sequence to restore saved position
// and attributes of the cursor.
func RestoreAttributes() string {
	return escape("8")
}
