package network

import "errors"

var (
	ErrNotFound       = errors.New("network resource not found")
	ErrConflict       = errors.New("network resource already exists")
	ErrInvalidState   = errors.New("invalid network resource state")
	ErrHasDependents  = errors.New("resource has dependent resources")
	ErrInvalidCIDR    = errors.New("invalid CIDR notation")
	ErrInvalidGateway = errors.New("gateway not within subnet CIDR")
	ErrInvalidRange   = errors.New("DHCP range not within subnet CIDR")
)
