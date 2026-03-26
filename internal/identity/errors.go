package identity

import "errors"

var (
	ErrNotFound        = errors.New("not found")
	ErrUnauthenticated = errors.New("unauthenticated")
	ErrTokenInvalid    = errors.New("invalid or unknown token")
	ErrForbidden       = errors.New("forbidden")
)
