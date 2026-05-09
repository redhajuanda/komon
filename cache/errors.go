package cache

import "errors"

var (
	// ErrNotFound is returned when a key is not found in the cache.
	ErrNotFound = errors.New("not found")
	// ErrInvalidSliceDestination means MGet/HGetAll dest is not *([]T or []*T).
	ErrInvalidSliceDestination = errors.New("cache: dest must be a non-nil pointer to slice ([]T or []*T)")
)
