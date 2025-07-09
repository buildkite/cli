package kong

import (
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	agentCmd "github.com/buildkite/cli/v3/pkg/cmd/agent"
	apiCmd "github.com/buildkite/cli/v3/pkg/cmd/api"
	artifactsCmd "github.com/buildkite/cli/v3/pkg/cmd/artifacts"
	buildCmd "github.com/buildkite/cli/v3/pkg/cmd/build"
	clusterCmd "github.com/buildkite/cli/v3/pkg/cmd/cluster"
	configureCmd "github.com/buildkite/cli/v3/pkg/cmd/configure"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	initCmd "github.com/buildkite/cli/v3/pkg/cmd/init"
	jobCmd "github.com/buildkite/cli/v3/pkg/cmd/job"
	pipelineCmd "github.com/buildkite/cli/v3/pkg/cmd/pipeline"
	packageCmd "github.com/buildkite/cli/v3/pkg/cmd/pkg"
	promptCmd "github.com/buildkite/cli/v3/pkg/cmd/prompt"
	useCmd "github.com/buildkite/cli/v3/pkg/cmd/use"
	"github.com/buildkite/cli/v3/pkg/cmd/user"
	versionCmd "github.com/buildkite/cli/v3/pkg/cmd/version"
)

// DocsCmd implementation for llms.txt generation
func (c *DocsCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	content, err := generateLLMSDoc(ctx)
	if err != nil {
		return err
	}

	if c.Output != "" {
		return os.WriteFile(c.Output, []byte(content), 0644)
	}

	fmt.Fprint(ctx.Stdout, content)
	return nil
}

func generateLLMSDoc(ctx *kong.Context) (string, error) {
	var sb strings.Builder

	// Header
	sb.WriteString("# Buildkite CLI (`bk`) Documentation\n\n")
	sb.WriteString("This is the command-line interface for Buildkite.\n\n")

	// Generate help text
	help := ctx.Model.Help
	sb.WriteString("## Usage\n\n")
	sb.WriteString("```\n")
	sb.WriteString(help)
	sb.WriteString("\n```\n\n")

	// Add command descriptions
	sb.WriteString("## Commands\n\n")

	// Walk through all commands and generate documentation
	for _, cmd := range ctx.Model.Children {
		if cmd.Type == kong.CommandNode {
			sb.WriteString(fmt.Sprintf("### %s\n\n", cmd.Name))
			sb.WriteString(fmt.Sprintf("%s\n\n", cmd.Help))

			// Add subcommands if any
			if len(cmd.Children) > 0 {
				sb.WriteString("#### Subcommands\n\n")
				for _, subcmd := range cmd.Children {
					if subcmd.Type == kong.CommandNode {
						sb.WriteString(fmt.Sprintf("- `%s %s`: %s\n", cmd.Name, subcmd.Name, subcmd.Help))
					}
				}
				sb.WriteString("\n")
			}

			// Add flags
			if len(cmd.Flags) > 0 {
				sb.WriteString("#### Flags\n\n")
				for _, flag := range cmd.Flags {
					short := ""
					if flag.Short != 0 {
						short = fmt.Sprintf("-%c, ", flag.Short)
					}
					sb.WriteString(fmt.Sprintf("- `%s--%s`: %s\n", short, flag.Name, flag.Help))
				}
				sb.WriteString("\n")
			}
		}
	}

	return sb.String(), nil
}

// Agent command implementations
func (c *AgentCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	fmt.Fprint(ctx.Stdout, ctx.Model.Help)
	return nil
}

func (c *AgentListCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	// Convert Kong flags to Cobra-style args
	args := []string{"list"}
	if c.Name != "" {
		args = append(args, "--name", c.Name)
	}
	if c.AgentVersion != "" {
		args = append(args, "--version", c.AgentVersion)
	}
	if c.Hostname != "" {
		args = append(args, "--hostname", c.Hostname)
	}
	if c.PerPage > 0 {
		args = append(args, "--per-page", fmt.Sprintf("%d", c.PerPage))
	}

	return runCobraCommand(agentCmd.NewCmdAgent(f), args)
}

func (c *AgentStopCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	args := []string{"stop"}
	if c.Force {
		args = append(args, "--force")
	}
	if c.Limit > 0 {
		args = append(args, "--limit", fmt.Sprintf("%d", c.Limit))
	}

	return runCobraCommand(agentCmd.NewCmdAgent(f), args)
}

