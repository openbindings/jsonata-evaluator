package jsonata

import (
	"encoding/base64"
	"errors"
)

// Compile-time proof of the claims in the doc comments.
var (
	_ BytesEncoding = base64.StdEncoding
	_ BytesEncoding = base64.URLEncoding
	_ BytesEncoding = base64.RawStdEncoding
	_ BytesEncoding = base64.RawURLEncoding

	_ error                       = (*Error)(nil)
	_ interface{ Is(error) bool } = (*Error)(nil)
	_                             = errors.Is
)

// The sentinels are not *Error values, so their fields cannot be mutated
// through a type assertion, and (*Error).Unwrap exposes a Resolver's cause.
var (
	_ interface{ Unwrap() error } = (*Error)(nil)
	_ error                       = sentinel("")
)
