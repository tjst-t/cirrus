package topology

import "errors"

var (
	ErrNotFound         = errors.New("not found")
	ErrConflict         = errors.New("already exists")
	ErrInvalidParent    = errors.New("invalid parent location")
	ErrInvalidType      = errors.New("invalid location type")
)