func (c *AgentViewCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	args := []string{"view"}
	if c.Web {
		args = append(args, "--web")
	}

	return runCobraCommand(agentCmd.NewCmdAgent(f), args)
}

// API command implementation
func (c *APICmd) Run(ctx *kong.Context, f *factory.Factory) error {
	args := []string{}
	if c.Method != "" {
		args = append(args, "--method", c.Method)
	}
	for _, header := range c.Header {
		args = append(args, "--header", header)
	}
	if c.Data != "" {
		args = append(args, "--data", c.Data)
	}
	if c.Analytics {
		args = append(args, "--analytics")
	}
	if c.File != "" {
		args = append(args, "--file", c.File)
	}

	return runCobraCommand(apiCmd.NewCmdAPI(f), args)
}

// Artifacts command implementations
func (c *ArtifactsCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	fmt.Fprint(ctx.Stdout, ctx.Model.Help)
	return nil
}

func (c *ArtifactsListCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	args := []string{"list"}
	if c.Job != "" {
		args = append(args, "--job", c.Job)
	}
	if c.Pipeline != "" {
		args = append(args, "--pipeline", c.Pipeline)
	}

	return runCobraCommand(artifactsCmd.NewCmdArtifacts(f), args)
}

func (c *ArtifactsDownloadCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	args := []string{"download"}
	return runCobraCommand(artifactsCmd.NewCmdArtifacts(f), args)
}

// Build command implementations
func (c *BuildCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	fmt.Fprint(ctx.Stdout, ctx.Model.Help)
	return nil
}

func (c *BuildCancelCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	args := []string{"cancel"}
	if c.Web {
		args = append(args, "--web")
	}
	if c.Pipeline != "" {
		args = append(args, "--pipeline", c.Pipeline)
	}
	if c.Yes {
		args = append(args, "--yes")
	}

	return runCobraCommand(buildCmd.NewCmdBuild(f), args)
}

func (c *BuildDownloadCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	args := []string{"download"}
	if c.Mine {
		args = append(args, "--mine")
	}
	if c.Branch != "" {
		args = append(args, "--branch", c.Branch)
	}
	if c.User != "" {
		args = append(args, "--user", c.User)
	}
	if c.Pipeline != "" {
		args = append(args, "--pipeline", c.Pipeline)
	}

	return runCobraCommand(buildCmd.NewCmdBuild(f), args)
}

func (c *BuildNewCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	args := []string{"new"}
	if c.Message != "" {
		args = append(args, "--message", c.Message)
	}
	if c.Commit != "" {
		args = append(args, "--commit", c.Commit)
	}
	if c.Branch != "" {
		args = append(args, "--branch", c.Branch)
	}
	if c.Web {
		args = append(args, "--web")
	}
	if c.Pipeline != "" {
		args = append(args, "--pipeline", c.Pipeline)
	}
	for k, v := range c.Env {
		args = append(args, "--env", fmt.Sprintf("%s=%s", k, v))
	}
	for k, v := range c.Metadata {
		args = append(args, "--metadata", fmt.Sprintf("%s=%s", k, v))
	}
	if c.IgnoreBranchFilters {
		args = append(args, "--ignore-branch-filters")
	}
	if c.Yes {
		args = append(args, "--yes")
	}
	if c.EnvFile != "" {
		args = append(args, "--env-file", c.EnvFile)
	}

	return runCobraCommand(buildCmd.NewCmdBuild(f), args)
}

func (c *BuildRebuildCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	args := []string{"rebuild"}
	if c.Mine {
		args = append(args, "--mine")
	}
	if c.Web {
		args = append(args, "--web")
	}
	if c.Branch != "" {
		args = append(args, "--branch", c.Branch)
	}
	if c.User != "" {
		args = append(args, "--user", c.User)
	}
	if c.Pipeline != "" {
		args = append(args, "--pipeline", c.Pipeline)
	}

	return runCobraCommand(buildCmd.NewCmdBuild(f), args)
}

func (c *BuildViewCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	args := []string{"view"}
	if c.Mine {
		args = append(args, "--mine")
	}
	if c.Web {
		args = append(args, "--web")
	}
	if c.Branch != "" {
		args = append(args, "--branch", c.Branch)
	}
	if c.User != "" {
		args = append(args, "--user", c.User)
	}
	if c.Pipeline != "" {
		args = append(args, "--pipeline", c.Pipeline)
	}

	return runCobraCommand(buildCmd.NewCmdBuild(f), args)
}

