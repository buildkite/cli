package cli

import (
	"fmt"
)

type DocsCommandContext struct {
	TerminalContext
	ConfigContext

	Debug        bool
	Query     	 string
}

func DocsHelp(ctx DocsCommandContext) error {
	fmt.Println("hello")
	
	return nil
}