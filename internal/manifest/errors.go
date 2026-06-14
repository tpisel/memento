package manifest

import "errors"

var (
	ErrNotFound          = errors.New("manifest not found")
	ErrInvalid           = errors.New("invalid manifest")
	ErrSchemaUnsupported = errors.New("manifest schema unsupported")
	ErrStale             = errors.New("stale manifest")
)
