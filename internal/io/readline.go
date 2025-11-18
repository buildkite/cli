package io

import (
	"bufio"
	"os"
	"strings"
	"syscall"

	"golang.org/x/term"
)

// ReadLine reads a line of input from stdin with terminal support.
// If running in a TTY, it uses x/term for better line editing (backspace, arrows, etc.).
// If not in a TTY (e.g., piped input), it falls back to bufio.
func ReadLine() (string, error) {
	fd := int(os.Stdin.Fd())

	// Check if we're in a TTY
	if !term.IsTerminal(fd) {
		// Not a TTY, use simple bufio
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(line), nil
	}

	// TTY - use x/term for better editing
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		// Fallback to bufio if raw mode fails
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(line), nil
	}

	terminal := term.NewTerminal(os.Stdin, "")
	line, err := terminal.ReadLine()
	_ = term.Restore(fd, oldState)

	if err != nil {
		return "", err
	}

	return strings.TrimSpace(line), nil
}

// ReadPassword reads a password from stdin without echoing.
// This is a convenience wrapper around term.ReadPassword.
func ReadPassword() (string, error) {
	passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", err
	}
	return string(passwordBytes), nil
}
