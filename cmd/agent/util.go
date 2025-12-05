package agent

import (
	"net/url"
	"strings"

	"github.com/buildkite/cli/v3/internal/config"
)

func parseAgentArg(agent string, conf *config.Config) (string, string) {
	var org, id string
	agentIsURL := strings.Contains(agent, ":")
	agentIsSlug := !agentIsURL && strings.Contains(agent, "/")

	if agentIsURL {
		url, err := url.Parse(agent)
		if err != nil {
			return "", ""
		}
		part := strings.Split(url.Path, "/")
		if part[3] == "agents" {
			org, id = part[2], part[4]
		} else {
			org, id = part[2], part[len(part)-1]
		}
	} else {
		if agentIsSlug {
			part := strings.Split(agent, "/")
			org, id = part[0], part[1]
		} else {
			org = conf.OrganizationSlug()
			id = agent
		}
	}

	return org, id
}
