package validate

import (
	"fmt"
	"regexp"
)

// namePattern allows lowercase alphanumeric and hyphens, must start with alphanumeric.
var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Name validates a resource name.
// Rules: lowercase alphanumeric and hyphens, must start with [a-z0-9], max 63 chars.
func Name(name string) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if len(name) > 63 {
		return fmt.Errorf("name must be at most 63 characters")
	}
	if !namePattern.MatchString(name) {
		return fmt.Errorf("name must consist of lowercase alphanumeric characters or hyphens, and must start with an alphanumeric character")
	}
	return nil
}
