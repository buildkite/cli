package job

import (
	"testing"

	"github.com/buildkite/cli/v3/internal/util"
)

func TestNewCmdJobCancel(t *testing.T) {
	cmd := NewCmdJobCancel(nil)

	if cmd.Use != "cancel <job id> [flags]" {
		t.Errorf("got %s, want cancel <job id> [flags]", cmd.Use)
	}

	if cmd.Short != "Cancel a job." {
		t.Errorf("got %s, want Cancel a job.", cmd.Short)
	}

	yesFlag := cmd.Flags().Lookup("yes")
	if yesFlag == nil {
		t.Error("--yes flag should exist")
	}
	if yesFlag.Shorthand != "y" {
		t.Errorf("--yes flag shorthand should be 'y', got '%s'", yesFlag.Shorthand)
	}

	webFlag := cmd.Flags().Lookup("web")
	if webFlag == nil {
		t.Error("--web flag should exist")
	}
	if webFlag.Shorthand != "w" {
		t.Errorf("--web flag shorthand should be 'w', got '%s'", webFlag.Shorthand)
	}
}

func TestGraphQLIDGeneration(t *testing.T) {
	jobUUID := "01993829-d2e7-4834-9611-bbeb8c1290db"
	graphqlID := util.GenerateGraphQLID("JobTypeCommand---", jobUUID)

	if graphqlID == "" {
		t.Error("GraphQL ID should not be empty")
	}
}
