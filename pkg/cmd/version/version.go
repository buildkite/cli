package version

import (
	"fmt"
	"os"
	"strings"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/spf13/cobra"
)

func NewCmdVersion(f *factory.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version of the CLI being used",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(os.Stdout, "%s\n", Format(f.Version))
		},
	}
}

func Format(version string) string {
	version = strings.TrimPrefix(version, "v")
	return fmt.Sprintf("bk version %s\n", version)
}
