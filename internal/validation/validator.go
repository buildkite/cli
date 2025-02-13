package validation

import (
	"fmt"
	"regexp"
	"strings"
)

type Validator struct {
	rules map[string][]Rule
}

func New() *Validator {
	return &Validator{
		rules: make(map[string][]Rule),
	}
}

func (v *Validator) AddRule(field string, rule Rule) {
	v.rules[field] = append(v.rules[field], rule)
}

// Validate validates a map of field/value pairs
func (v *Validator) Validate(fields map[string]interface{}) error {
	var errors ValidationErrors

	for field, value := range fields {
		if rules, ok := v.rules[field]; ok {
			for _, rule := range rules {
				if err := rule.Validate(value); err != nil {
					errors = append(errors, ValidationError{
						Field:   field,
						Message: err.Error(),
					})
				}
			}
		}
	}

	if len(errors) > 0 {
		return errors
	}
	return nil
}

type RequiredRule struct{}

func (r RequiredRule) Validate(value interface{}) error {
	if value == nil {
		return fmt.Errorf("field is required")
	}
	if s, ok := value.(string); ok && strings.TrimSpace(s) == "" {
		return fmt.Errorf("field is required")
	}
	return nil
}

type SlugRule struct{}

func (r SlugRule) Validate(value interface{}) error {
	s, ok := value.(string)
	if !ok {
		return fmt.Errorf("value must be a string")
	}

	matched, _ := regexp.MatchString(`^[a-z0-9]+(?:-[a-z0-9]+)*$`, s)
	if !matched {
		return fmt.Errorf("must be a valid slug (lowercase letters, numbers, and hyphens)")
	}
	return nil
}

type UUIDRule struct{}

func (r UUIDRule) Validate(value interface{}) error {
	s, ok := value.(string)
	if !ok {
		return fmt.Errorf("value must be a string")
	}

	matched, _ := regexp.MatchString(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, strings.ToLower(s))
	if !matched {
		return fmt.Errorf("must be a valid UUID")
	}
	return nil
}

type MinValueRule struct {
	min int
}

func (r MinValueRule) Validate(value interface{}) error {
	num, ok := value.(int)
	if !ok {
		return fmt.Errorf("value must be an integer")
	}

	if num < r.min {
		return fmt.Errorf("value must be at least %d", r.min)
	}
	return nil
}

// Common rules that can be reused
var (
	Required = RequiredRule{}
	Slug     = SlugRule{}
	UUID     = UUIDRule{}
	MinValue = func(min int) MinValueRule {
		return MinValueRule{min: min}
	}
)
