package github

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"time"

	githubclient "github.com/google/go-github/github"
	githuboauth "golang.org/x/oauth2/github"

	"github.com/skratchdot/open-golang/open"
	"golang.org/x/oauth2"
)

const (
	oauthClientID       = "d5a2938d576279fbc995"
	oauthScopes         = "user:email,repo"
	githubStageOneURI   = "https://github.com/login/device/code"
	githubStageThreeURI = "https://github.com/login/oauth/access_token"
	githubGrantType     = "urn:ietf:params:oauth:grant-type:device_code"
)

type githubStageOneResponse struct {
	deviceCode      string
	userCode        string
	verificationURI string
	expiresAt       time.Time
	interval        int
}

func NewClientFromToken(token *oauth2.Token) *githubclient.Client {
	oauthConf := oauth2.Config{
		ClientID: oauthClientID,
		Endpoint: githuboauth.Endpoint,
	}
	oauthClient := oauthConf.Client(oauth2.NoContext, token)
	return githubclient.NewClient(oauthClient)
}

func Authenticate() (*oauth2.Token, error) {
	stageOne, err := requestGitHubStageOne()
	if err != nil {
		return nil, err
	}
	fmt.Println("User Code:", stageOne.userCode)

	if err := open.Run(stageOne.verificationURI); err != nil {
		return nil, err
	}

	token, err := requestGitHubStageThree(stageOne)
	if err != nil {
		return nil, err
	}
	return token, nil
}

func requestGitHubStageOne() (*githubStageOneResponse, error) {
	uri := fmt.Sprintf("%s?client_id=%s&scope=%s", githubStageOneURI, oauthClientID, oauthScopes)
	resp, err := http.Post(uri, "application/json", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub returned non-200 response (%d)", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	params, err := parseQueryWithSingleValues(string(body))
	if err != nil {
		return nil, err
	}

	response := githubStageOneResponse{}

	if deviceCode, ok := params["device_code"]; ok {
		response.deviceCode = deviceCode
	}
	if userCode, ok := params["user_code"]; ok {
		response.userCode = userCode
	}
	if verificationURI, ok := params["verification_uri"]; ok {
		response.verificationURI = verificationURI
	}
	if expiresIn, ok := params["expires_in"]; ok {
		expiresInInt, err := strconv.Atoi(expiresIn)
		if err != nil {
			return nil, err
		}
		response.expiresAt = time.Now().Add(time.Second * time.Duration(expiresInInt))
	}
	if interval, ok := params["interval"]; ok {
		intervalInt, err := strconv.Atoi(interval)
		if err != nil {
			return nil, err
		}
		response.interval = intervalInt
	}

	return &response, nil
}

func requestGitHubStageThree(stageOne *githubStageOneResponse) (*oauth2.Token, error) {

	uri := fmt.Sprintf("%s?client_id=%s&device_code=%s&grant_type=%s", githubStageThreeURI, oauthClientID, stageOne.deviceCode, githubGrantType)

	for {
		fmt.Printf(".")

		resp, err := http.Post(uri, "application/json", nil)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("GitHub returned non-200 response (%d)", resp.StatusCode)
		}

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		params, err := parseQueryWithSingleValues(string(body))
		if err != nil {
			return nil, err
		}

		if errType, ok := params["error"]; ok {
			if errType != "authorization_pending" {
				return nil, fmt.Errorf("Error response from GitHub: %s (%s)", params["error_description"], errType)
			}
		}

		if accessToken, ok := params["access_token"]; ok {
			fmt.Printf("\n")
			token := oauth2.Token{AccessToken: accessToken}
			return &token, nil
		}

		time.Sleep(time.Second * time.Duration(stageOne.interval))
	}
}

// Takes a query string like:
//
//    foo=bar&baz=1
//
// .. and returns a map[string]string
//
//    {"foo" => "bar", "baz" => "1"}
//
// Query strings can have duplicate keys and the values are merged into an array,
// however this assumes there's only a single value and ignores any extras.
func parseQueryWithSingleValues(data string) (map[string]string, error) {
	initialParams, err := url.ParseQuery(data)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for k, v := range initialParams {
		if len(v) > 0 {
			result[k] = v[0]
		}
	}
	return result, nil
}
