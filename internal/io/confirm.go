package io

import (
	"fmt"
	"strings"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
)

// Confirm prompts the user with a yes/no question.
// Returns true if the user confirmed, false otherwise.
//
// IMPORTANT: Commands using Confirm() must call f.SetGlobalFlags(cmd) in PreRunE.
// See factory.SetGlobalFlags() documentation for details.
//
// Usage:
//
//	confirmed, err := io.Confirm(f, "Do the thing?")
//	if err != nil {
//	    return err
//	}
//	if confirmed {
//	    // do the thing
//	}
func Confirm(f *factory.Factory, prompt string) (bool, error) {
	// Check if --yes flag is set
	if f.SkipConfirm {
		return true, nil
	}

	// Check if --no-input flag is set
	if f.NoInput {
		return false, fmt.Errorf("interactive input required but --no-input is set")
	}

	fmt.Printf("%s [y/N]: ", prompt)

	response, err := ReadLine()
	if err != nil {
		return false, err
	}

	response = strings.ToLower(response)
	return response == "y" || response == "yes", nil
}
