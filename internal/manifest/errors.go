package manifest

import "errors"

var (
	ErrNotFound = errors.New("manifest not found")
	ErrInvalid  = errors.New("invalid manifest")
	ErrStale    = errors.New("stale manifest")
)
