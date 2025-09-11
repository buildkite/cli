package job

import (
	"context"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/graphql"
	bk_io "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/internal/util"
	"github.com/buildkite/cli/v3/internal/validation/scopes"
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
		Example: "$ bk job cancel 0190046e-e199-453b-a302-a21a4d649d31",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			cmdScopes := scopes.GetCommandScopes(cmd)
			tokenScopes := f.Config.GetTokenScopes()
			if len(tokenScopes) == 0 {
				return fmt.Errorf("no scopes found in token. Please ensure you're using a token with appropriate scopes")
			}

			if err := scopes.ValidateScopes(cmdScopes, tokenScopes); err != nil {
				return err
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			jobID := args[0]
			graphqlID := util.GenerateGraphQLID("JobTypeCommand---", jobID)

			return cancelJob(cmd.Context(), graphqlID, web, f)
		},
	}

	cmd.Annotations = map[string]string{
		"requiredScopes": string(scopes.WriteBuilds),
	}

	cmd.Flags().BoolVarP(&web, "web", "w", false, "Open the job in a web browser after it has been cancelled.")
	cmd.Flags().SortFlags = false

	return &cmd
}

func cancelJob(ctx context.Context, jobID string, web bool, f *factory.Factory) error {
	var err error
	var result *graphql.CancelJobResponse
	spinErr := bk_io.SpinWhile(fmt.Sprintf("Cancelling job %s", jobID), func() {
		result, err = graphql.CancelJob(ctx, f.GraphQLClient, jobID)
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