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
