package init

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"
)

const (
	defaultPipelineYAML = `steps:
  - label: "Hello, world! 👋"
    command: echo "Hello, world!"`
)

type InitCmd struct{}

func (c *InitCmd) Run(kongCtx *kong.Context) error {
	if found, path := findExistingPipelineFile(""); found {
		fmt.Printf("✨ File found at %s. You're good to go!\n", path)
		return nil
	}

	pipelineFile := filepath.Join(".buildkite", "pipeline.yaml")
	err := os.MkdirAll(filepath.Dir(pipelineFile), 0o755)
	if err != nil {
		return err
	}

	err = os.WriteFile(pipelineFile, []byte(defaultPipelineYAML), 0o660)
	if err != nil {
		return err
	}

	fmt.Printf("✨ File created at %s. You're good to go!\n", pipelineFile)

	return nil
}

func findExistingPipelineFile(base string) (bool, string) {
	// the order in which buildkite-agent checks for files
	paths := []string{
		"buildkite.yml",
		"buildkite.yaml",
		"buildkite.json",
		filepath.FromSlash(".buildkite/pipeline.yml"),
		filepath.FromSlash(".buildkite/pipeline.yaml"),
		filepath.FromSlash(".buildkite/pipeline.json"),
		filepath.FromSlash("buildkite/pipeline.yml"),
		filepath.FromSlash("buildkite/pipeline.yaml"),
		filepath.FromSlash("buildkite/pipeline.json"),
	}

	for _, path := range paths {
		path = filepath.Join(base, path)
		if _, err := os.Stat(path); err == nil {
			return true, path
		}
	}

	return false, ""
}
