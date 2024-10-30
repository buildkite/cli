package util

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/pkg/browser"
)

func GenerateGraphQLID(prefix, uuid string) string {
	var graphqlID strings.Builder
	wr := base64.NewEncoder(base64.StdEncoding, &graphqlID)
	fmt.Fprintf(wr, "%s%s", prefix, uuid)
	wr.Close()

	return graphqlID.String()
}

func OpenInWebBrowser(openInWeb bool, webUrl string) error {
	if openInWeb {
		err := browser.OpenURL(webUrl)
		if err != nil {
			fmt.Println("Error opening browser: ", err)
			return err
		}
	}
	return nil
}
