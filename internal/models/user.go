package models

// User represents a Buildkite user
type User struct {
	ID        string `json:"id,omitempty" yaml:"id,omitempty"`
	GraphQLID string `json:"graphql_id,omitempty" yaml:"graphql_id,omitempty"`
	Name      string `json:"name,omitempty" yaml:"name,omitempty"`
	Email     string `json:"email,omitempty" yaml:"email,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty" yaml:"avatar_url,omitempty"`
	CreatedAt string `json:"created_at,omitempty" yaml:"created_at,omitempty"`
}