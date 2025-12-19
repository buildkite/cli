package output

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestViewable_TextOutput(t *testing.T) {
	t.Parallel()

	type Data struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	v := Viewable[Data]{
		Data: Data{Name: "test", Value: 42},
		Render: func(d Data) string {
			return "Name: " + d.Name
		},
	}

	got := v.TextOutput()
	want := "Name: test"
	if got != want {
		t.Errorf("TextOutput() = %q, want %q", got, want)
	}
}

func TestViewable_MarshalJSON(t *testing.T) {
	t.Parallel()

	type Data struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	v := Viewable[Data]{
		Data:   Data{Name: "test", Value: 42},
		Render: func(d Data) string { return "" },
	}

	got, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}

	var unmarshaled Data
	if err := json.Unmarshal(got, &unmarshaled); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if unmarshaled.Name != "test" || unmarshaled.Value != 42 {
		t.Errorf("MarshalJSON() produced incorrect data: %+v", unmarshaled)
	}
}

func TestViewable_MarshalYAML(t *testing.T) {
	t.Parallel()

	type Data struct {
		Name  string `yaml:"name"`
		Value int    `yaml:"value"`
	}

	v := Viewable[Data]{
		Data:   Data{Name: "test", Value: 42},
		Render: func(d Data) string { return "" },
	}

	got, err := yaml.Marshal(v)
	if err != nil {
		t.Fatalf("MarshalYAML() error = %v", err)
	}

	var unmarshaled Data
	if err := yaml.Unmarshal(got, &unmarshaled); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if unmarshaled.Name != "test" || unmarshaled.Value != 42 {
		t.Errorf("MarshalYAML() produced incorrect data: %+v", unmarshaled)
	}
}
