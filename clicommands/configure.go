package clicommands

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/99designs/keyring"
	"github.com/buildkite/buildkite-cli/config"
	"github.com/fatih/color"
	"golang.org/x/crypto/ssh/terminal"
	"gopkg.in/alecthomas/kingpin.v2"
)

type ConfigureCommandInput struct {
	Keyring keyring.Keyring
}

func ConfigureConfigureCommand(app *kingpin.Application) {
	input := ConfigureCommandInput{}

	cmd := app.Command("configure", "Configure bk")

	cmd.Action(func(c *kingpin.ParseContext) error {
		input.Keyring = keyringImpl
		ConfigureCommand(app, input)
		return nil
	})
}

func ConfigureCommand(app *kingpin.Application, input ConfigureCommandInput) {
	// termProgram := os.Getenv(`TERM_PROGRAM`)
	// fmt.Printf("Term: %s", termProgram)

	bkColor := color.New(color.Bold, color.FgGreen).SprintFunc()
	boldWhite := color.New(color.Bold, color.FgHiWhite).SprintFunc()

	// Load the config
	config, err := config.Open()
	if err != nil {
		app.Fatalf("Failed to open config file: %v", err)
	}

	fmt.Println(bkColor("Ok! Let's get started with configuring bk 🚀\n") +
		"First we need to generate a Buildkite GraphQL token. Create one at https://buildkite.com/user/api-access-tokens/new. " +
		"Make sure to tick the GraphQL scope at the bottom.\n")

	token, err := readPassword(boldWhite("GraphQL Token"))
	if err != nil {
		app.Fatalf("Failed to read token from terminal")
	}

	username, err := readString(boldWhite("Buildkite Username"))
	if err != nil {
		app.Fatalf("Failed to read token from terminal: %v", err)
	}

	fmt.Println()

	// Set the token in the keyring
	err = keyringImpl.Set(keyring.Item{
		Key:         "graphql-token",
		Label:       "Buildkite GraphQL Token",
		Description: "Buildkite GraphQL Token",
		Data:        []byte(token),
	})
	if err != nil {
		app.Fatalf("Failed to set token into keyring: %v", err)
	}

	fmt.Printf(bkColor("Securely stored graphql token! 💪\n"))

	config.BuildkiteUsername = username

	// write config changes to disk
	if err = config.Write(); err != nil {
		app.Fatalf("Failed to write config: %v", err)
	}

	fmt.Printf(bkColor("Wrote configuration to %s 📝\n"), config.Path)

}

func readPassword(prompt string) (string, error) {
	fmt.Fprintf(os.Stderr, "%s: ", prompt)

	b, err := terminal.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", err
	}
	fmt.Println()
	return string(b), nil
}

func readString(prompt string) (string, error) {
	fmt.Fprintf(os.Stderr, "%s: ", prompt)

	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}
