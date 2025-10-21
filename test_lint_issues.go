package main

import (
	"fmt"
	"os"
	"io/ioutil" // deprecated package
)

// unused function
func unusedFunction() {
	fmt.Println("never called")
}

func messyCode() {
	// ineffective assignment
	x := 5
	x = 10

	// unused variable
	y := "hello"

	// inefficient string concatenation in loop
	result := ""
	for i := 0; i < 100; i++ {
		result = result + "a"
	}

	// unchecked error
	data, _ := ioutil.ReadFile("test.txt")
	fmt.Println(data)

	// unnecessary else
	if x > 5 {
		fmt.Println("big")
	} else {
		fmt.Println("small")
	}
}

// exported function without comment
func BadlyNamedFunc() error {
	// empty if body
	if true {
	}

	return nil
}

func errorHandling() {
	file, err := os.Open("test.txt")
	if err != nil {
		// error not properly wrapped
		return
	}
	file.Close() // defer not used
}
