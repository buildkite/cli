package job

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/graphql"
	bk_io "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/internal/util"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/spf13/cobra"
)

const jobCommandPrefix = "JobTypeCommand---"

func NewCmdJobRetry(f *factory.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "retry <job id>",
		DisableFlagsInUseLine: true,
		Short:                 "Retry a job",
		Long: heredoc.Doc(`
			Use this command to retry build jobs.
		`),
		Args:    cobra.ExactArgs(1),
		Example: "$ bk job retry 0190046e-e199-453b-a302-a21a4d649d31",
		RunE: func(cmd *cobra.Command, args []string) error {
			// given a job UUID argument, we need to generate the GraphQL ID matching
			uuid := args[0]
			graphqlID := util.GenerateGraphQLID(jobCommandPrefix, uuid)

			var err error
			var j *graphql.RetryJobResponse
			spinErr := bk_io.SpinWhile("Retrying job", func() {
				j, err = graphql.RetryJob(cmd.Context(), f.GraphQLClient, graphqlID)
			})
			if spinErr != nil {
				return spinErr
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), "Successfully retried job: "+j.JobTypeCommandRetry.JobTypeCommand.Url)

			return err
		},
	}

	return cmd
}
