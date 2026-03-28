package az

import "errors"

var (
	ErrNotFound = errors.New("availability zone not found")
	ErrConflict = errors.New("availability zone already exists")
)
