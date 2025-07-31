package io

import (
	"errors"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/mattn/go-isatty"
)

const (
	typeOrganizationMessage = "Pick an organization"
	typePipelineMessage     = "Select a pipeline"
)

// PromptForOne will show the list of options to the user, allowing them to select one to return.
// It's possible for them to choose none or cancel the selection, resulting in an error.
func PromptForOne(resource string, options []string) (string, error) {
	// If no TTY is available, cannot prompt user for selection
	if !isatty.IsTerminal(os.Stdout.Fd()) {
		return "", errors.New("cannot prompt for selection: no TTY available (use appropriate flags to specify the selection)")
	}

	var message string
	switch resource {
	case "pipeline":
		message = typePipelineMessage
	case "organization":
		message = typeOrganizationMessage
	default:
		message = "Please select one of the options below"
	}
	selected := new(string)
	err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title(message).
			Options(
				huh.NewOptions(options...)...,
			).Value(selected),
	),
	).Run()
	return *selected, err
}
