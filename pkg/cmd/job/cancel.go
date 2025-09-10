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

func NewCmdJobCancel(f *factory.Factory) *cobra.Command {
	var confirmed bool

	cmd := &cobra.Command{
		Use:                   "cancel <job id>",
		DisableFlagsInUseLine: true,
		Short:                 "Cancel a job",
		Long: heredoc.Doc(`
			Use this command to cancel build jobs.
			
			The job must be in a cancellable state (e.g., running, scheduled, assigned).
		`),
		Args:    cobra.ExactArgs(1),
		Example: "$ bk job cancel 0190046e-e199-453b-a302-a21a4d649d31",
		RunE: func(cmd *cobra.Command, args []string) error {
			// given a job UUID argument, we need to generate the GraphQL ID matching
			uuid := args[0]
			graphqlID := util.GenerateGraphQLID(jobCommandPrefix, uuid)

			// Confirm the cancellation unless --yes flag is used
			err := bk_io.Confirm(&confirmed, fmt.Sprintf("Cancel job %s", uuid))
			if err != nil {
				return err
			}

			if !confirmed {
				return nil
			}

			var j *graphql.CancelJobResponse
			spinErr := bk_io.SpinWhile("Cancelling job", func() {
				j, err = graphql.CancelJob(cmd.Context(), f.GraphQLClient, graphqlID)
			})
			if spinErr != nil {
				return spinErr
			}

			// Handle error from GraphQL mutation
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), "Successfully cancelled job: "+j.JobTypeCommandCancel.JobTypeCommand.Url)

			return err
		},
	}

	cmd.Flags().BoolVarP(&confirmed, "yes", "y", false, "Skip the confirmation prompt. Useful if being used in automation/CI.")
	cmd.Flags().SortFlags = false

	return cmd
}
