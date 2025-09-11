package io

import (
	"github.com/charmbracelet/huh"
)

func Confirm(confirmed *bool, title string) error {
	if *confirmed {
		return nil
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
