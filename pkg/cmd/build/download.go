package build

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/briandowns/spinner"
	"github.com/buildkite/cli/v3/internal/build"
	buildResolver "github.com/buildkite/cli/v3/internal/build/resolver"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/spf13/cobra"
)

func NewCmdBuildDownload(f *factory.Factory) *cobra.Command {
	var logs bool

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "download [number [pipeline]] [flags]",
		Short:                 "Download resources of a build",
		Long:                  "Download allows you to download resources from a build.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !logs {
				fmt.Println("Nothing to download")
				return nil
			}
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

			s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
			s.Suffix = " Downloading logs"
			s.Start()
			defer s.Stop()
			err = download(*bld, f)

			return err
		},
	}

	cmd.Flags().BoolVar(&logs, "logs", false, "Download all job logs for the build")

	return &cmd
}

func download(build build.Build, f *factory.Factory) error {
	b, _, err := f.RestAPIClient.Builds.Get(build.Organization, build.Pipeline, fmt.Sprint(build.BuildNumber), nil)
	if err != nil {
		return err
	}
	if b == nil {
		return fmt.Errorf("could not find build for %s #%d", build.Pipeline, build.BuildNumber)
	}

	directory := fmt.Sprintf("build-logs-%s", *b.ID)
	err = os.MkdirAll(directory, os.ModePerm)
	if err != nil {
		return err
	}

	for _, job := range b.Jobs {
		// only script (command) jobs will have logs
		if job == nil || *job.Type != "script" {
			continue
		}

		log, _, err := f.RestAPIClient.Jobs.GetJobLog(build.Organization, build.Pipeline, *b.ID, *job.ID)
		if err != nil {
			return err
		}
		if log == nil {
			return fmt.Errorf("could not get logs for job %s", *job.ID)
		}

		err = os.WriteFile(filepath.Join(directory, *job.ID), []byte(*log.Content), 0644)
		if err != nil {
			return err
		}
	}

	fmt.Printf("\nDownloaded logs to: %s\n", directory)

	return nil
}
