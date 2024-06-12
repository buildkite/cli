package io

import "github.com/charmbracelet/huh"

// PromptForOne will show the list of options to the user, allowing them to select one to return.
// It's possible for them to choose none or cancel the selection, resulting in an error.
func PromptForOne(options []string) (string, error) {
	selected := new(string)
	err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Pick an organization").
			Options(
				huh.NewOptions[string](options...)...,
			).Value(selected),
	),
	).Run()
	return *selected, err
}
