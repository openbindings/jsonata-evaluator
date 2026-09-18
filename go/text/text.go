// Package text evaluates compiled JSONata expressions over JSON text. It is a
// convenience over package jsonata for callers that already hold JSON bytes.
// It adds nothing the native API lacks and is not the evaluation boundary.
package text

import (
	"context"

	"github.com/openbindings/jsonata-evaluator/go/jsonata"
)

// Decode decodes JSON text into the values package jsonata admits: numbers
// as json.Number, objects as *jsonata.Object preserving member order, arrays
// as []any. A text that is not one complete JSON value, or that contains a
// duplicate member name within an object, is an error.
func Decode(data []byte) (any, error) { panic("unimplemented") }

// Evaluate decodes inputJSON with Decode, evaluates expr over it, and encodes
// the result with encoding/json, which renders *jsonata.Object in member
// order, json.Number as its token, and []byte with base64.StdEncoding. An
// absent result is present == false with no bytes and a nil error.
func Evaluate(ctx context.Context, expr *jsonata.Expression, inputJSON []byte, opts ...jsonata.EvalOption) (out []byte, present bool, err error) {
	panic("unimplemented")
}
