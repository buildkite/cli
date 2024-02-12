package build

import (
	"net/url"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/spf13/cobra"
)

func NewCmdBuild(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		Use:   "build <command>",
		Short: "Manage pipeline builds",
		Long:  "Work with Buildkite pipeline builds.",
		Example: heredoc.Doc(`
			# To create a new build
			$ bk build new -m "Build from cli" -c "HEAD" -b "main" 
		`),
		PersistentPreRunE: validation.CheckValidConfiguration(f.Config),
		Annotations: map[string]string{
			"help:arguments": heredoc.Doc(`
				A pipeline is passed as an argument. It can be supplied in any of the following formats:
				- "PIPELINE_SLUG"
				- "ORGANIZATION_SLUG/PIPELINE_SLUG" 
				- by URL, e.g. "https://buildkite.com/buildkite/buildkite-cli"
			`),
		},
	}

	cmd.AddCommand(NewCmdBuildNew(f))
	cmd.AddCommand(NewCmdBuildView(f))
	cmd.AddCommand(NewCmdBuildRebuild(f))

	return &cmd
}

func parsePipelineArg(arg string, conf *config.Config) (string, string) {
	var org, pipeline string
	pipelineIsURL := strings.Contains(arg, ":")
	pipelineIsSlug := !pipelineIsURL && strings.Contains(arg, "/")

	if pipelineIsURL {
		url, err := url.Parse(arg)
		if err != nil {
			return "", ""
		}
		// eg: url.Path = /buildkite/buildkite-cli
		part := strings.Split(url.Path, "/")
		org, pipeline = part[1], part[2]
	} else if pipelineIsSlug {
		part := strings.Split(arg, "/")
		org, pipeline = part[0], part[1]
	} else {
		org = conf.Organization
		pipeline = arg
	}
	return org, pipeline
}
