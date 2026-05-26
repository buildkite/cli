package job

import "fmt"

func configuredOrganization(organization string) (string, error) {
	if organization == "" {
		return "", fmt.Errorf("no organization configured. Run bk auth login, or bk use, to set an organization")
	}

	return organization, nil
}
