package job

import (
	"context"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/graphql"
	bk_io "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/internal/util"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/spf13/cobra"
)

func NewCmdJobCancel(f *factory.Factory) *cobra.Command {
	var web bool

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "cancel <job id> [flags]",
		Args:                  cobra.ExactArgs(1),
		Short:                 "Cancel a job.",
		Long: heredoc.Doc(`
			Cancel the given job.
		`),
		Example: heredoc.Doc(`
			# Cancel a job (with confirmation prompt)
			$ bk job cancel 0190046e-e199-453b-a302-a21a4d649d31

			# Cancel a job without confirmation (useful for automation)
			$ bk job --yes cancel 0190046e-e199-453b-a302-a21a4d649d31

			# Cancel a job and open it in browser
			$ bk job --yes cancel 0190046e-e199-453b-a302-a21a4d649d31 --web
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			jobID := args[0]
			graphqlID := util.GenerateGraphQLID("JobTypeCommand---", jobID)

			confirmed, err := bk_io.Confirm(f, fmt.Sprintf("Cancel job %s", jobID))
			if err != nil {
				return err
			}

			if confirmed {
				return cancelJob(cmd.Context(), jobID, graphqlID, web, f)
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&web, "web", "w", false, "Open the job in a web browser after it has been cancelled.")
	cmd.Flags().SortFlags = false

	return &cmd
}

func cancelJob(ctx context.Context, displayID, apiID string, web bool, f *factory.Factory) error {
	var err error
	var result *graphql.CancelJobResponse
	spinErr := bk_io.SpinWhile(f, fmt.Sprintf("Cancelling job %s", displayID), func() {
		result, err = graphql.CancelJob(ctx, f.GraphQLClient, apiID)
	})
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return err
	}

	job := result.JobTypeCommandCancel.JobTypeCommand
	fmt.Printf("Job canceled: %s\n", job.Url)

	return util.OpenInWebBrowser(web, job.Url)
}
