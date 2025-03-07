// Package testutil provides reusable test helpers for the CLI
package testutil

import (
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v4"
	"github.com/go-git/go-git/v5"
	"github.com/spf13/afero"
)

// CreateFactory creates a Factory with a mock configuration and API client
func CreateFactory(t *testing.T, serverURL, org string, repo *git.Repository) *factory.Factory {
	t.Helper()

	bkClient, err := buildkite.NewOpts(buildkite.WithBaseURL(serverURL))
	if err != nil {
		t.Errorf("Error creating buildkite client: %s", err)
	}

	conf := config.New(afero.NewMemMapFs(), nil)
	err = conf.SelectOrganization(org)
	if err != nil {
		t.Errorf("Error selecting organization: %s", err)
	}
	return &factory.Factory{
		Config:        conf,
		RestAPIClient: bkClient,
		GitRepository: repo,
	}
}

// GitRepository creates a test git repository
func GitRepository() *git.Repository {
	repo, _ := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true, EnableDotGitCommonDir: true})
	return repo
}
