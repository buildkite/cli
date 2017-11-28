package commands

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/99designs/keyring"
	"github.com/briandowns/spinner"

	githubclient "github.com/google/go-github/github"

	"github.com/buildkite/buildkite-cli/buildkite"
	"github.com/buildkite/buildkite-cli/integrations/github"
	"github.com/fatih/color"
	"golang.org/x/oauth2"
)

const defaultPipelineYAML = `# Default pipeline from buildkite-cli
steps:
- label: Example Test
  command: echo "Hello!"
`

type InitCommandInput struct {
	Keyring keyring.Keyring
	Debug   bool
	Dir     string
}

func InitCommand(input InitCommandInput) error {
	dir, err := filepath.Abs(input.Dir)
	if err != nil {
		return NewExitError(err, 1)
	}

	pipelineFile := filepath.Join(dir, ".buildkite", "pipeline.yml")
	pipelineFileAdded := false

	// make sure we've got the directory in place for .buildkite/
	_ = os.Mkdir(filepath.Dir(pipelineFile), 0770)

	// create a .buildkite/pipeline.yml if one doesn't exist
	if _, err := os.Stat(pipelineFile); err == nil {
		fmt.Printf(color.YellowString("There is already a .buildkite/pipeline.yml, skipping creating it ⚠️\n"))
	} else {
		if err = ioutil.WriteFile(pipelineFile, []byte(defaultPipelineYAML), 0660); err != nil {
			return NewExitError(err, 1)
		}
		pipelineFileAdded = true
		fmt.Println(color.GreenString("Created .buildkite/pipeline.yml ✅\n"))
	}

	gitDir := filepath.Join(dir, ".git")

	// check we have a git directory
	if _, err := os.Stat(gitDir); err != nil {
		return NewExitError(fmt.Errorf("%s isn't a git managed project! Try `git init`", dir), 1)
	}

	debugf("[init] Examining git dir %s", gitDir)

	// get the remote url, e.g git@github.com:buildkite/buildkite-cli.git
	cmd := exec.Command("git", "--git-dir", gitDir, "remote", "get-url", "origin")
	output, err := cmd.CombinedOutput()

	if err != nil {
		return NewExitError(fmt.Errorf("Error getting git remote information: %v", err), 1)
	}

	debugf("[init] Git remote: %#v", string(output))

	u, err := buildkite.ParseGittableURL(strings.TrimSpace(string(output)))
	if err != nil {
		return NewExitError(fmt.Errorf("Error parsing git remote: %v", err), 1)
	}

	debugf("[init] Parsed %q as %#v", output, u)

	pathParts := strings.SplitN(strings.TrimLeft(strings.TrimSuffix(u.Path, ".git"), "/"), "/", 2)
	org := pathParts[0]
	repo := pathParts[1]

	fmt.Printf(color.GreenString("Found github remote for %s/%s ✅\n"), org, repo)

	var token oauth2.Token

	err = buildkite.RetrieveCredential(input.Keyring, buildkite.GithubOAuthToken, &token)
	if err != nil {
		return NewExitError(fmt.Errorf("Error retriving github oauth credentials: %v", err), 1)
	}

	gh := github.NewClientFromToken(&token)

	fmt.Printf("Checking github repository config for %s/%s: ", org, repo)

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Start()

	hooks, _, err := gh.Repositories.ListHooks(context.Background(), "buildkite", "agent", &githubclient.ListOptions{})
	s.Stop()
	if err != nil {
		fmt.Printf("❌\n\n")
		return NewExitError(err, 1)
	}

	fmt.Println()
	isGithubWebhookSetup := false

	debugf("[init] Found %d webhooks", len(hooks))

	for _, hook := range hooks {
		if strings.Contains(hook.GetURL(), "webhook.buildbox.io") || strings.Contains(hook.GetURL(), "webhook.buildkite.com") {
			isGithubWebhookSetup = true
			break
		}
	}

	if isGithubWebhookSetup {
		fmt.Printf("\nThere is already a webhook setup in github, skipping creating one ⚠️\n")
	} else {
		debugf("[init] Creating a webhook")
	}

	if pipelineFileAdded {
		fmt.Println("A pipeline.yml file was created in .buildkite, you will need to manually commit this")
		fmt.Println("For future reference, you can automatically commit this with --commit")
	}

	// fmt.Println(headerColor("Ok! Let's get started with configuring bk 🚀\n"))
	return nil
}
