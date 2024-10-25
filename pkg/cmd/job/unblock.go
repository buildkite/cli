package job

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/graphql"
	bk_io "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/internal/util"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/charmbracelet/huh/spinner"
	"github.com/spf13/cobra"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

const jobBlockPrefix = "JobTypeBlock---"

func NewCmdJobUnblock(f *factory.Factory) *cobra.Command {
	var data string

	cmd := &cobra.Command{
		Use:                   "unblock <job id>",
		DisableFlagsInUseLine: true,
		Short:                 "Unblock a job",
		Long: heredoc.Doc(`
			Use this command to unblock build jobs.
			Currently, this does not support submitting fields to the step.
		`),
		Args:    cobra.ExactArgs(1),
		Example: "$ bk job unblock 0190046e-e199-453b-a302-a21a4d649d31",
		RunE: func(cmd *cobra.Command, args []string) error {
			// given a job UUID argument, we need to generate the GraphQL ID matching
			uuid := args[0]
			graphqlID := util.GenerateGraphQLID(jobBlockPrefix, uuid)

			// get unblock step fields if available
			var fields *string
			if bk_io.HasDataAvailable(cmd.InOrStdin()) {
				stdin := new(strings.Builder)
				_, err := io.Copy(stdin, cmd.InOrStdin())
				if err != nil {
					return err
				}
				input := stdin.String()
				fields = &input
			} else if data != "" {
				fields = &data
			} else {
				// the graphql API errors if providing a null fields value so we need to provide and empty json object
				input := "{}"
				fields = &input
			}

			var err error
			spinErr := spinner.New().
				Title("Unblocking job").
				Action(func() {
					_, err = graphql.UnblockJob(cmd.Context(), f.GraphQLClient, graphqlID, fields)
				}).
				Run()
			if spinErr != nil {
				return spinErr
			}

			// handle a "graphql error" if the job is already unblocked
			if err != nil {
				var errList gqlerror.List
				if errors.As(err, &errList) {
					for _, err := range errList {
						if err.Message == "The job's state must be blocked" {
							fmt.Fprintln(cmd.OutOrStdout(), "This job is already unblocked")
							return nil
						}
					}
				}
				return err
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), "Successfully unblocked job")

			return err
		},
	}

	cmd.Flags().StringVar(&data, "data", "", "JSON formatted data to unblock the job.")

	return cmd
}
