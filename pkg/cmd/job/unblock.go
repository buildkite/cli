package job

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/graphql"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/charmbracelet/huh/spinner"
	"github.com/spf13/cobra"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

const jobCommandPrefix = "JobTypeBlock---"

func NewCmdJobUnblock(f *factory.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "unblock <job id>",
		Short: "Unblock a job",
		Long: heredoc.Doc(`
			Use this command to unblock build jobs.
			Currently, this does not support submitting fields to the step.
		`),
		Args:    cobra.ExactArgs(1),
		Example: "$ bk job unblock 0190046e-e199-453b-a302-a21a4d649d31",
		RunE: func(cmd *cobra.Command, args []string) error {
			// given a job UUID argument, we need to generate the GraphQL ID matching
			uuid := args[0]
			graphqlID := generateGraphQLID(uuid)

			var err error
			spinErr := spinner.New().
				Title("Unblocking job").
				Action(func() {
					_, err = graphql.UnblockJob(cmd.Context(), f.GraphQLClient, graphqlID)
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
}

func generateGraphQLID(uuid string) string {
	var graphqlID strings.Builder
	wr := base64.NewEncoder(base64.StdEncoding, &graphqlID)
	fmt.Fprintf(wr, "%s%s", jobCommandPrefix, uuid)
	wr.Close()

	return graphqlID.String()
}
