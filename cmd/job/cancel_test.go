package job

import (
	"testing"

	"github.com/buildkite/cli/v3/internal/util"
)

func TestCancelCmdStructure(t *testing.T) {
	t.Parallel()

	cmd := &CancelCmd{
		JobID: "01993829-d2e7-4834-9611-bbeb8c1290db",
		Web:   true,
	}

	if cmd.JobID == "" {
		t.Error("JobID should be set")
	}

	if !cmd.Web {
		t.Error("Web flag should be true")
	}
}

func TestGraphQLIDGeneration(t *testing.T) {
	t.Parallel()

	jobUUID := "01993829-d2e7-4834-9611-bbeb8c1290db"
	graphqlID := util.GenerateGraphQLID("JobTypeCommand---", jobUUID)

	if graphqlID == "" {
		t.Error("GraphQL ID should not be empty")
	}
}
