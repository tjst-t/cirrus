package api

import (
	"fmt"

	"github.com/tjst-t/cirrus/internal/validate"
)

const maxDescriptionLength = 256

// validateName delegates to the shared validate.Name rule:
// lowercase alphanumeric and hyphens, must start with [a-z0-9], max 63 chars.
func validateName(name string) error {
	return validate.Name(name)
}

// validateDescription checks optional description length.
func validateDescription(description string) error {
	if len(description) > maxDescriptionLength {
		return fmt.Errorf("description must be at most %d characters", maxDescriptionLength)
	}
	return nil
}
