package pkg

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

func GitRemote(dir string) (string, error) {
	gitDir := filepath.Join(dir, ".git")

	// get the remote url, e.g git@github.com:buildkite/cli.git
	cmd := exec.Command("git", "--git-dir", gitDir, "remote", "get-url", "origin")
	output, err := cmd.CombinedOutput()

	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

func ParseGithubRemote(gitRemote string) (string, string, error) {
	u, err := parseGittableURL(gitRemote)
	if err != nil {
		return "", "", err
	}

	pathParts := strings.SplitN(strings.TrimLeft(strings.TrimSuffix(u.Path, ".git"), "/"), "/", 2)
	org := pathParts[0]
	repo := pathParts[1]

	return org, repo, nil
}

// Liberally borrowed from https://github.com/buildkite/agent/blob/6553217b9c5f7a1b67d4da6bd9d9f4de83904aaf/bootstrap/git.go

var (
	hasSchemePattern  = regexp.MustCompile("^[^:]+://")
	scpLikeURLPattern = regexp.MustCompile("^([^@]+@)?([^:]+):/?(.+)$")
)

// parseGittableURL parses and converts a git repository url into a url.URL
func parseGittableURL(ref string) (*url.URL, error) {
	if _, err := os.Stat(ref); os.IsExist(err) {
		return url.Parse(fmt.Sprintf("file://%s", ref))
	}

	if !hasSchemePattern.MatchString(ref) && scpLikeURLPattern.MatchString(ref) {
		matched := scpLikeURLPattern.FindStringSubmatch(ref)
		user := matched[1]
		host := matched[2]
		path := matched[3]

		ref = fmt.Sprintf("ssh://%s%s/%s", user, host, path)
	}

	return url.Parse(ref)
}

// Clean up the SSH host and remove any key identifiers. See:
// git@github.com-custom-identifier:foo/bar.git
// https://buildkite.com/docs/agent/ssh-keys#creating-multiple-ssh-keys
var gitHostAliasRegexp = regexp.MustCompile(`-[a-z0-9\-]+$`)

func stripAliasesFromGitHost(host string) string {
	return gitHostAliasRegexp.ReplaceAllString(host, "")
}
