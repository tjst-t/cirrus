package storage

import "errors"

var (
	ErrBackendNotFound    = errors.New("storage backend not found")
	ErrVolumeTypeNotFound = errors.New("volume type not found")
	ErrVolumeNotFound     = errors.New("volume not found")
	ErrVolumeInUse        = errors.New("volume is in use")
	ErrNoMatchingBackend  = errors.New("no backend matches volume type requirements")
	ErrBackendNotActive   = errors.New("storage backend is not active")
	ErrVolumeName         = errors.New("volume name already exists in tenant")
)