func (c *BuildWatchCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	args := []string{"watch"}
	if c.Pipeline != "" {
		args = append(args, "--pipeline", c.Pipeline)
	}
	if c.Branch != "" {
		args = append(args, "--branch", c.Branch)
	}
	if c.Interval > 0 {
		args = append(args, "--interval", fmt.Sprintf("%d", c.Interval))
	}

	return runCobraCommand(buildCmd.NewCmdBuild(f), args)
}

// Helper function to run existing Cobra commands
func runCobraCommand(cmd interface{}, args []string) error {
	// This is a bridge function to run the existing Cobra commands
	// For now, we'll return an error indicating the command needs to be implemented
	return fmt.Errorf("command not yet implemented in Kong version")
}

// Additional command implementations would go here...
// For brevity, I'll implement a few more key ones:

// Version command
func (c *VersionCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	fmt.Println(versionCmd.Format(f.Version))
	return nil
}

// Init command
func (c *InitCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	return runCobraCommand(initCmd.NewCmdInit(f), []string{})
}

// Use command
func (c *UseCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	return runCobraCommand(useCmd.NewCmdUse(f), []string{})
}

// Configure command implementations
func (c *ConfigureCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	fmt.Fprint(ctx.Stdout, ctx.Model.Help)
	return nil
}

func (c *ConfigureAddCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	args := []string{"add"}
	if c.Force {
		args = append(args, "--force")
	}
	if c.Org != "" {
		args = append(args, "--org", c.Org)
	}
	if c.Token != "" {
		args = append(args, "--token", c.Token)
	}

	return runCobraCommand(configureCmd.NewCmdConfigure(f), args)
}

// Cluster command implementations
func (c *ClusterCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	fmt.Fprint(ctx.Stdout, ctx.Model.Help)
	return nil
}

func (c *ClusterListCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	args := []string{"list"}
	return runCobraCommand(clusterCmd.NewCmdCluster(f), args)
}

func (c *ClusterViewCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	args := []string{"view"}
	return runCobraCommand(clusterCmd.NewCmdCluster(f), args)
}

// Job command implementations
func (c *JobCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	fmt.Fprint(ctx.Stdout, ctx.Model.Help)
	return nil
}

func (c *JobRetryCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	args := []string{"retry"}
	return runCobraCommand(jobCmd.NewCmdJob(f), args)
}

func (c *JobUnblockCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	args := []string{"unblock"}
	if c.Data != "" {
		args = append(args, "--data", c.Data)
	}
	return runCobraCommand(jobCmd.NewCmdJob(f), args)
}

// Package command implementations
func (c *PackageCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	fmt.Fprint(ctx.Stdout, ctx.Model.Help)
	return nil
}

func (c *PackagePushCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	args := []string{"push"}
	if c.StdinFileName != "" {
		args = append(args, "--stdin-file-name", c.StdinFileName)
	}
	if c.Web {
		args = append(args, "--web")
	}
	return runCobraCommand(packageCmd.NewCmdPackage(f), args)
}

// Pipeline command implementations
func (c *PipelineCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	fmt.Fprint(ctx.Stdout, ctx.Model.Help)
	return nil
}

func (c *PipelineCreateCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	args := []string{"create"}
	return runCobraCommand(pipelineCmd.NewCmdPipeline(f), args)
}

func (c *PipelineViewCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	args := []string{"view"}
	if c.Web {
		args = append(args, "--web")
	}
	return runCobraCommand(pipelineCmd.NewCmdPipeline(f), args)
}

func (c *PipelineValidateCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	args := []string{"validate"}
	if c.File != "" {
		args = append(args, "--file", c.File)
	}
	return runCobraCommand(pipelineCmd.NewCmdPipeline(f), args)
}

// Prompt command implementation
func (c *PromptCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	args := []string{}
	if c.Format != "" {
		args = append(args, "--format", c.Format)
	}
	if c.Shell != "" {
		args = append(args, "--shell", c.Shell)
	}
	return runCobraCommand(promptCmd.NewCmdPrompt(f), args)
}

// User command implementations
func (c *UserCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	fmt.Fprint(ctx.Stdout, ctx.Model.Help)
	return nil
}

func (c *UserInviteCmd) Run(ctx *kong.Context, f *factory.Factory) error {
	args := []string{"invite"}
	return runCobraCommand(user.CommandUser(f), args)
}
