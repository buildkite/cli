package local

import (
	"errors"
	"sync"
)

type agent struct {
	ID          string
	AccessToken string
	Name        string
	State       string
}

type agentPool struct {
	sync.Mutex
	agents map[string]*agent
}

func newAgentPool() *agentPool {
	return &agentPool{
		agents: map[string]*agent{},
	}
}

func (ap *agentPool) Register(a agent) {
	ap.Lock()
	defer ap.Unlock()
	ap.agents[a.ID] = &a
}

func (ap *agentPool) Get(agentID string) (*agent, error) {
	ap.Lock()
	defer ap.Unlock()
	agent, ok := ap.agents[agentID]
	if !ok {
		return nil, errors.New("No agent found")
	}
	return agent, nil
}

func (ap *agentPool) Connect(agentID string) {
	ap.Lock()
	defer ap.Unlock()
	agent := ap.agents[agentID]
	agent.State = "connected"
}

func (ap *agentPool) Disconnect(agentID string) {
	ap.Lock()
	defer ap.Unlock()
	agent := ap.agents[agentID]
	agent.State = "disconnected"
}

func (ap *agentPool) Authenticate(accessToken string) (string, error) {
	ap.Lock()
	defer ap.Unlock()
	for _, agent := range ap.agents {
		if agent.AccessToken == accessToken {
			return agent.ID, nil
		}
	}
	return "", errors.New("No agent exists with that agent token")
}
