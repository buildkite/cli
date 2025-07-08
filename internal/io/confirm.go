package io

import (
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/mattn/go-isatty"
)

func Confirm(confirmed *bool, title string) error {
	// If already confirmed via flag, skip the prompt
	if *confirmed {
		return nil
	}

	// In non-TTY environments, fail by default with yes flag message
	if !isatty.IsTerminal(os.Stdout.Fd()) {
		return fmt.Errorf("confirmation required but not running in a terminal; use -y or --yes to confirm")
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(title).
				Affirmative("Yes").
				Negative("No").
				Value(confirmed),
		),
	)

	err := form.Run()

	// no need to return error if ctrl-c
	if err != nil && err == huh.ErrUserAborted {
		return nil
	}

	return err
}
