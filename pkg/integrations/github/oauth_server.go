package github

import (
	"fmt"
	"net"
	"net/http"

	githubclient "github.com/google/go-github/github"
	githuboauth "golang.org/x/oauth2/github"

	"github.com/skratchdot/open-golang/open"
	"golang.org/x/oauth2"
)

var (
	ServerBind    = "127.0.0.1:7024"
	ServerAddress = "http://127.0.0.1:7024"
)

var (
	oauthConf = oauth2.Config{
		ClientID:     "d5a2938d576279fbc995",
		ClientSecret: "aadcab62248629c3d0018b9e54e4d3fc37fdd29e",
		Scopes:       []string{"user:email", "repo"},
		Endpoint:     githuboauth.Endpoint,
	}
)

func NewClientFromToken(token *oauth2.Token) *githubclient.Client {
	oauthClient := oauthConf.Client(oauth2.NoContext, token)
	return githubclient.NewClient(oauthClient)
}

func Authenticate() (*oauth2.Token, error) {
	ln, err := net.Listen("tcp", ServerBind)
	if err != nil {
		return nil, err
	}

	handler := githubAuthHandler{
		OAuthStateString: "da39a3ee5e6b4b0d3255bfef95601890afd80709",
		TokenCh:          make(chan *oauth2.Token),
	}

	go func() {
		_ = http.Serve(ln, handler)
	}()

	if err := open.Run(ServerAddress); err != nil {
		return nil, err
	}

	select {
	case token := <-handler.TokenCh:
		return token, ln.Close()
	case err := <-handler.ErrCh:
		return nil, err
	}
}

type githubAuthHandler struct {
	OAuthConf        *oauth2.Config
	OAuthStateString string
	TokenCh          chan *oauth2.Token
	ErrCh            chan error
}

func (handler githubAuthHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.URL.Path {
	case `/`:
		http.Redirect(w, req, "/login", http.StatusMovedPermanently)
	case `/login`:
		url := oauthConf.AuthCodeURL(handler.OAuthStateString, oauth2.AccessTypeOnline)
		http.Redirect(w, req, url, http.StatusTemporaryRedirect)
	case `/github_oauth_cb`:
		// check the oauth state string matches, prevent replay attacks
		if state := req.FormValue("state"); state != handler.OAuthStateString {
			err := fmt.Errorf("Invalid oauth state, expected %q, got %q", handler.OAuthStateString, state)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			handler.ErrCh <- err
			return
		}
		token, err := oauthConf.Exchange(oauth2.NoContext, req.FormValue("code"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			handler.ErrCh <- err
			return
		}
		handler.TokenCh <- token
		fmt.Fprintf(w, "Successfully authenticated! You can close this tab and return to cli now")
	default:
		http.Error(w, "Page not found", http.StatusNotFound)
	}
}
