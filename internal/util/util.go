package util

import (
	"encoding/base64"
	"fmt"
	"strings"
)

func GenerateGraphQLID(prefix, uuid string) string {
	var graphqlID strings.Builder
	wr := base64.NewEncoder(base64.StdEncoding, &graphqlID)
	fmt.Fprintf(wr, "%s%s", prefix, uuid)
	wr.Close()

	return graphqlID.String()
}
