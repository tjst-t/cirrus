package network

import "errors"

var (
	ErrNotFound      = errors.New("network resource not found")
	ErrConflict      = errors.New("network resource already exists")
	ErrInvalidState  = errors.New("invalid network resource state")
	ErrHasDependents = errors.New("resource has dependent resources")
	ErrCIDRExhausted = errors.New("network CIDR address space exhausted")
)
