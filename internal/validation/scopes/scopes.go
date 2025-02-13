package scopes

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

type CommandScopes struct {
	Required []Scope
}

func ValidateCommandScopes(cmd *cobra.Command, tokenScopes []string) error {
	commandScopes := GetCommandScopes(cmd)
	return ValidateScopes(commandScopes, tokenScopes)
}

func GetCommandScopes(cmd *cobra.Command) CommandScopes {
	required := []Scope{}

	if reqScopes, ok := cmd.Annotations["requiredScopes"]; ok {
		for _, scope := range strings.Split(reqScopes, ",") {
			required = append(required, Scope(strings.TrimSpace(scope)))
		}
	}

	return CommandScopes{
		Required: required,
	}
}

func ValidateScopes(cmdScopes CommandScopes, tokenScopes []string) error {
	missingRequired := []string{}

	for _, requiredScope := range cmdScopes.Required {
		found := false
		for _, tokenScope := range tokenScopes {
			if string(requiredScope) == tokenScope {
				found = true
				break
			}
		}
		if !found {
			missingRequired = append(missingRequired, string(requiredScope))
		}
	}

	if len(missingRequired) > 0 {
		return fmt.Errorf("missing required scopes: %s", strings.Join(missingRequired, ", "))
	}

	return nil
}
