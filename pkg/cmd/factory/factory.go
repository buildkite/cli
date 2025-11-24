package factory

import (
	"fmt"
	"net/http"

	"github.com/Khan/genqlient/graphql"
	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/internal/version"
	buildkite "github.com/buildkite/go-buildkite/v4"
	git "github.com/go-git/go-git/v5"
	"github.com/spf13/cobra"
)

var userAgent string

type Factory struct {
	Config        *config.Config
	GitRepository *git.Repository
	GraphQLClient graphql.Client
	RestAPIClient *buildkite.Client
	Version       string
	SkipConfirm   bool
	NoInput       bool
	Quiet         bool
}

// SetGlobalFlags reads the global persistent flags and sets them on the factory.
// This should be called in PreRunE of commands that need to use global flags.
// It's safe to call multiple times and will only set flags if they're present.
//
// NOTE: Due to Cobra limitations, global flags must be positioned AFTER at least one subcommand.
// Valid:   bk job --yes cancel <id>  or  bk job cancel --yes <id>
// Invalid: bk --yes job cancel <id>
//
// Once the CLI is fully migrated to Kong (currently in progress), the limitation will be removed
// and global flags will work in any position, including: bk --yes job cancel <id>
func (f *Factory) SetGlobalFlags(cmd *cobra.Command) {
	if yes, err := cmd.Flags().GetBool("yes"); err == nil && yes {
		f.SkipConfirm = yes
	}
	if noInput, err := cmd.Flags().GetBool("no-input"); err == nil && noInput {
		f.NoInput = noInput
	}
	if quiet, err := cmd.Flags().GetBool("quiet"); err == nil && quiet {
		f.Quiet = quiet
	}
}

type gqlHTTPClient struct {
	client *http.Client
	token  string
}

func init() {
	userAgent = fmt.Sprintf("%s buildkite-cli/%s", buildkite.DefaultUserAgent, version.Version)
}

func (a *gqlHTTPClient) Do(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.token))
	req.Header.Set("User-Agent", userAgent)
	return a.client.Do(req)
}

func New(version string) (*Factory, error) {
	repo, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true, EnableDotGitCommonDir: true})
	if err != nil {
		if err == git.ErrRepositoryNotExists {
			repo = nil
		}
	}

	conf := config.New(nil, repo)
	buildkiteClient, err := buildkite.NewOpts(
		buildkite.WithBaseURL(conf.RESTAPIEndpoint()),
		buildkite.WithTokenAuth(conf.APIToken()),
		buildkite.WithUserAgent(userAgent),
	)
	if err != nil {
		return nil, fmt.Errorf("creating buildkite client: %w", err)
	}

	graphqlHTTPClient := &gqlHTTPClient{client: http.DefaultClient, token: conf.APIToken()}

	return &Factory{
		Config:        conf,
		GitRepository: repo,
		GraphQLClient: graphql.NewClient(conf.GetGraphQLEndpoint(), graphqlHTTPClient),
		RestAPIClient: buildkiteClient,
		Version:       version,
	}, nil
}
