package io

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	typeOrganizationMessage = "Pick an organization"
	typePipelineMessage     = "Select a pipeline"
)

// PromptForOne will show the list of options to the user, allowing them to select one to return.
// It's possible for them to choose none or cancel the selection, resulting in an error.
// If noInput is true, it will fail instead of prompting.
//
// For global flag support requirements, see the Confirm() function documentation.
func PromptForOne(resource string, options []string, noInput bool) (string, error) {
	if noInput {
		return "", fmt.Errorf("interactive input required but --no-input flag is set")
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

	if len(options) == 0 {
		return "", fmt.Errorf("no options available")
	}

	fmt.Printf("%s:\n", message)
	for i, option := range options {
		fmt.Printf("  %d. %s\n", i+1, option)
	}
	fmt.Printf("Enter number (1-%d): ", len(options))

	var response string
	fmt.Scanln(&response)

	response = strings.TrimSpace(response)
	num, err := strconv.Atoi(response)
	if err != nil || num < 1 || num > len(options) {
		return "", fmt.Errorf("invalid selection")
	}

	return options[num-1], nil
}
