package models

// Annotation represents a Buildkite annotation on a build
type Annotation struct {
	ID          string `json:"id,omitempty" yaml:"id,omitempty"`
	Context     string `json:"context,omitempty" yaml:"context,omitempty"`
	Style       string `json:"style,omitempty" yaml:"style,omitempty"`
	Body        string `json:"body,omitempty" yaml:"body,omitempty"`
	BodyHTML    string `json:"body_html,omitempty" yaml:"body_html,omitempty"`
	CreatedAt   string `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	CreatedBy   *User  `json:"created_by,omitempty" yaml:"created_by,omitempty"`
}