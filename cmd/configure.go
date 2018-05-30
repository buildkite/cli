package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/99designs/keyring"
	"github.com/briandowns/spinner"
	"github.com/buildkite/cli/pkg"
	"github.com/buildkite/cli/pkg/config"
	"github.com/buildkite/cli/pkg/github"
	"github.com/buildkite/cli/pkg/graphql"
	"github.com/fatih/color"
)

type ConfigureCommandInput struct {
	Keyring   keyring.Keyring
	Debug     bool
	DebugHTTP bool
	Terminal  interface {
		Header(h string)
		Println(s ...interface{})
		Printf(s string, v ...interface{})
		WaitForKeyPress(prompt string)
		Spinner() pkg.Spinner
		ReadPassword(prompt string) (string, error)
	}
}

func ConfigureDefaultCommand(i ConfigureCommandInput) error {
	i.Terminal.Header("Ok! Let's get started with configuring bk 🚀\n")

	if err := ConfigureBuildkiteGraphQLCommand(i); err != nil {
		return err
	}

	i.Terminal.Println()
	i.Terminal.Printf("For now, we will assume you are using Github. " +
		"Support for others is coming soon! 😓\n\n")

	if err := ConfigureGithubCommand(i); err != nil {
		return err
	}

	i.Terminal.Printf("\nOk, you are good to go!\n")
	return nil
}

func ConfigureGithubCommand(i ConfigureCommandInput) error {
	i.Terminal.Header("Let's configure your github.com credentials 💻")

	i.Terminal.Printf("We need to authorize this app to access your repositories. " +
		"This authorization is stored securely locally, buildkite.com never gets access to it.\n\n")

	i.Terminal.WaitForKeyPress(color.WhiteString("When you press enter, your default browser will open and authenticate to github.com"))

	s := i.Terminal.Spinner()
	s.Start()

	token, err := github.Authenticate()
	s.Stop()

	if err != nil {
		i.Terminal.Printf("❌\n\n")
		return NewExitError(fmt.Errorf("Github OAuth error: %v", err), 1)
	}

	client := github.NewClientFromToken(token)
	user, _, err := client.Users.Get(context.Background(), "")
	if err != nil {
		return NewExitError(fmt.Errorf("Github Users.Get() failed: %v", err), 1)
	}

	i.Terminal.Println()
	i.Terminal.Printf(color.GreenString("Authenticated as %s ✅\n"), *user.Login)

	if err = config.StoreCredential(i.Keyring, config.GithubOAuthToken, token); err != nil {
		return NewExitError(err, 1)
	}

	i.Terminal.Printf(color.GreenString("Securely stored Github token! 💪\n"))
	return nil
}

func ConfigureBuildkiteGraphQLCommand(i ConfigureCommandInput) error {
	i.Terminal.Header("Configuring Buildkite GraphQL credentials")

	cfg, err := config.Open()
	if err != nil {
		return NewExitError(fmt.Errorf("Failed to open config file: %v", err), 1)
	}

	i.Terminal.Printf("Create a GraphQL token at https://buildkite.com/user/api-access-tokens/new. " +
		"Make sure to tick the GraphQL scope at the bottom.\n\n")

	token, err := i.Terminal.ReadPassword(color.WhiteString("GraphQL Token"))
	if err != nil {
		return NewExitError(fmt.Errorf("Failed to read token from terminal: %v", err), 1)
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Start()

	if i.DebugHTTP {
		graphql.DebugHTTP = true
	}

	client, err := graphql.NewClient(token)
	if err != nil {
		return NewExitError(fmt.Errorf("Failed to create a client: %v", err), 1)
	}

	resp, err := client.Do(`{ viewer { user { email, uuid } } }`)
	s.Stop()

	if err != nil {
		fmt.Printf("❌\n\n")
		return NewExitError(err, 1)
	}

	var userQueryResponse struct {
		Data struct {
			Viewer struct {
				User struct {
					Email string `json:"email"`
					UUID  string `json:"uuid"`
				} `json:"user"`
			} `json:"viewer"`
		} `json:"data"`
	}

	if err = resp.DecodeInto(&userQueryResponse); err != nil {
		return NewExitError(fmt.Errorf("Failed to parse GraphQL response: %v", err), 1)
	}

	i.Terminal.Printf(color.GreenString("%s ✅\n\n"),
		userQueryResponse.Data.Viewer.User.Email)

	if err = config.StoreCredential(i.Keyring, config.BuildkiteGraphQLToken, token); err != nil {
		return NewExitError(err, 1)
	}

	i.Terminal.Printf(color.GreenString("Securely stored GraphQL token! 💪\n"))

	cfg.BuildkiteEmail = userQueryResponse.Data.Viewer.User.Email
	cfg.BuildkiteUUID = userQueryResponse.Data.Viewer.User.UUID

	if err = cfg.Write(); err != nil {
		return NewExitError(fmt.Errorf("Failed to write config: %v", err), 1)
	}

	i.Terminal.Printf(color.GreenString("Wrote configuration to %s 📝\n"), config.Path)
	return nil
}
