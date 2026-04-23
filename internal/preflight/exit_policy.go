package preflight

import (
	"fmt"
	"slices"
	"strings"

	bkErrors "github.com/buildkite/cli/v3/internal/errors"
)

type ExitPolicy int

const (
	ExitOnBuildFailing ExitPolicy = iota
	ExitOnBuildTerminal
)

func (p *ExitPolicy) UnmarshalText(text []byte) error {
	value := string(text)
	name, _, _ := strings.Cut(value, ":")
	switch name {
	case "build-failing":
		*p = ExitOnBuildFailing
	case "build-terminal":
		*p = ExitOnBuildTerminal
	default:
		return bkErrors.NewValidationError(fmt.Errorf("unsupported --exit-on value %q", value), "invalid exit condition")
	}
	return nil
}

func EffectiveExitPolicy(policies []ExitPolicy) ExitPolicy {
	if slices.Contains(policies, ExitOnBuildTerminal) {
		return ExitOnBuildTerminal
	}
	return ExitOnBuildFailing
}

func ValidateExitPolicies(policies []ExitPolicy, watch bool) error {
	if len(policies) > 0 && !watch {
		return bkErrors.NewValidationError(fmt.Errorf("--exit-on requires --watch"), "exit conditions require watch mode")
	}
	if slices.Contains(policies, ExitOnBuildFailing) && slices.Contains(policies, ExitOnBuildTerminal) {
		return bkErrors.NewValidationError(fmt.Errorf("build-failing and build-terminal cannot be used together"), "invalid exit conditions")
	}
	return nil
}
