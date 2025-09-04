package io

import (
	"os"

	"github.com/charmbracelet/huh"
	"github.com/mattn/go-isatty"
)

func Confirm(confirmed *bool, title string) error {
	// If already confirmed via flag, skip the prompt
	if *confirmed {
		return nil
	}

	// If no TTY is available, default to confirmed=true (non-interactive mode)
	if !isatty.IsTerminal(os.Stdout.Fd()) {
		*confirmed = true
		return nil
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(title).
				Affirmative("Yes").
				Negative("No").
				Value(confirmed),
		).WithHide(*confirmed), // user can bypass the prompt by passing the flag
	)

	err := form.Run()

	// no need to return error if ctrl-c
	if err != nil && err == huh.ErrUserAborted {
		return nil
	}

	return err
}
