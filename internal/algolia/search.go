package algolia

import (
	"github.com/algolia/algoliasearch-client-go/v3/algolia/search"
)

const indexName = "prod_docs"

var (
	index *search.Index
)

func init() {
	client := search.NewClient(appID, searchKey)
	index = client.InitIndex(indexName)
}

// Search will search the algolia docs index for the given query and return result URLs
func Search(query string) ([]string, error) {
	res, err := index.Search(query)
	if err != nil {
		return nil, err
	}

	var results []string
	for i, v := range res.Hits {
		// finish early if we find more than 3 results
		if i >= 3 {
			break
		}
		if hit, ok := v["url"].(string); ok {
			results = append(results, hit)
		}
	}

	return results, nil
}
