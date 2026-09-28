package secret

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
)

type MigrateGitHubActionsCmd struct {
	Prepare MigrateGitHubActionsPrepareCmd `cmd:"" help:"Prepare a reviewed GitHub Actions migration workflow."`
	Run     MigrateGitHubActionsRunCmd     `cmd:"" help:"Verify and dispatch a prepared migration workflow."`
}

type MigrateGitHubActionsPrepareCmd struct {
	Organization string   `help:"Destination Buildkite organization slug."`
	Cluster      string   `help:"Assert the destination cluster UUID."`
	Pipeline     string   `help:"Destination Buildkite pipeline slug."`
	PolicyFile   string   `help:"Buildkite secret access policy YAML file." type:"path"`
	SecretNames  []string `help:"Exact GitHub Actions secret name to migrate (repeatable)." name:"secret"`
	Matches      []string `help:"Glob matching GitHub Actions secret names (repeatable)." name:"match"`
	Output       string   `help:"Write the workflow without replacing an existing file." type:"path"`
}

type MigrateGitHubActionsRunCmd struct {
	Workflow string `help:"Prepared workflow committed to the repository default branch." required:"" type:"path"`
}

func (c *MigrateGitHubActionsPrepareCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	if err := c.validate(); err != nil {
		return err
	}
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()), factory.WithOrgOverride(c.Organization))
	if err != nil {
		return err
	}
	if err := validation.ValidateConfigurationForOrg(f.Config, kongCtx.Command(), c.Organization); err != nil {
		return err
	}
	organization := c.Organization
	if organization == "" {
		organization = f.Config.OrganizationSlug()
	}
	migration := newGitHubActionsMigration(f)
	return migration.prepare(context.Background(), *c, organization, globals.DisableInput())
}

func (c *MigrateGitHubActionsRunCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	if err := validateMigrationWorkflowPath(c.Workflow); err != nil {
		return fmt.Errorf("--workflow: %w", err)
	}
	workflow, err := readMigrationWorkflow(c.Workflow)
	if err != nil {
		return err
	}
	manifest, err := decodeMigrationManifest(workflow)
	if err != nil {
		return err
	}
	// The reviewed manifest owns the destination. Resolve that organization's
	// credential rather than silently using whichever organization is selected.
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()), factory.WithOrgOverride(manifest.Organization))
	if err != nil {
		return err
	}
	if err := validation.ValidateConfigurationForOrg(f.Config, kongCtx.Command(), manifest.Organization); err != nil {
		return err
	}
	migration := newGitHubActionsMigration(f)
	return migration.run(context.Background(), c.Workflow)
}

func newGitHubActionsMigration(f *factory.Factory) *githubActionsMigration {
	return &githubActionsMigration{
		bk:     migrationBuildkiteAPI{client: f.RestAPIClient},
		gh:     execGitHubRunner{stderr: os.Stderr},
		input:  bufio.NewReader(os.Stdin),
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
}

func (c *MigrateGitHubActionsPrepareCmd) validate() error {
	if c.Organization != "" && !organizationSlugPattern.MatchString(c.Organization) {
		return errors.New("--organization must be a lowercase Buildkite organization slug")
	}
	if c.Cluster != "" && !uuidPattern.MatchString(c.Cluster) {
		return errors.New("--cluster must be a Buildkite cluster UUID")
	}
	if c.Pipeline != "" && !organizationSlugPattern.MatchString(c.Pipeline) {
		return errors.New("--pipeline must be a lowercase Buildkite pipeline slug")
	}
	if c.PolicyFile == "-" {
		return errors.New("--policy-file must be a file path, not stdin")
	}
	for _, pattern := range c.Matches {
		if _, err := path.Match(pattern, "NAME"); err != nil {
			return fmt.Errorf("invalid --match glob %q: %w", pattern, err)
		}
	}
	if c.Output != "" {
		if err := validateMigrationWorkflowPath(c.Output); err != nil {
			return fmt.Errorf("--output: %w", err)
		}
	}
	return nil
}
