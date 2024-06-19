package cluster

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/spf13/cobra"
)

func NewCmdCluster(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		Use:   "cluster <command>",
		Short: "View cluster information",
		Args:  cobra.ArbitraryArgs,
		Long:  "Work with Buildkite cluster.",
		Example: heredoc.Doc(`
			# To view cluster details
			$ bk cluster view -c "cluster_id"
		`),
		PersistentPreRunE: validation.CheckValidConfiguration(f.Config),
		Annotations: map[string]string{
			"help:arguments": heredoc.Doc(`
				Organization slug and cluster id is passed as an argument. It can be supplied in any of the following formats:
				- "ORG_SLUG"
			`),
		},
	}
	cmd.AddCommand(NewCmdClusterView(f))

	return &cmd
}
