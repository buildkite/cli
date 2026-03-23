package preflight

import (
	"errors"
	"testing"

	bkErrors "github.com/buildkite/cli/v3/internal/errors"

	"github.com/buildkite/cli/v3/internal/cli"
)

type stubGlobals struct{}

func (s stubGlobals) SkipConfirmation() bool { return false }
func (s stubGlobals) DisableInput() bool     { return false }
func (s stubGlobals) IsQuiet() bool          { return false }
func (s stubGlobals) DisablePager() bool     { return false }
func (s stubGlobals) EnableDebug() bool      { return false }

var _ cli.GlobalFlags = stubGlobals{}

func TestPreflightCmd_Run(t *testing.T) {
	t.Run("returns validation error when experiment disabled", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "")

		cmd := &PreflightCmd{}
		err := cmd.Run(nil, stubGlobals{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		var bkErr *bkErrors.Error
		if !errors.As(err, &bkErr) {
			t.Fatalf("expected bkErrors.Error, got %T: %v", err, err)
		}
		if !errors.Is(bkErr, bkErrors.ErrValidation) {
			t.Errorf("expected ErrValidation, got category: %v", bkErr.Category)
		}
	})

	t.Run("returns validation error when not in git repo", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		// Run from a temp dir that is not a git repo.
		t.Chdir(t.TempDir())

		cmd := &PreflightCmd{}
		err := cmd.Run(nil, stubGlobals{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		var bkErr *bkErrors.Error
		if !errors.As(err, &bkErr) {
			t.Fatalf("expected bkErrors.Error, got %T: %v", err, err)
		}
		if !errors.Is(bkErr, bkErrors.ErrValidation) {
			t.Errorf("expected ErrValidation, got category: %v", bkErr.Category)
		}
	})
}
