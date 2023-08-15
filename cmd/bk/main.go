package main

import (
	"context"
	"fmt"
	"os"

	"github.com/buildkite/cli/v3/pkg/cmd/root"
)

func main() {
	code := mainRun()
	os.Exit(code)
}

func mainRun() int {
	ctx := context.Background()

	rootCmd, err := root.NewCmdRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create root command: %s\n", err)
		return 1
	}

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "failed to execute command: %s\n", err)
		return 1
	}

	return 0
}
