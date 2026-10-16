package io

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/buildkite/cli/v3/internal/programstatus"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

const (
	typeOrganizationMessage = "Pick an organization"
	typePipelineMessage     = "Select a pipeline"
)

// PromptForOne will show the list of options to the user, allowing them to select one to return.
// It's possible for them to choose none or cancel the selection, resulting in an error.
// If noInput is true, it will fail instead of prompting.
// If there's no TTY available, it will also fail instead of prompting.
//
// For global flag support requirements, see the Confirm() function documentation.
func PromptForOne(resource string, options []string, noInput bool) (string, error) {
	return PromptForOneTo(os.Stdout, resource, options, noInput)
}

// PromptForOneTo sends the selection prompt to out, keeping structured output separate.
func PromptForOneTo(out io.Writer, resource string, options []string, noInput bool) (string, error) {
	if noInput {
		return "", fmt.Errorf("interactive input required but --no-input flag is set")
	}

	// Check if we have a TTY available - if not, treat it as if noInput is true
	if !isTerminal(os.Stdin) {
		return "", fmt.Errorf("interactive input required but no TTY available")
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

	fmt.Fprintf(out, "%s:\n", message)
	for i, option := range options {
		fmt.Fprintf(out, "  %d. %s\n", i+1, option)
	}
	prompt := fmt.Sprintf("Enter number (1-%d): ", len(options))
	fmt.Fprint(out, prompt)

	unblock := programstatus.Block(ansi.ProgramStatusKindQuestion, message)
	response, err := ReadLine()
	unblock()
	if err != nil {
		return "", err
	}
	num, err := strconv.Atoi(response)
	if err != nil || num < 1 || num > len(options) {
		return "", fmt.Errorf("invalid selection")
	}

	if file, ok := out.(*os.File); ok {
		clearPreviousLines(file, renderedLineCount(message, options, prompt+response, terminalWidth(file)))
	}

	return options[num-1], nil
}

func renderedLineCount(message string, options []string, prompt string, width int) int {
	lines := wrappedLineCount(message+":", width)
	for i, option := range options {
		lines += wrappedLineCount(fmt.Sprintf("  %d. %s", i+1, option), width)
	}
	lines += wrappedLineCount(prompt, width)
	return lines
}

func wrappedLineCount(s string, width int) int {
	if width <= 0 {
		return 1
	}

	lineWidth := runewidth.StringWidth(s)
	if lineWidth == 0 {
		return 1
	}

	return (lineWidth-1)/width + 1
}

// PromptForInput prompts the user to enter a string value.
// If a default value is provided, it will be shown in brackets and used if the user presses enter.
// If noInput is true, it will return the default value or an error if no default is provided.
func PromptForInput(prompt, defaultVal string, noInput bool) (string, error) {
	if noInput {
		if defaultVal != "" {
			return defaultVal, nil
		}
		return "", fmt.Errorf("interactive input required but --no-input flag is set")
	}

	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", prompt, defaultVal)
	} else {
		fmt.Printf("%s: ", prompt)
	}

	unblock := programstatus.Block(ansi.ProgramStatusKindQuestion, prompt)
	response, err := ReadLine()
	unblock()
	if err != nil {
		return "", err
	}

	response = strings.TrimSpace(response)
	if response == "" && defaultVal != "" {
		return defaultVal, nil
	}

	if response == "" {
		return "", fmt.Errorf("no value provided for %s", prompt)
	}

	return response, nil
}
