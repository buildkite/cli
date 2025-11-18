package io

import (
	"fmt"
	"strings"
)

// Confirm prompts the user with a yes/no question.
//
// IMPORTANT: If your command uses Confirm(), you MUST call f.SetGlobalFlags(cmd) in PreRunE:
//
//	PreRunE: func(cmd *cobra.Command, args []string) error {
//	    f.SetGlobalFlags(cmd)  // Required for --yes and --no-input support
//	    // ... rest of your logic
//	}
//
// Then use factory flags in RunE:
//
//	confirmed := f.SkipConfirm  // Respects global --yes flag
//	io.Confirm(&confirmed, "Do the thing?")
//
// Do NOT add individual --yes flags to commands. Use the global flag pattern above.
func Confirm(confirmed *bool, title string) error {
	if *confirmed {
		return nil
	}

	fmt.Printf("%s [y/N]: ", title)
	var response string
	_, _ = fmt.Scanln(&response)

	response = strings.ToLower(strings.TrimSpace(response))
	*confirmed = response == "y" || response == "yes"

	return nil
}
