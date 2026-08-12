package job

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type remoteSession struct {
	Endpoint    string    `json:"endpoint"`
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func (s remoteSession) validate(kind string) error {
	switch {
	case s.Endpoint == "":
		return fmt.Errorf("buildkite API returned %s session without an endpoint", kind)
	case s.AccessToken == "":
		return fmt.Errorf("buildkite API returned %s session without an access token", kind)
	case s.ExpiresAt.IsZero():
		return fmt.Errorf("buildkite API returned %s session without an expiry", kind)
	default:
		return nil
	}
}

type remoteAccessToken string

func (t remoteAccessToken) IssueToken(context.Context, time.Duration, bool) (string, error) {
	if t == "" {
		return "", errors.New("remote access token is empty")
	}
	return string(t), nil
}
