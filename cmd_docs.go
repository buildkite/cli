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

type payload struct{
	Params  question `json:"params,omitempty"`
	Project string   `json:"project,omitempty"`
}

type question struct {
	Question string `json:"question,omitempty"`
}

type Response struct {
	Status    string    `json:"status"`
	Errors    []string  `json:"errors"`
	Output    Output    `json:"output"`
	Credits   []Credit  `json:"credits_used"`
	ExecTime  int       `json:"executionTime"`
	Cost      float64   `json:"cost"`
}

type Output struct {
	Answer            string   `json:"answer"`
	Prompt            string   `json:"prompt"`
	UserKeyUsed       bool     `json:"user_key_used"`
	ValidationHistory []string `json:"validation_history"`
	CreditsCost       float64  `json:"credits_cost"`
}

type Credit struct {
	Credits     float64 `json:"credits"`
	Name        string  `json:"name"`
	Multiplier  float64 `json:"multiplier,omitempty"`
	NumUnits    float64 `json:"num_units,omitempty"`
}

func DocsHelp(ctx DocsCommandContext) error {
	fmt.Println("hello")
}
