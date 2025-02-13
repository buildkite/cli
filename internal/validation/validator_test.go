package validation

import (
	"strings"
	"testing"
)

func TestRequiredRule(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input   interface{}
		wantErr bool
		errMsg  string
	}{
		"nil value": {
			input:   nil,
			wantErr: true,
			errMsg:  "field is required",
		},
		"empty string": {
			input:   "",
			wantErr: true,
			errMsg:  "field is required",
		},
		"whitespace string": {
			input:   "   ",
			wantErr: true,
			errMsg:  "field is required",
		},
		"valid string": {
			input:   "test",
			wantErr: false,
		},
		"valid number": {
			input:   42,
			wantErr: false,
		},
	}

	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := Required.Validate(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if !strings.Contains(err.Error(), tc.errMsg) {
					t.Errorf("expected error containing %q, got %q", tc.errMsg, err.Error())
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}

	t.Run("demonstrating multiple errors per field", func(t *testing.T) {
		v := New()
		v.AddRule("name", Required)
		v.AddRule("name", Slug)

		err := v.Validate(map[string]interface{}{
			"name": "",
		})

		if err == nil {
			t.Error("expected validation errors but got none")
		}

		if validationErrs, ok := err.(ValidationErrors); ok {
			if len(validationErrs) != 2 {
				t.Errorf("expected 2 validation errors for empty field (Required + Slug), got %d", len(validationErrs))
			}
		}
	})
}

func TestSlugRule(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input   interface{}
		wantErr bool
		errMsg  string
	}{
		"valid slug": {
			input:   "my-slug-123",
			wantErr: false,
		},
		"invalid characters": {
			input:   "My Slug!",
			wantErr: true,
			errMsg:  "must be a valid slug",
		},
		"consecutive hyphens": {
			input:   "my--slug",
			wantErr: true,
			errMsg:  "must be a valid slug",
		},
		"non-string input": {
			input:   123,
			wantErr: true,
			errMsg:  "value must be a string",
		},
	}

	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := Slug.Validate(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if !strings.Contains(err.Error(), tc.errMsg) {
					t.Errorf("expected error containing %q, got %q", tc.errMsg, err.Error())
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestMinValueRule(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input   interface{}
		min     int
		wantErr bool
		errMsg  string
	}{
		"valid value above minimum": {
			input:   5,
			min:     1,
			wantErr: false,
		},
		"value equal to minimum": {
			input:   1,
			min:     1,
			wantErr: false,
		},
		"value below minimum": {
			input:   0,
			min:     1,
			wantErr: true,
			errMsg:  "value must be at least 1",
		},
		"non-integer input": {
			input:   "not a number",
			min:     1,
			wantErr: true,
			errMsg:  "value must be an integer",
		},
	}

	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rule := MinValue(tc.min)
			err := rule.Validate(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if !strings.Contains(err.Error(), tc.errMsg) {
					t.Errorf("expected error containing %q, got %q", tc.errMsg, err.Error())
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidator(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		fields   map[string]interface{}
		rules    map[string][]Rule
		wantErr  bool
		errCount int
	}{
		"single valid field": {
			fields: map[string]interface{}{
				"name": "test-slug",
			},
			rules: map[string][]Rule{
				"name": {Required, Slug},
			},
			wantErr: false,
		},
		"multiple valid fields": {
			fields: map[string]interface{}{
				"name":    "test-slug",
				"count":   5,
				"enabled": true,
			},
			rules: map[string][]Rule{
				"name":  {Required, Slug},
				"count": {MinValue(1)},
			},
			wantErr: false,
		},
		"single invalid field": {
			fields: map[string]interface{}{
				"count": 0,
			},
			rules: map[string][]Rule{
				"count": {MinValue(1)},
			},
			wantErr:  true,
			errCount: 1,
		},
		"multiple failures": {
			fields: map[string]interface{}{
				"name":  "",
				"count": 0,
			},
			rules: map[string][]Rule{
				"name":  {Required, Slug},
				"count": {MinValue(1)},
			},
			wantErr:  true,
			errCount: 3,
		},
	}

	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			v := New()

			// Add rules to validator
			for field, rules := range tc.rules {
				for _, rule := range rules {
					v.AddRule(field, rule)
				}
			}

			err := v.Validate(tc.fields)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				if validationErrs, ok := err.(ValidationErrors); ok {
					if len(validationErrs) != tc.errCount {
						t.Errorf("expected %d validation errors, got %d", tc.errCount, len(validationErrs))
					}
				} else {
					t.Error("expected ValidationErrors type")
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestUUIDRule(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input   interface{}
		wantErr bool
		errMsg  string
	}{
		"valid UUID": {
			input:   "123e4567-e89b-12d3-a456-426614174000",
			wantErr: false,
		},
		"invalid format": {
			input:   "not-a-uuid",
			wantErr: true,
			errMsg:  "must be a valid UUID",
		},
		"wrong length": {
			input:   "123e4567-e89b-12d3-a456",
			wantErr: true,
			errMsg:  "must be a valid UUID",
		},
		"non-string input": {
			input:   123,
			wantErr: true,
			errMsg:  "value must be a string",
		},
	}

	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := UUID.Validate(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if !strings.Contains(err.Error(), tc.errMsg) {
					t.Errorf("expected error containing %q, got %q", tc.errMsg, err.Error())
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
