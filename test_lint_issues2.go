package main

import (
	"errors"
	"fmt"
)

// ExportedType without proper documentation
type ExportedType struct {
	Name string
	age  int // mixing exported and unexported fields
}

func (e ExportedType) String() string {
	return e.Name
}

// pointer receiver not used consistently
func (e ExportedType) Method1() {
	fmt.Println(e.Name)
}

func (e *ExportedType) Method2() {
	fmt.Println(e.Name)
}

func poorErrorHandling() error {
	// creating errors without context
	err := errors.New("something bad happened")

	// comparing errors with ==
	if err == errors.New("something bad happened") {
		return err
	}

	// naked return in long function
	return nil
}

func complexFunction(a, b, c, d, e, f, g, h, i, j int) int {
	// too many parameters
	// high cyclomatic complexity
	if a > 0 {
		if b > 0 {
			if c > 0 {
				if d > 0 {
					if e > 0 {
						if f > 0 {
							return g + h + i + j
						}
					}
				}
			}
		}
	}
	return 0
}

var GlobalVariable = "bad" // unexported global starting with capital
