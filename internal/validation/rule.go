package validation

type Rule interface {
	Validate(value interface{}) error
}
