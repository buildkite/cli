package github

import (
	"strings"

	"github.com/buildkite/cli/git"
)

func ParseGithubRemote(gitRemote string) (string, string, error) {
	u, err := git.ParseGittableURL(gitRemote)
	if err != nil {
		return "", "", err
	}

	pathParts := strings.SplitN(strings.TrimLeft(strings.TrimSuffix(u.Path, ".git"), "/"), "/", 2)
	org := pathParts[0]
	repo := pathParts[1]

	return org, repo, nil
}
