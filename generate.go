package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/Khan/genqlient/generate"
	"github.com/suessflorian/gqlfetch"
)

//go:generate go run generate.go
func main() {
	const schemaFile = "schema.graphql"

	if _, err := os.Stat(schemaFile); errors.Is(err, os.ErrNotExist) {
		var headers http.Header = http.Header{
			"Authorization": []string{fmt.Sprintf("Bearer %s", os.Getenv("BUILDKITE_GRAPHQL_TOKEN"))},
		}

		fmt.Printf("Generating new schema file at %s\n", schemaFile)
		schema, err := gqlfetch.BuildClientSchemaWithHeaders(context.Background(), "https://graphql.buildkite.com/v1", headers, false)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		if err = os.WriteFile(schemaFile, []byte(schema), 0644); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		fmt.Printf("Schema written to %s\n", schemaFile)
	}

	fmt.Println("Generating GraphQL code")
	generate.Main()
}
