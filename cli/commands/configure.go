package commands

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/99designs/keyring"
	"github.com/briandowns/spinner"
	"github.com/buildkite/buildkite-cli/buildkite"
	"github.com/buildkite/buildkite-cli/buildkite/graphql"
	"github.com/buildkite/buildkite-cli/integrations/github"
	"github.com/fatih/color"
)

type ConfigureCommandInput struct {
	Keyring   keyring.Keyring
	Debug     bool
	DebugHTTP bool
}

func ConfigureDefaultCommand(input ConfigureCommandInput) error {
	fmt.Println(headerColor("Ok! Let's get started with configuring bk 🚀\n"))

	if err := ConfigureBuildkiteRestCommand(input); err != nil {
		return err
	}

	fmt.Println()
	fmt.Printf("For now, we will assume you are using Github. Support for others is coming soon! 😓\n\n")

	if err := ConfigureGithubCommand(input); err != nil {
		return err
	}

	fmt.Printf("\nOk, you are good to go!\n")
	return nil
}

func ConfigureGithubCommand(input ConfigureCommandInput) error {
	header("Let's configure your github.com credentials 💻")

	fmt.Printf("We need to authorize this app to access your repositories. " +
		"This authorization is stored securely locally, buildkite.com never gets access to it.\n\n")

	waitForKeyPress(color.WhiteString("When you press enter, your default browser will open and authenticate to github.com"))

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Start()

	token, err := github.Authenticate()
	s.Stop()

	if err != nil {
		fmt.Printf("❌\n\n")
		return NewExitError(fmt.Errorf("Github OAuth error: %v", err), 1)
	}

	client := github.NewClientFromToken(token)
	user, _, err := client.Users.Get(context.Background(), "")
	if err != nil {
		return NewExitError(fmt.Errorf("Github Users.Get() failed: %v", err), 1)
	}

	fmt.Println()
	fmt.Printf(color.GreenString("Authenticated as %s ✅\n"), *user.Login)

	if err = buildkite.StoreCredential(input.Keyring, buildkite.GithubOAuthToken, token); err != nil {
		return NewExitError(err, 1)
	}

	fmt.Printf(color.GreenString("Securely stored Github token! 💪\n"))
	return nil
}

func ConfigureBuildkiteRestCommand(input ConfigureCommandInput) error {
	header("Configuring Buildkite REST credentials")

	return errors.New("Not implemented")
}

func ConfigureBuildkiteGraphqlCommand(input ConfigureCommandInput) error {
	header("Configuring Buildkite Graphql credentials")

	config, err := buildkite.OpenConfig()
	if err != nil {
		return NewExitError(fmt.Errorf("Failed to open config file: %v", err), 1)
	}

	fmt.Printf("Create a GraphQL token at https://buildkite.com/user/api-access-tokens/new. " +
		"Make sure to tick the GraphQL scope at the bottom.\n\n")

	token, err := readPassword(color.WhiteString("GraphQL Token"))
	if err != nil {
		return NewExitError(fmt.Errorf("Failed to read token from terminal: %v", err), 1)
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Start()

	if input.DebugHTTP {
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

	fmt.Printf(color.GreenString("%s ✅\n\n"),
		userQueryResponse.Data.Viewer.User.Email)

	if err = buildkite.StoreCredential(input.Keyring, buildkite.BuildkiteGraphQLToken, token); err != nil {
		return NewExitError(err, 1)
	}

	fmt.Printf(color.GreenString("Securely stored GraphQL token! 💪\n"))

	config.BuildkiteEmail = userQueryResponse.Data.Viewer.User.Email
	config.BuildkiteUUID = userQueryResponse.Data.Viewer.User.UUID

	// write config changes to disk
	if err = config.Write(); err != nil {
		return NewExitError(fmt.Errorf("Failed to write config: %v", err), 1)
	}

	fmt.Printf(color.GreenString("Wrote configuration to %s 📝\n"), config.Path)

	return nil
}
