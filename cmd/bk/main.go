package main

import (
	"context"
	"fmt"
	"os"

	"github.com/buildkite/cli/v3/internal/build"
	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/pkg/cmd/root"
	"github.com/spf13/viper"
)

func main() {
	code := mainRun()
	os.Exit(code)
}

func mainRun() int {
	ctx := context.Background()
	v := viper.New()
	v.SetConfigFile(config.ConfigFile())
	v.AutomaticEnv()
	// attempt to read in config file but it might not exist
	_ = v.ReadInConfig()

	rootCmd, err := root.NewCmdRoot(v, build.Version)
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
