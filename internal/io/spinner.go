package io

import (
	"os"

	"github.com/charmbracelet/huh/spinner"
	"github.com/mattn/go-isatty"
)

func SpinWhile(name string, action func()) error {
	if isatty.IsTerminal(os.Stdout.Fd()) {
		action()

		return nil
	}

	return spinner.New().
		Title(name).
		Action(action).
		Run()
}
