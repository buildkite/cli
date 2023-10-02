package cli

import (
	"bufio"
	"errors"
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

func (t *Terminal) Failure(msg string) {
	t.Printf(color.RedString("%s ❌\n"), msg)
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
	Println(msg string)
	Success(msg string)
	Failure(msg string)
}

func (t *Terminal) Try() Tryer {
	return &tryPrompt{terminal: t, spinner: t.Spinner()}
}

type tryPrompt struct {
	terminal *Terminal
	message  string
	spinner  Spinner
}

func (t *tryPrompt) Start(msg string) {
	t.message = msg
	t.terminal.Printf(color.WhiteString("%s: ", t.message))
	t.spinner.Start()
}

func (t *tryPrompt) Println(msg string) {
	t.spinner.Stop()
	fmt.Printf("\r")
	fmt.Printf(cursor.ClearEntireLine())
	t.terminal.Println(msg)
	t.terminal.Printf(color.WhiteString("%s: ", t.message))
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

type ExitCoder interface {
	ExitCode() int
}

type ExitError struct {
	err        error
	exitCode   int
	suggestion string
}

func NewExitError(err error, exitCode int) ExitError {
	return ExitError{err: err, exitCode: exitCode}
}

func NewExitErrorSuggestion(err error, exitCode int, suggestion string) ExitError {
	return ExitError{err: err, exitCode: exitCode, suggestion: suggestion}
}

func NewExitErrorString(msg string, exitCode int) ExitError {
	return NewExitError(errors.New(msg), exitCode)
}

func NewExitErrorStringSuggestion(msg string, exitCode int, suggestion string) ExitError {
	return NewExitErrorSuggestion(errors.New(msg), exitCode, suggestion)
}

func (ex ExitError) ExitCode() int {
	return ex.exitCode
}

func (ex ExitError) Error() string {
	return ex.err.Error()
}

func (ex ExitError) HasSuggestion() bool {
	return ex.suggestion != ""
}

func (ex ExitError) Suggestion() string {
	return ex.suggestion
}
