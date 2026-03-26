package host

import "errors"

var (
	ErrNotFound     = errors.New("host not found")
	ErrConflict     = errors.New("host already exists")
	ErrInvalidState = errors.New("invalid operational state")
)
