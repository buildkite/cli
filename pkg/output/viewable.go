package output

import (
	"encoding/json"
)

// Viewable wraps any type to provide formatted output support.
// It delegates JSON/YAML marshaling directly to the underlying data,
// while using a custom render function for text output.
type Viewable[T any] struct {
	Data   T
	Render func(T) string
}

// TextOutput implements the Formatter interface for text output.
func (v Viewable[T]) TextOutput() string {
	return v.Render(v.Data)
}

// MarshalJSON delegates JSON marshaling to the underlying data.
func (v Viewable[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.Data)
}

// MarshalYAML delegates YAML marshaling to the underlying data.
func (v Viewable[T]) MarshalYAML() (interface{}, error) {
	return v.Data, nil
}
