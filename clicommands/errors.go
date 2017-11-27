package clicommands

import "errors"

type ExitCoder interface {
	ExitCode() int
}

type ExitError struct {
	err      error
	exitCode int
}

func NewExitError(err error, exitCode int) ExitError {
	return ExitError{err: err, exitCode: exitCode}
}

func NewExitErrorString(msg string, exitCode int) ExitError {
	return NewExitError(errors.New(msg), exitCode)
}

func (ex ExitError) ExitCode() int {
	return ex.exitCode
}

func (ex ExitError) Error() string {
	return ex.err.Error()
}
