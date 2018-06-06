package cli

import (
	"context"
	"fmt"

	"github.com/buildkite/cli/config"
	"github.com/buildkite/cli/github"
	"github.com/buildkite/cli/graphql"
	"github.com/fatih/color"
)

type ConfigureCommandContext struct {
	TerminalContext
	KeyringContext

	Debug        bool
	DebugGraphQL bool
}

func ConfigureDefaultCommand(ctx ConfigureCommandContext) error {
	ctx.Header("Ok! Let's get started with configuring bk 🚀\n")

	if err := ConfigureBuildkiteGraphQLCommand(ctx); err != nil {
		return err
	}

	ctx.Println()
	ctx.Printf("For now, we will assume you are using Github. " +
		"Support for others is coming soon! 😓\n\n")

	if err := ConfigureGithubCommand(ctx); err != nil {
		return err
	}

	ctx.Printf("\nOk, you are good to go!\n")
	return nil
}

func ConfigureGithubCommand(ctx ConfigureCommandContext) error {
	ctx.Header("Let's configure your github.com credentials 💻")

	ctx.Printf("We need to authorize this app to access your repositories. " +
		"This authorization is stored securely locally, buildkite.com never gets access to it.\n\n")

	ctx.WaitForKeyPress(color.WhiteString("When you press enter, your default browser will open and authenticate to github.com"))

	s := ctx.Spinner()
	s.Start()

	token, err := github.Authenticate()
	s.Stop()

	if err != nil {
		ctx.Printf("❌\n\n")
		return NewExitError(fmt.Errorf("Github OAuth error: %v", err), 1)
	}

	client := github.NewClientFromToken(token)
	user, _, err := client.Users.Get(context.Background(), "")
	if err != nil {
		return NewExitError(fmt.Errorf("Github Users.Get() failed: %v", err), 1)
	}

	ctx.Println()
	ctx.Printf(color.GreenString("Authenticated as %s ✅\n"), *user.Login)

	if err = config.StoreCredential(ctx.Keyring, config.GithubOAuthToken, token); err != nil {
		return NewExitError(err, 1)
	}

	ctx.Printf(color.GreenString("Securely stored Github token! 💪\n"))
	return nil
}

func ConfigureBuildkiteGraphQLCommand(ctx ConfigureCommandContext) error {
	ctx.Header("Configuring Buildkite GraphQL credentials")

	cfg, err := config.Open()
	if err != nil {
		return NewExitError(fmt.Errorf("Failed to open config file: %v", err), 1)
	}

	ctx.Printf("Create a GraphQL token at https://buildkite.com/user/api-access-tokens/new. " +
		"Make sure to tick the GraphQL scope at the bottom.\n\n")

	token, err := ctx.ReadPassword(color.WhiteString("GraphQL Token"))
	if err != nil {
		return NewExitError(fmt.Errorf("Failed to read token from terminal: %v", err), 1)
	}

	s := ctx.Spinner()
	s.Start()

	client, err := graphql.NewClient(token)
	if err != nil {
		return NewExitError(fmt.Errorf("Failed to create a client: %v", err), 1)
	}

	resp, err := client.Do(`
		query {
			viewer {
				user {
					email
					uuid
				}
			}
		}
	`, nil)

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

	ctx.Printf(color.GreenString("%s ✅\n\n"),
		userQueryResponse.Data.Viewer.User.Email)

	if err = config.StoreCredential(ctx.Keyring, config.BuildkiteGraphQLToken, token); err != nil {
		return NewExitError(err, 1)
	}

	ctx.Printf(color.GreenString("Securely stored GraphQL token! 💪\n"))

	cfg.BuildkiteEmail = userQueryResponse.Data.Viewer.User.Email
	cfg.BuildkiteUUID = userQueryResponse.Data.Viewer.User.UUID

	if err = cfg.Write(); err != nil {
		return NewExitError(fmt.Errorf("Failed to write config: %v", err), 1)
	}

	ctx.Printf(color.GreenString("Wrote configuration to %s 📝\n"), config.Path)
	return nil
}
