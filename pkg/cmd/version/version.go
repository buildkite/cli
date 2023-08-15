package version

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func NewCmdVersion() *cobra.Command {
	return &cobra.Command{
		Use: "version",
		Hidden: true,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(os.Stdout, "3.0\n")
		},
	}
}
