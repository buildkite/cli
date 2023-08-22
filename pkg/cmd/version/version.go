package version

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func NewCmdVersion() *cobra.Command {
	return &cobra.Command{
		Use:    "version",
		Hidden: true,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(os.Stdout, cmd.Root().Annotations["versionInfo"])
		},
	}
}

func Format(version string) string {
	version = strings.TrimPrefix(version, "v")
	return fmt.Sprintf("bk version %s\n", version)
}
