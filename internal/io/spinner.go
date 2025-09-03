package io

import (
	"github.com/charmbracelet/huh/spinner"
)

func SpinWhile(name string, action func()) error {
	if !IsTerminal() {
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
