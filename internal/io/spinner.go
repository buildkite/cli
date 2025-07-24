package io

import (
	"os"

	"github.com/charmbracelet/huh/spinner"
	"github.com/mattn/go-isatty"
)

func SpinWhile(name string, action func()) error {
	return SpinWhileQuiet(name, action, false)
}

func SpinWhileQuiet(name string, action func(), quiet bool) error {
	if !isatty.IsTerminal(os.Stdout.Fd()) || quiet {
		// No TTY available or quiet mode requested, just run the action without spinner
		action()
		return nil
	}

	// TTY is available and not quiet mode, use the spinner
	return spinner.New().
		Title(name).
		Action(action).
		Run()
}
