package jsonata

import "encoding/base64"

// Compile-time proof of the claim in WithBytesEncoding's doc comment.
var (
	_ BytesEncoding = base64.StdEncoding
	_ BytesEncoding = base64.URLEncoding
	_ BytesEncoding = base64.RawStdEncoding
	_ BytesEncoding = base64.RawURLEncoding
)
