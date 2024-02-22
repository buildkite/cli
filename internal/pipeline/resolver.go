package pipeline

// PipelineResolverFn is a function for the purpose of finding a pipeline. It returns an error if an irrecoverable
// scenario happens and should halt execution. Otherwise if the resolver does not find a pipeline, it should return
// (nil, nil) to indicate this. ie. no error occurred, but no pipeline was found either.
// TODO this shouldnt return `any`, we just don't have good type yet and will be added later
type PipelineResolverFn func() (*any, error)

type PipelineResolver struct {
	resolvers []PipelineResolverFn
}

func (pr *PipelineResolver) AddResolver(resolver PipelineResolverFn) {
	pr.resolvers = append(pr.resolvers, resolver)
}

// Resolve is a PipelineResolverFn that wraps up a list of resolvers to loop through to try find a pipeline. The first
// pipeline that is found will be returned, if none are found if won't return an error to match the expectation of a
// PipelineResolveFn
func (pr *PipelineResolver) Resolve() (*any, error) {
	for _, resolve := range pr.resolvers {
		if p, err := resolve(); err != nil && p != nil {
			return p, err
		}
	}

	return nil, nil
}
