package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/buildkite/cli/v2/local"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
)

var projectUUID string
var apiEndpoint string
var funProjectUUID string
var funApiEndpoint string

type DocsCommandContext struct {
	TerminalContext
	ConfigContext

	Debug  bool
	Fun    bool
	Prompt string
}

type errMsg error

type markdown string

type Model struct {
	spinner  spinner.Model
	quitting bool
	err      error
	reason   string
	markdown markdown
	cmd      DocsCommandContext
}

type payload struct {
	Params  question `json:"params"`
	Project string   `json:"project,omitempty"`
}

type question struct {
	Question    string   `json:"question"`
	ChatHistory []string `json:"chat_history"`
}

type response struct {
	Output output `json:"output"`
}

type output struct {
	Answer string `json:"answer"`
}

func waitForActivity(m Model) tea.Cmd {
	return func() tea.Msg {
		response, err := LoadDocsCmd(m.cmd)
		if err != nil {
			return err
		}
		if response == "" {
			response = "Something went wrong getting a response"
		}
		return markdown(response)
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, waitForActivity(m))
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		default:
			return m, nil
		}
	case markdown:
		m.markdown = msg
		m.quitting = true
		return m, tea.Quit

	case errMsg:
		m.err = msg
		return m, nil

	default:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
}

func (m Model) View() string {
	if m.err != nil {
		return m.err.Error()
	}

	if m.quitting {
		return string(m.markdown)
	} else {
		return fmt.Sprintf("%s %s...\n", m.spinner.View(), m.reason)
	}
}

func DocsHelp(ctx DocsCommandContext) error {
	s := spinner.New()
	s.Spinner = spinner.Line
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	m := Model{
		reason:  loadReason(),
		spinner: s,
		cmd:     ctx,
	}
	p := tea.NewProgram(m)

	if _, err := p.Run(); err != nil {
		fmt.Println("Could not start program: ", err)
		return err
	}

	return nil
}

func LoadDocsCmd(ctx DocsCommandContext) (string, error) {
	// Enable debug if the --debug flag is enabled
	if ctx.Debug {
		local.Debug = true
	}

	if ctx.Fun {
		// Use the fun URL and project for our responses!
		if project, exists := os.LookupEnv("FUN_RELEVANCE_PROJECT"); exists {
			funProjectUUID = project
			projectUUID = funProjectUUID
		}
		if url, exists := os.LookupEnv("FUN_RELEVANCE_API_URL"); exists {
			funApiEndpoint = url
			apiEndpoint = funApiEndpoint
		}
	} else {
		//Check for Project and API URL, fail if no value set
		if project, exists := os.LookupEnv("RELEVANCE_PROJECT"); exists {
			projectUUID = project
		}
		if url, exists := os.LookupEnv("RELEVANCE_API_URL"); exists {
			apiEndpoint = url
		}
	}
	// Obtain prompt, setup Project, URL, Payload
	prompt := ctx.Prompt

	// we just want to send an empty string for chat history right now to use the chain
	payload := payload{
		Params: question{
			Question:    prompt,
			ChatHistory: []string{},
		},
		Project: projectUUID,
	}

	payloadBytes, err := json.Marshal(payload)
	debugf("Are we sending the question properly?\n %s \n what about the payload:\n %s", payload.Params.Question, payloadBytes)
	if err != nil {
		log.Errorf("🚨 Error %v", err)
		return "", err
	}

	req, err := http.NewRequest("POST", apiEndpoint, bytes.NewBuffer(payloadBytes))
	if err != nil {
		log.Errorf("🚨 Error %v", err)
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}

	debugf("Sending the request to Relevance AI")
	resp, err := client.Do(req)
	if err != nil {
		log.Errorf("🚨 Error %v", err)
		return "", err
	}

	defer resp.Body.Close()

	debugf("Attempting to read response bytes from Relevance AI")
	responseBytes, err := ioutil.ReadAll(resp.Body)
	debugf("Status code %s", resp.Status)
	debugf("Obtained response %s", responseBytes)
	if err != nil {
		log.Errorf("Unable to read response body %v", err)
		return "", err
	}

	var responseBody response

	debugf("Unmarshalling response from Relevance AI")
	err = json.Unmarshal(responseBytes, &responseBody)
	if err != nil {
		log.Errorf("Unable to marshal JSON %v", err)
		return "", err
	}

	debugf("Relevance AI full returned responseBody:\n %s", responseBody.Output.Answer)
	in := responseBody.Output.Answer

	debugf("Rendering Glamour response for output")
	out, err := glamour.Render(in, "dark")

	if err != nil {
		log.Errorf("Error rendering markdown %v", err)
		return "", err
	}
	return out, nil
}

func loadReason() string {
	//create the reasons slice and append reasons to it
	reasons := make([]string, 0)
	reasons = append(reasons,
		"Counting bobcats",
		"Buying helicopters",
		"Spending keithbucks",
		"Chasing cars",
		"Calling JJ",
	)
	rand.Seed(time.Now().Unix())
	n := rand.Int() % len(reasons)
	return reasons[n]

}
