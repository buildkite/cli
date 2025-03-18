package scopes

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

// Scopes is a helper class to manipulate a list of scopes.
type Scopes []Scope

// NewScopes creates a new Scopes from the given list of scopes.
func NewScopes(scopes ...Scope) Scopes {
	return scopes
}

// NewScopesFromString a list of scopes separated by "," from string s.
func NewScopesFromString(s string) Scopes {
	var scopes Scopes
	for _, scope := range strings.Split(s, ",") {
		scopes = append(scopes, Scope(strings.TrimSpace(scope)))
	}
	return scopes
}

// String builds a string with the list of scopes joined by ",".
func (scopes Scopes) String() string {
	var strs []string
	for _, scope := range scopes {
		strs = append(strs, string(scope))
	}
	return strings.Join(strs, ",")
}

type CommandScopes struct {
	Required Scopes
}

func ValidateCommandScopes(cmd *cobra.Command, tokenScopes []string) error {
	commandScopes := GetCommandScopes(cmd)
	return ValidateScopes(commandScopes, tokenScopes)
}

func GetCommandScopes(cmd *cobra.Command) CommandScopes {
	required := Scopes{}

	if reqScopes, ok := cmd.Annotations["requiredScopes"]; ok {
		required = NewScopesFromString(reqScopes)
	}

	return CommandScopes{
		Required: required,
	}
}

func ValidateScopes(cmdScopes CommandScopes, tokenScopes []string) error {
	missingRequired := []string{}

	for _, requiredScope := range cmdScopes.Required {
		if !slices.Contains(tokenScopes, string(requiredScope)) {
			missingRequired = append(missingRequired, string(requiredScope))
		}
	}

	if len(missingRequired) > 0 {
		return fmt.Errorf("missing required scopes: %s", strings.Join(missingRequired, ", "))
	}

	return nil
}
