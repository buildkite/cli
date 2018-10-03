package local

import (
	"encoding/json"
)

type step struct {
	Command *commandStep `json:"-"`
	Wait    *waitStep    `json:"-"`
	Block   *blockStep   `json:"-"`
	Trigger *triggerStep `json:"-"`
}

func (s *step) UnmarshalJSON(data []byte) error {
	var intermediate map[string]interface{}

	if err := json.Unmarshal(data, &intermediate); err != nil {
		return err
	}

	if _, ok := intermediate["wait"]; ok {
		return json.Unmarshal(data, &s.Wait)
	}

	if _, ok := intermediate["block"]; ok {
		return json.Unmarshal(data, &s.Block)
	}

	if _, ok := intermediate["trigger"]; ok {
		return json.Unmarshal(data, &s.Trigger)
	}

	return json.Unmarshal(data, &s.Command)
}

type blockStep struct {
	Block string `json:"block"`
}

type waitStep struct {
	Wait              string `json:"wait"`
	ContinueOnFailure bool   `json:"continue_on_failure"`
}

type triggerStep struct {
	Trigger string `json:"trigger"`
}

type commandStep struct {
	Label    string   `json:"label"`
	Commands []string `json:"-"`
	Plugins  []Plugin `json:"-"`
}

func (s *commandStep) UnmarshalJSON(data []byte) error {
	var intermediate struct {
		Label    string        `json:"label"`
		Commands stringOrSlice `json:"commands"`
		Command  stringOrSlice `json:"command"`
	}

	if err := json.Unmarshal(data, &intermediate); err != nil {
		return err
	}

	s.Label = intermediate.Label

	// Normalize command vs commands
	s.Commands = append(s.Commands, intermediate.Command...)
	s.Commands = append(s.Commands, intermediate.Commands...)

	var pluginSlice struct {
		Plugins []map[string]interface{} `json:"plugins"`
	}

	if err := json.Unmarshal(data, &pluginSlice); err == nil {
		for _, p := range pluginSlice.Plugins {
			for k, v := range p {
				s.Plugins = append(s.Plugins, Plugin{
					Name:   k,
					Params: v.(map[string]interface{}),
				})
			}
		}
	}

	var pluginMap struct {
		Plugins map[string]interface{} `json:"plugins"`
	}

	if err := json.Unmarshal(data, &pluginMap); err == nil {
		for k, v := range pluginMap.Plugins {
			s.Plugins = append(s.Plugins, Plugin{
				Name:   k,
				Params: v.(map[string]interface{}),
			})
		}
	}

	return nil
}

type pipelineUpload struct {
	Pipeline pipeline `json:"pipeline"`
	Replace  bool     `json:"replace"`
}

type pipeline struct {
	Steps []step            `json:"steps"`
	Env   map[string]string `json:"env"`
}

type stringOrSlice []string

func (s *stringOrSlice) UnmarshalJSON(data []byte) error {
	var str string

	if err := json.Unmarshal(data, &str); err == nil {
		*s = []string{str}
		return nil
	}

	var strSlice []string

	if err := json.Unmarshal(data, &strSlice); err != nil {
		return err
	}

	*s = strSlice
	return nil
}
