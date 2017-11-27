package commands

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"golang.org/x/crypto/ssh/terminal"
)

var (
	headerColor = color.New(color.Bold, color.FgGreen).SprintfFunc()
)

func header(h string) {
	fmt.Printf(headerColor("──── " + h + "\n\n"))
}

func waitForKeyPress(prompt string) {
	fmt.Fprintf(os.Stderr, "%s", prompt)
	var input string
	fmt.Scanln(&input)
}

func readPassword(prompt string) (string, error) {
	fmt.Fprintf(os.Stderr, "%s: ", prompt)

	b, err := terminal.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", err
	}
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
