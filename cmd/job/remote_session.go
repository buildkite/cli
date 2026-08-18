package job

import (
	"context"
	"errors"
	"time"
)

type remoteSession struct {
	Endpoint    string    `json:"endpoint"`
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func (s remoteSession) validate() error {
	switch {
	case s.Endpoint == "":
		return errors.New("without an endpoint")
	case s.AccessToken == "":
		return errors.New("without an access token")
	case s.ExpiresAt.IsZero():
		return errors.New("without an expiry")
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
