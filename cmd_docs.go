package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/log"
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

	// Enable debug if the --debug flag is enabled
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

	// If the project is not setup, error 
	if project == "" {
		log.Errorf("🚨 Error: RELEVANCE_PROJECT is not set")
	}

	debugf("Are we sending the question properly?\n %s \n what about the payload:\n %v", payload.Params.Question, payload)
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Errorf("🚨 Error %v", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payloadBytes))
	if err != nil {
		log.Errorf("🚨 Error %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}

	debugf("Sending the request to Relevance AI")
	resp, err := client.Do(req)
	if err != nil {
		log.Errorf("🚨 Error %v", err)
	}
	debugf("Obtained response %v", resp.Body)

	defer resp.Body.Close()

	debugf("Attempting to read response bytes from Relevance AI")
	responseBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("Unable to read response body %v", err)
	}

	var responseBody Response

	debugf("Unmarshalling response from Relevance AI")
	err = json.Unmarshal(responseBytes, &responseBody)
	if err != nil {
		log.Errorf("Unable to marshal JSON %v", err)
	}

	debugf("Relevance AI rull returned responseBody:\n %d", responseBody.Output.Answer)
	in := responseBody.Output.Answer

	debugf("Rendering Glamour response for output")
	out, err := glamour.Render(in, "dark")
	
	if err != nil{
		log.Errorf("Error rendering markdown %v", err)
	}

	fmt.Print(out)
	fmt.Print("> ")	
	
	return nil
}
