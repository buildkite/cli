package main

import (
	"context"
	"fmt"
	"os"

	"github.com/buildkite/cli/v3/internal/version"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/root"
)

func main() {
	code := mainRun()
	os.Exit(code)
}

func mainRun() int {
	ctx := context.Background()
	f, err := factory.New(version.Version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create factory: %s\n", err)
		return 1
	}

	rootCmd, err := root.NewCmdRoot(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create root command: %s\n", err)
		return 1
	}

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		// Errors already get printed by the cobra command runner
		return 1
	}

	return 0
}
