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
		Args:  cobra.ArbitraryArgs,
		Long:  "Manage organization clusters",
		Short: "Manage organization clusters",
		Example: heredoc.Doc(`
			# To view cluster details
			$ bk cluster view "cluster_id"
		`),
		PersistentPreRunE: validation.CheckValidConfiguration(f.Config),
	}
	cmd.AddCommand(NewCmdClusterView(f))
	cmd.AddCommand(NewCmdClusterList(f))

	return &cmd
}
