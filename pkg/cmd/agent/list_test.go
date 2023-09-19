package agent

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/buildkite/cli/v3/internal/printer"
)

// This test essentially covers the `view` command too; it uses the same PrintOutput function.
func TestListAgent(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		Foo string `json:"foo"`
	}{
		{
			Foo: "bar",
		},
	}

	for _, testcase := range testcases {
		test := testcase
		fmt.Printf("Testcase: %+v\n", testcase)
		t.Run("checks the struct prints", func(t *testing.T) {
			t.Parallel()

			got, err := printer.PrintOutput(printer.JSON, test)
			if err != nil {
				t.Fatalf("Failed to print output: %v", err)
			}
			want := `{
    "foo": "bar"
}`

			var gotObj, wantObj interface{}
			if err := json.Unmarshal([]byte(got), &gotObj); err != nil {
				t.Fatalf("Failed to unmarshal 'got': %v", err)
			}
			if err := json.Unmarshal([]byte(want), &wantObj); err != nil {
				t.Fatalf("Failed to unmarshal 'want': %v", err)
			}

			if !reflect.DeepEqual(gotObj, wantObj) {
				t.Errorf("got %s, want %s", got, want)
			}
		})
	}
}
