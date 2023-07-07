package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/charmbracelet/glamour"
	"github.com/buildkite/cli/v2/local"
)

type DocsCommandContext struct {
	TerminalContext
	ConfigContext

	Debug        bool
	Prompt     	 string
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

	if ctx.Debug {
		local.Debug = true
	}

	// Obrain prompt, setup Project, URL, Payload
	prompt := ctx.Prompt
	project := os.Getenv("RELEVANCE_PROJECT")
	url := "https://api-f1db6c.stack.tryrelevance.com/latest/studios/4dd12ab5-b49b-483f-8896-04cdf3d7091c/trigger_limited"
	payload := payload{
		Params: question{
			Question: prompt,
		},
		Project: project,
	}

	// If the project is not setup
	if project == "" {
		debugf("🚨 Error: RELEVANCE_PROJECT is not set")
	}

	debugf("Are we sending the question properly?\n %s \n what about the payload:\n %v", payload.Params.Question, payload)
	
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		debugf("🚨 Error %v", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payloadBytes))
	if err != nil {
		debugf("🚨 Error %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}

	resp, err := client.Do(req)
	if err != nil {
		debugf("🚨 Error %v", err)
	}

	// log.Infof("Got the response %v", resp.Body
	defer resp.Body.Close()
	responseBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		debugf("Unable to read response body %v", err)
	}

	// log.Infof("Here is the raw response: %d", resp.Body)

	var responseBody Response

	err = json.Unmarshal(responseBytes, &responseBody)
	if err != nil {
		debugf("Unable to marshal JSON %v", err)
	}

	// log.Infof("Here is the full body:\n %d", responseBody.Output.Answer)
	in := responseBody.Output.Answer
	out, err := glamour.Render(in, "dark")
	
	if err != nil{
		debugf("Error rendering markdown %v", err)
	}
	fmt.Print(out)
	fmt.Print("> ")	
	
	return nil
}
