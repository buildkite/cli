package git

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Remote returns the git remote for a given directory
func Remote(dir string) (string, error) {
	return gitCommand(dir, `remote`, `get-url`, `origin`)
}

// Commit returns the HEAD commit hash for a directory
func Commit(dir string) (string, error) {
	return gitCommand(dir, `log`, `-1`, `--pretty=%H`)
}

// Message returns the HEAD commit message for a directory
func Message(dir string) (string, error) {
	return gitCommand(dir, `log`, `-1`, `--pretty=%B`)
}

// Branch returns the branch name for a directory
func Branch(dir string) (string, error) {
	return gitCommand(dir, `rev-parse`, `--abbrev-ref`, `HEAD`)
}

// Run a git command against a dir and capture combined output or error
func gitCommand(dir string, params ...string) (string, error) {
	gitDir := filepath.Join(dir, ".git")
	gitParams := append([]string{"--git-dir", gitDir}, params...)

	cmd := exec.Command("git", gitParams...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

// Liberally borrowed from https://github.com/buildkite/agent/blob/6553217b9c5f7a1b67d4da6bd9d9f4de83904aaf/bootstrap/git.go

var (
	hasSchemePattern  = regexp.MustCompile("^[^:]+://")
	scpLikeURLPattern = regexp.MustCompile("^([^@]+@)?([^:]+):/?(.+)$")
)

// ParseGittableURL parses and converts a git repository url into a url.URL
func ParseGittableURL(ref string) (*url.URL, error) {
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

// MatchRemotes compares two github remotes and returns true if the point
// to the same project. This allows for git and https remotes to be compared
// and host aliases to be normalized
func MatchRemotes(r1, r2 string) bool {
	if r1 == r2 {
		return true
	}

	u1, err := ParseGittableURL(strings.TrimSuffix(r1, ".git"))
	if err != nil {
		return false
	}

	u2, err := ParseGittableURL(strings.TrimSuffix(r2, ".git"))
	if err != nil {
		return false
	}

	return u1.Host == u2.Host && u1.Path == u2.Path
}
