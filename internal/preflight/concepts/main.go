// +build ignore

package main

import (
	"fmt"
	"os"
	"strings"

	c1 "github.com/buildkite/cli/v3/internal/preflight/concepts/1"
	c2 "github.com/buildkite/cli/v3/internal/preflight/concepts/2"
	c3 "github.com/buildkite/cli/v3/internal/preflight/concepts/3"
	c4 "github.com/buildkite/cli/v3/internal/preflight/concepts/4"
	c5 "github.com/buildkite/cli/v3/internal/preflight/concepts/5"
)

func main() {
	pick := "all"
	if len(os.Args) > 1 {
		pick = os.Args[1]
	}

	sep := strings.Repeat("═", 76)

	demos := map[string]func(){
		"1": func() {
			fmt.Println(sep)
			fmt.Println("  CONCEPT 1: LAB NOTEBOOK")
			fmt.Println(sep)
			fmt.Println(c1.Demo())
		},
		"2": func() {
			fmt.Println(sep)
			fmt.Println("  CONCEPT 2: OSCILLOSCOPE")
			fmt.Println(sep)
			fmt.Println(c2.Demo())
		},
		"3": func() {
			fmt.Println(sep)
			fmt.Println("  CONCEPT 3: REDLINE (failed)")
			fmt.Println(sep)
			fmt.Println(c3.Demo())
			fmt.Println(sep)
			fmt.Println("  CONCEPT 3: REDLINE (passed)")
			fmt.Println(sep)
			fmt.Println(c3.DemoPassing())
		},
		"4": func() {
			fmt.Println(sep)
			fmt.Println("  CONCEPT 4: SPECTROGRAPH")
			fmt.Println(sep)
			fmt.Println(c4.Demo())
		},
		"5": func() {
			fmt.Println(sep)
			fmt.Println("  CONCEPT 5: APERTURE (running)")
			fmt.Println(sep)
			fmt.Println(c5.DemoRunning())
			fmt.Println(sep)
			fmt.Println("  CONCEPT 5: APERTURE (failed)")
			fmt.Println(sep)
			fmt.Println(c5.Demo())
			fmt.Println(sep)
			fmt.Println("  CONCEPT 5: APERTURE (passed)")
			fmt.Println(sep)
			fmt.Println(c5.DemoPassed())
		},
	}

	if pick == "all" {
		for _, k := range []string{"1", "2", "3", "4", "5"} {
			demos[k]()
		}
		return
	}

	fn, ok := demos[pick]
	if !ok {
		fmt.Fprintf(os.Stderr, "usage: go run main.go [1|2|3|4|5|all]\n")
		os.Exit(1)
	}
	fn()
}
