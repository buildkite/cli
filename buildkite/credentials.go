package buildkite

import (
	"encoding/json"
	"errors"

	"github.com/99designs/keyring"
)

type CredentialType struct {
	Label string
	Key   string
}

var (
	BuildkiteGraphQLToken = CredentialType{"Buildkite GraphQL Token", "graphql-token"}
	BuildkiteRestToken    = CredentialType{"Buildkite REST Token", "rest-token"}
	GithubOAuthToken      = CredentialType{"Github OAuth Token", "rest-token"}
)

func ListCredentials() []CredentialType {
	return nil
}

func StoreCredential(kr keyring.Keyring, t CredentialType, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return NewExitError(err, 1)
	}
	return kr.Set(keyring.Item{
		Key:   t.Key,
		Label: t.Label,
		Data:  data,
	})
}

func RetrieveCredential(kr keyring.Keyring, t CredentialType, into interface{}) error {
	return nil, errors.New("Not implemented")
}
