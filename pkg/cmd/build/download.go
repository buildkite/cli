package build

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/buildkite/cli/v3/internal/build"
	buildResolver "github.com/buildkite/cli/v3/internal/build/resolver"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/charmbracelet/huh/spinner"
	"github.com/spf13/cobra"
)

func NewCmdBuildDownload(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "download [number [pipeline]] [flags]",
		Short:                 "Download job logs for a build",
		Long:                  "Download allows you to download resources for a build.",
		RunE: func(cmd *cobra.Command, args []string) error {
			pipelineRes := pipelineResolver.NewAggregateResolver(
				pipelineResolver.ResolveFromPositionalArgument(args, 1, f.Config),
				pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOne),
				pipelineResolver.ResolveFromRepository(f, pipelineResolver.CachedPicker(f.Config, pipelineResolver.PickOne)),
			)

			buildRes := buildResolver.NewAggregateResolver(
				buildResolver.ResolveFromPositionalArgument(args, 0, pipelineRes.Resolve, f.Config),
				buildResolver.ResolveBuildFromCurrentBranch(f.GitRepository, pipelineRes.Resolve, f),
			)

			bld, err := buildRes.Resolve(cmd.Context())
			if err != nil {
				return err
			}
			if bld == nil {
				return fmt.Errorf("could not resolve a build")
			}

			var dir string
			spinErr := spinner.New().
				Title("Downloading logs").
				Action(func() {
					dir, err = download(*bld, f)
				}).
				Run()
			if spinErr != nil {
				return spinErr
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Downloaded logs to: %s\n", dir)

			return err
		},
	}

	return &cmd
}

func download(build build.Build, f *factory.Factory) (string, error) {
	b, _, err := f.RestAPIClient.Builds.Get(build.Organization, build.Pipeline, fmt.Sprint(build.BuildNumber), nil)
	if err != nil {
		return "", err
	}
	if b == nil {
		return "", fmt.Errorf("could not find build for %s #%d", build.Pipeline, build.BuildNumber)
	}

	directory := fmt.Sprintf("build-logs-%s", *b.ID)
	err = os.MkdirAll(directory, os.ModePerm)
	if err != nil {
		return "", err
	}

	for _, job := range b.Jobs {
		// only script (command) jobs will have logs
		if job == nil || *job.Type != "script" {
			continue
		}

		log, _, err := f.RestAPIClient.Jobs.GetJobLog(build.Organization, build.Pipeline, *b.ID, *job.ID)
		if err != nil {
			return "", err
		}
		if log == nil {
			return "", fmt.Errorf("could not get logs for job %s", *job.ID)
		}

		err = os.WriteFile(filepath.Join(directory, *job.ID), []byte(*log.Content), 0o644)
		if err != nil {
			return "", err
		}
	}

	return directory, nil
}
