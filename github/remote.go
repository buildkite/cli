package github

import (
	"fmt"
	"strings"

	"github.com/buildkite/cli/v2/git"
)

func ParseRemote(gitRemote string) (string, string, error) {
	u, err := git.ParseGittableURL(gitRemote)
	if err != nil {
		return "", "", err
	}

	pathParts := strings.SplitN(strings.TrimLeft(strings.TrimSuffix(u.Path, ".git"), "/"), "/", 2)

	if len(pathParts) < 2 {
		return "", "", fmt.Errorf("Failed to parse remote %q", gitRemote)
	}

	org := pathParts[0]
	repo := pathParts[1]

	return org, repo, nil
}
