package pipeline

// TODO temporary type for resolvers to return. this should be amended in future work
type Pipeline any

// PipelineResolverFn is a function for the purpose of finding a pipeline. It returns an error if an irrecoverable
// scenario happens and should halt execution. Otherwise if the resolver does not find a pipeline, it should return
// (nil, nil) to indicate this. ie. no error occurred, but no pipeline was found either.
type PipelineResolverFn func() (Pipeline, error)

type AggregateResolver []PipelineResolverFn

// Resolve is a PipelineResolverFn that wraps up a list of resolvers to loop through to try find a pipeline. The first
// pipeline that is found will be returned, if none are found if won't return an error to match the expectation of a
// PipelineResolveFn
func (pr AggregateResolver) Resolve() (Pipeline, error) {
	for _, resolve := range pr {
		p, err := resolve()
		if err != nil {
			return nil, err
		}
		if p != nil {
			return p, nil
		}
	}

	return nil, nil
}
