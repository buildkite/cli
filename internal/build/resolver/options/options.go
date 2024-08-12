package options

import (
	"context"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v3/buildkite"
	"github.com/go-git/go-git/v5"
)

// OptionsResolverFn is a function for adding a branch filter to REST API requests
type OptionsResolverFn func(context.Context) OptionsFn

// OptionsFn is a function to apply modifications to the list builds API request ie. for adding additional filters
type OptionsFn func(*buildkite.BuildsListOptions) error

type AggregateResolver []OptionsResolverFn

func (ar AggregateResolver) WithResolverWhen(condition bool, resovler OptionsResolverFn) AggregateResolver {
	if condition {
		return append(ar, resovler)
	}
	return ar
}

// ResolveBranchFromFlag returns a function that is used to add a branch filter to a build list options
func ResolveBranchFromFlag(branch string) OptionsResolverFn {
	return func(context.Context) OptionsFn {
		return func(options *buildkite.BuildsListOptions) error {
			if branch != "" && len(options.Branch) == 0 {
				options.Branch = append(options.Branch, branch)
			}
			return nil
		}
	}
}

// ResolveBranchFromRepository returns a function that is used to add a branch filter to a build list options
func ResolveBranchFromRepository(repo *git.Repository) OptionsResolverFn {
	return func(context.Context) OptionsFn {
		return func(options *buildkite.BuildsListOptions) error {
			var branch string
			if repo != nil && len(options.Branch) == 0 {
				head, err := repo.Head()
				if err != nil {
					return err
				}
				branch = head.Name().Short()
				options.Branch = append(options.Branch, branch)
			}
			return nil
		}
	}
}

// ResolveUserFromFlag returns a function that is used to add a user filter to a build list options
func ResolveUserFromFlag(user string) OptionsResolverFn {
	return func(context.Context) OptionsFn {
		return func(options *buildkite.BuildsListOptions) error {
			// set the user filter if the given user exists and a filter is not already set
			if user != "" && options.Creator == "" {
				options.Creator = user
			}
			return nil
		}
	}
}

// ResolveCurrentUser returns a function that is used to add a user filter to a build list options
func ResolveCurrentUser(f *factory.Factory) OptionsResolverFn {
	return func(context.Context) OptionsFn {
		return func(options *buildkite.BuildsListOptions) error {
			// if creator filter already applied, dont apply another
			if options.Creator != "" {
				return nil
			}
			user, _, err := f.RestAPIClient.User.Get()
			if err != nil {
				return err
			}
			// set the user filter if the given user exists and a filter is not already set
			if user != nil {
				options.Creator = *user.ID
			}
			return nil
		}
	}
}
