package pkg

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ahmetb/go-cursor"
	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	sshterminal "golang.org/x/crypto/ssh/terminal"
)

var (
	headerColorFunc = color.New(color.Bold, color.FgGreen).SprintfFunc()
)

type Terminal struct {
}

func (t *Terminal) Header(h string) {
	fmt.Printf(headerColorFunc("──── " + h + "\n\n"))
}

func (t *Terminal) WaitForKeyPress(prompt string) {
	fmt.Fprintf(os.Stderr, "%s", prompt)
	var input string
	fmt.Scanln(&input)
}

func (t *Terminal) ReadPassword(prompt string) (string, error) {
	fmt.Fprintf(os.Stderr, "%s: ", prompt)

	b, err := sshterminal.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (t *Terminal) ReadString(prompt string) (string, error) {
	fmt.Fprintf(os.Stderr, "%s: ", prompt)

	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}

func (t *Terminal) Println(s ...interface{}) {
	fmt.Println(s...)
}

func (t *Terminal) Printf(s string, v ...interface{}) {
	fmt.Printf(s, v...)
}

type Spinner interface {
	Start()
	Stop()
}

func (t *Terminal) Spinner() Spinner {
	return spinner.New(spinner.CharSets[14], 100*time.Millisecond)
}

type Tryer interface {
	Start(msg string)
	Success(msg string)
	Failure(msg string)
}

func (t *Terminal) Try() Tryer {
	return &tryPrompt{terminal: t, spinner: t.Spinner()}
}

type tryPrompt struct {
	terminal *Terminal
	spinner  Spinner
}

func (t *tryPrompt) Start(msg string) {
	t.terminal.Printf(color.WhiteString("%s: "), msg)
	t.spinner.Start()
}

func (t *tryPrompt) Success(msg string) {
	t.spinner.Stop()
	cursor.MoveLeft(2)
	cursor.ClearLineRight()
	t.terminal.Printf(color.GreenString("%s ✅\n"), msg)
}

func (t *tryPrompt) Failure(msg string) {
	t.spinner.Stop()
	cursor.MoveLeft(2)
	cursor.ClearLineRight()
	t.terminal.Printf(color.RedString("%s ❌\n"), msg)
}
