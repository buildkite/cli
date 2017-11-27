package clicommands

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/99designs/keyring"
	"github.com/briandowns/spinner"
	"github.com/buildkite/buildkite-cli/api"
	"github.com/buildkite/buildkite-cli/config"
	"github.com/buildkite/buildkite-cli/github"
	"github.com/fatih/color"
)

type ConfigureCommandInput struct {
	Keyring   keyring.Keyring
	Debug     bool
	DebugHTTP bool
}

func ConfigureCommand(input ConfigureCommandInput) error {
	fmt.Println(color.WhiteString("Ok! Let's get started with configuring bk 🚀\n"))

	if err := ConfigureBuildkiteCommand(input); err != nil {
		return err
	}

	fmt.Println()

	if err := ConfigureGithubCommand(input); err != nil {
		return err
	}

	fmt.Printf("\nOk, you are good to go!\n")
	return nil
}

func ConfigureGithubCommand(input ConfigureCommandInput) error {
	fmt.Printf("Let's configure your github.com credentials 💻\n")
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

	tokenData, err := json.Marshal(token)
	if err != nil {
		return NewExitError(err, 1)
	}

	// Set the OAuth token in the keyring
	err = input.Keyring.Set(keyring.Item{
		Key:         "github-token",
		Label:       "Buildkite Github OAuth Token",
		Description: "Buildkite Github OAuth Token",
		Data:        tokenData,
	})
	if err != nil {
		return NewExitError(fmt.Errorf("Failed to set token into keyring: %v", err), 2)
	}

	fmt.Printf(color.GreenString("Securely stored Github token! 💪\n"))
	return nil
}

func ConfigureBuildkiteCommand(input ConfigureCommandInput) error {
	config, err := config.Open()
	if err != nil {
		return NewExitError(fmt.Errorf("Failed to open config file: %v", err), 1)
	}

	fmt.Printf("We need to generate a Buildkite GraphQL token. Create one at https://buildkite.com/user/api-access-tokens/new. " +
		"Make sure to tick the GraphQL scope at the bottom.\n\n")

	token, err := readPassword(color.WhiteString("GraphQL Token"))
	if err != nil {
		return NewExitError(fmt.Errorf("Failed to read token from terminal: %v", err), 1)
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Start()

	if input.DebugHTTP {
		api.DebugHTTP = true
	}

	client, err := api.NewClient(token)
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

	// Set the token in the keyring
	err = input.Keyring.Set(keyring.Item{
		Key:         "graphql-token",
		Label:       "Buildkite GraphQL Token",
		Description: "Buildkite GraphQL Token",
		Data:        []byte(token),
	})
	if err != nil {
		return NewExitError(fmt.Errorf("Failed to set token into keyring: %v", err), 2)
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
