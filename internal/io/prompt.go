package io

import "github.com/charmbracelet/huh"

const (
	typeOrganizationMessage = "Pick an organization"
	typePipelineMessage     = "Select a pipeline"
)

// PromptForOne will show the list of options to the user, allowing them to select one to return.
// It's possible for them to choose none or cancel the selection, resulting in an error.
func PromptForOne(resource string, options []string) (string, error) {
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
