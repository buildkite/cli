package io

import "github.com/AlecAivazis/survey/v2"

// PromptForOne will show the list of options to the user, allowing them to select one to return.
// It's possible for them to choose none or cancel the selection, resulting in an error.
func PromptForOne(options []string) (string, error) {
	prompt := &survey.Select{
		Options: options,
	}
	selected := new(string)
	err := survey.AskOne(prompt, selected)

	return *selected, err
}
