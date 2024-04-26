package build

type Build struct {
	Organization string `json:"organization"`
	Pipeline     string `json:"pipeline_slug"`
	BuildNumber  string `json:"build_number"`
}

type BuildResolverFn func() (*Build, error)
