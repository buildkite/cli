package validation

import "github.com/google/uuid"

// UUID validates that the given string is a valid UUID.
func ValidateUUID(s string) error {
	_, err := uuid.Parse(s)
	if err != nil {
		return err
	}
	return nil
}
