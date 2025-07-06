package io

import (
	"os"

	"github.com/charmbracelet/huh/spinner"
	"github.com/mattn/go-isatty"
)

func SpinWhile(name string, action func()) error {
	if !isatty.IsTerminal(os.Stdout.Fd()) {
		// No TTY available, just run the action without spinner
		action()
		return nil
	}

	// TTY is available, use the spinner
	return spinner.New().
		Title(name).
		Action(action).
		Run()
}
