package init

import (
	"fmt"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"log"
	"os"
)

var (
	yamlContent = []byte(
		`steps:
  - label: "Hello, world! 👋"
    command: echo "Hello, world!"`,
	)

	pipelineFile = ".buildkite/pipeline.yaml"
)

func NewCmdInit(v *viper.Viper) *cobra.Command {
	if _, err := os.Stat(pipelineFile); err == nil {
		fmt.Println("🕵️  pipeline.yaml found at .buildkite/pipeline.yaml. You're already good to go!")
	} else {
		file, err := os.Create(pipelineFile)

		if err != nil {
			log.Fatalf("failed creating file: %s", err)
		}

		defer file.Close()
		_, err = file.Write(yamlContent)

		if err != nil {
			log.Fatalf("failed to write to file: %f", err)
		}

		fmt.Println("✨ Buildkite pipeline successfully initialized 🎉")
	}

	cmd := &cobra.Command{
		Use:   "init",
		Args:  cobra.ExactArgs(0),
		Short: "Initialize Buildkite pipeline directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			err := os.MkdirAll(".buildkite", 0755)
			if err != nil {
				return err
			}
			return nil
		},
	}

	return cmd
}
