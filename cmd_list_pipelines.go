package cli

import (
	"fmt"

	"github.com/sahilm/fuzzy"
)

type ListPipelinesCommandContext struct {
	TerminalContext
	KeyringContext

	Debug     bool
	DebugHTTP bool

	Fuzzy   string
	Limit   int
	ShowURL bool
}

func ListPipelinesCommand(ctx ListPipelinesCommandContext) error {
	bk, err := ctx.BuildkiteGraphQLClient()
	if err != nil {
		return NewExitError(err, 1)
	}

	pipelines, err := listPipelines(bk)
	if err != nil {
		return NewExitError(err, 1)
	}

	var counter int
	var pipelineStrings []string

	for _, pipeline := range pipelines {
		pipelineStrings = append(pipelineStrings,
			fmt.Sprintf("%s/%s", pipeline.Org, pipeline.Slug))
	}

	if ctx.Fuzzy != "" {
		const bold = "\033[1m%s\033[0m"
		matches := fuzzy.Find(ctx.Fuzzy, pipelineStrings)

		for _, match := range matches {
			counter++
			if ctx.Limit > 0 && counter > ctx.Limit {
				break
			}

			if ctx.ShowURL {
				ctx.Printf(`https://buildkite.com/`)
			}
			for i := 0; i < len(match.Str); i++ {
				if contains(i, match.MatchedIndexes) {
					fmt.Print(fmt.Sprintf(bold, string(match.Str[i])))
				} else {
					fmt.Print(string(match.Str[i]))
				}

			}
			fmt.Println()
		}

		return nil
	}

	for _, p := range pipelineStrings {
		counter++
		if ctx.Limit > 0 && counter > ctx.Limit {
			break
		}
		if ctx.ShowURL {
			ctx.Printf(`https://buildkite.com/`)
		}
		ctx.Println(p)
	}

	return nil
}

func contains(needle int, haystack []int) bool {
	for _, i := range haystack {
		if needle == i {
			return true
		}
	}
	return false
}
