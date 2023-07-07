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


type response struct {
	Status    string    `json:"status"`
	Errors    []string  `json:"errors"`
	Output    output    `json:"output"`
	Credits   []credit  `json:"credits_used"`
	ExecTime  int       `json:"executionTime"`
	Cost      float64   `json:"cost"`
}

type output struct {
	Answer            string   `json:"answer"`
	Prompt            string   `json:"prompt"`
	UserKeyUsed       bool     `json:"user_key_used"`
	ValidationHistory []string `json:"validation_history"`
	CreditsCost       float64  `json:"credits_cost"`
}

type credit struct {
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

	// Obtain prompt, setup Project, URL, Payload
	prompt := ctx.Prompt
	//Check for Project and API URL, fail if no value set
	project, exists := os.LookupEnv("RELEVANCE_PROJECT")
	if !exists {
		log.Errorf("🚨 Error: RELEVANCE_PROJECT is not set")
	}
	url, exists := os.LookupEnv("RELEVANCE_API_URL")
	if !exists {
		log.Errorf("🚨 Error: RELEVANCE_URL is not set")
		return nil
	}


	payload := payload{
		Params: question{
			Question: prompt,
		},
		Project: project,
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

	var responseBody response

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
	return nil
}
