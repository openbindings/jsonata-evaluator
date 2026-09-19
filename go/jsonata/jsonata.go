// Package jsonata evaluates JSONata 2.1 expressions over Go values.
//
// The JSONata documentation pinned by DocumentationCommit defines what an
// expression does. This package's value model, built from Go's own types,
// defines what values are and how they combine, the way regexp implements
// regular-expression notation over Go strings rather than porting another
// engine. Integers are exact up to Limits.MaxIntegerBits, decimals are
// float64, and an operand is never converted inexactly: a value that cannot
// be represented exactly is an error, never an approximation. No expression,
// whatever its content, can panic the calling goroutine, recurse without
// bound, or allocate without bound. A fatal runtime error (a goroutine
// stack overflow, concurrent map access, memory exhaustion) is not a panic
// and cannot be recovered; this package prevents the first and third by
// bounding depth and bytes, and the second only while the caller does not
// modify a value the evaluation is reading. A panic inside a caller's
// Resolver, view, BytesEncoding, or Now propagates. The design is written
// up at https://github.com/openbindings/jsonata-evaluator.
//
// Status: design. Every function in this package is a signature and a doc
// comment; none is implemented.
//
// Compile once, evaluate many:
//
//	expr, err := jsonata.Compile(`{ "id": user_id, "name": display_name }`)
//	if err != nil { ... }
//	in, err := jsonata.Unmarshal(body)   // keeps 9007199254740993 exact
//	if err != nil { ... }
//	out, present, err := expr.Eval(ctx, in, nil)
//	switch {
//	case err != nil:   // an *Error, or ctx.Err()
//	case !present:     // the result is absent (JSONata undefined)
//	case out == nil:   // the result is null
//	}
//	b, err := jsonata.Marshal(out, nil)
//
// Every package-level function runs under DefaultLimits; the same function
// as a method of a Limits value runs under those bounds, as net.Dial and
// net.Dialer.DialContext relate.
//
// # Getting values in
//
// Inputs are Go values, not JSON text. A caller holding JSON bytes decodes
// them with Unmarshal, which keeps every integer exact; decoding with
// encoding/json into any turns 9007199254740993 into the float64
// 9007199254740992 before this package sees it, and the package cannot tell
// that a rounded integral float64 was ever anything else. A json.Decoder
// with UseNumber set is also fine.
//
// These are the admitted values: nil, bool, string, []byte, json.Number,
// json.RawMessage, int, int8, int16, int32, int64, uint, uint8, uint16,
// uint32, uint64, float32, float64, *big.Int, *Object, any value implementing
// ObjectView or ArrayView, and any []T or map[string]T whose T is admitted
// (so []any, map[string]any, []string, map[string]int64, and so on), and any
// named type whose underlying type is one of those. Admission is by exact
// type, not structure: []any{v} with a foreign v is an admitted array with a
// foreign element, while []MyStruct, [16]byte, time.Time, and any type that
// merely implements json.Marshaler are foreign as a whole. Classification is
// by exact type first: []byte, which is []uint8, is bytes and never an array
// of numbers; a json.RawMessage is decoded as JSON on each read, charged per
// byte to MaxWork and bounded as Unmarshal is, so two reads of one
// RawMessage yield two distinct values (a caller that reads one more than
// once decodes it with Unmarshal first), and a nil or empty one is malformed
// (CodeMalformedInput); any other named type whose underlying type is
// []byte is bytes. A nil []byte is the empty bytes. A nil []T is the empty
// array, a nil map[string]T and a nil *Object are the empty object, because
// Go code routinely leaves a collection unset and $count(items) = 0 should
// hold for it rather than items being null; every other nil pointer,
// including a nil *big.Int, is null. A string that is not valid UTF-8 is an
// error (CodeUnsupportedValue) when observed. A view is read through its
// methods wherever it appears: as the input, in a binding, or from a
// Resolver; a view is never passed to a Resolver.
//
// Anything else is foreign. The expression can read a foreign value only
// through the Resolver in Env; without one it is an error
// (CodeUnsupportedValue). A foreign value nested inside an admitted
// container, or bound in Env.Bindings, is resolved when read like any other.
// Admission is per value, on first read: a value the expression never reads
// is never checked, and Prepare checks nothing but the root. Carriage is not
// a read: a value the expression only selects, copies, or rearranges is
// neither classified nor validated, so an invalid string or a foreign value
// can pass through unread; only an operation that observes a value's content
// classifies it. Values are read by reference and never copied on admission.
//
// NoInput is the input for evaluating with no input at all: $ is then
// absent (undefined), as the reference implementation's evaluate() with no
// argument behaves, which differs from a nil input, where $ is null.
//
// # Getting values out
//
// The language distinguishes an absent result (undefined) from the value
// null. Eval, Select, and Complete return (value, present, err): present is
// false and value is nil for an absent result, a present nil is null, and on
// a non-nil error both are their zero values. Callers must inspect present;
// v, _, err := expr.Eval(...) silently turns absent into null.
//
// A value the expression only selects, copies, or rearranges is carried: it
// is returned as the same Go value, same type, same identity. Only carriage
// preserves a value's representation, and only order-observing operations
// observe an object's member order, which is part of its representation and
// not of its value. So $ over a map[string]any input returns that map, while
// { "a": $.a } returns a new *Object. The documentation's sequence rules
// apply to the returned value: a sequence of one value is that value, a
// sequence of more is a []any, an empty sequence is absent, and an array the
// expression constructs, or keeps with the [] suffix, is a []any however
// many elements it has. The transform operator's copy is a new tree of
// *Object and []any whose scalars are carried. A caller that must handle
// "an object" uses Member, which reads either, writes a two-arm switch over
// map[string]any and *Object, or encodes the result with Marshal, which
// handles both; Int64 and Float64 recover a number from any admitted
// representation. A foreign value that was carried is returned as the
// original foreign value, never as the view the Resolver produced;
// Materialize converts a result that still contains foreign values, and
// EvalMarshal resolves and encodes in one pass. A result that is or contains
// a function value cannot leave an evaluation and is an error
// (CodeUnsupportedValue). A result may alias the input and Env.Bindings by
// identity; Clone makes an owned copy. The serialized size of a result is
// not bounded by the evaluation's node count: a result may reference one
// carried value up to MaxOutputNodes times, so Marshal is bounded on its
// own.
//
// # Errors
//
// A failure is an *Error, matched with errors.As against *Error, or with
// errors.Is against a sentinel such as ErrBudget:
//
//	var e *jsonata.Error
//	if errors.As(err, &e) && e.Code == "D1001" { ... }
//
// Code.Class groups codes by what a caller can do about them. Context
// cancellation and deadlines are not Errors; they are returned as ctx.Err(),
// so errors.Is(err, context.Canceled) holds. Compile fails with an *Error of
// ClassSyntax carrying Offset, Line, Column, and Token, or with CodeBudget
// when the source exceeds MaxExpressionBytes or MaxDepth.
//
// # Numbers
//
// Every admitted numeric representation denotes one numeric value, and every
// operation that observes a number (arithmetic, comparison, ordering,
// membership, sort keys, $type, $string) is a function of that value, never
// of the representation. Only carriage preserves the representation. The
// value function is this: an integer representation denotes its integer; a
// decimal token or literal denotes the nearest float64, fixed when it is
// decoded or first read, and that is where a decimal's precision is fixed;
// a float64 denotes itself. So 9007199254740993.0 = 9007199254740993 is
// false: the left side is the float64 9007199254740992.
//
// Admission widens without loss: int through int64 and uint through uint32
// are int64; uint and uint64 are int64 when they fit and *big.Int otherwise;
// float32 is float64. A float64 that is not finite is an error (D1001). A
// json.Number is classified each time it is read (it is a string and holds
// no cache), charged per byte as work: an integral token within int64 is
// int64, an integral token beyond int64 is *big.Int, a token with a fraction
// or exponent is the nearest float64, a token whose nearest float64 is not
// finite is an error (D1001), and a token that is not a JSON number is an
// error (CodeUnsupportedValue). Unmarshal decodes straight to int64,
// *big.Int, and float64 so that decoded input pays no such cost. A numeric
// literal in an expression follows the same rule: 1 and 9007199254740993 are
// int64, 18446744073709551616 is *big.Int, and 1.0, 1e2, and 0.5 are
// float64. An integer token, literal, json.Number, or $number argument
// whose digit count exceeds what MaxIntegerBits can hold is refused
// (CodeBudget) before it is parsed. $number parses a string by the same
// rule, with the 0x, 0o, and 0b prefixes the documentation lists, so
// $number("9007199254740993") is exact.
//
// A value is an integer when it is integral, whatever its representation
// and whatever produced it: a float64 whose value is integral takes part in
// arithmetic and comparison as that integer (every integral float64 is
// exactly some integer, and none exceeds MaxIntegerBits), so x / 100.0 is
// x / 100, json.Number("1.0") + 9007199254740993 is exact, and 1e23 + 1 is
// the exact integer 99999999999999991611393, because the float64 spelled
// 1e23 is that integer. Exactness is a property of the value, not of its
// provenance: a rounded result that lands on an integer is thereafter an
// exact integer. A float64 that is not integral is a decimal.
//
// Results are int64, float64, or *big.Int. Integer with integer yields an
// exact integer for +, -, *, and an exact division, int64 when the result
// fits and *big.Int otherwise, so the result never depends on which
// representation the operands arrived in; a *big.Int result whose bit
// length exceeds Limits.MaxIntegerBits is an error (CodeBudget). Two
// policies govern the decimal domain, and they are two rather than one. An
// integer operand combined with a decimal operand must convert to float64
// exactly, and is an error (CodeInexact) otherwise, so 9007199254740993 +
// 0.5 fails while 0.1 + 0.2 yields the float64 sum, and x / 1e9 succeeds
// where x * 1e-9 fails for an x beyond 2^53 (1e9 is integral; 1e-9 is a
// decimal): the refusal is where an identifier would lose digits on
// entering the decimal domain. "/" over two integers whose quotient is not
// an integer yields the exact quotient rounded once to float64, so
// 1758000000000000123 / 1000000 yields 1758000000000.0002 rather than
// failing, and 9007199254740993 / 2 yields the float64 4503599627370496,
// which is thereafter an exact integer: the language defines division to
// yield fractions and float64 is what a fraction is in this model. "%" is
// Go's % for integers and math.Mod for floats. Division or remainder by zero
// and a non-finite result are errors (D1001). $sum, $abs, $floor, $ceil,
// $round at precision 0, and $power with a non-negative integer exponent
// stay in the integer domain; $sqrt, $average, and $power otherwise yield
// float64. $sum adds left to right. $sqrt is correctly rounded; $power with
// a non-integer exponent is math.Pow and may differ from another host's in
// the last unit. $formatNumber, $formatInteger, and $formatBase render any
// integer's exact digits and follow the picture-string rules the
// documentation incorporates; $parseInteger yields an exact integer.
// $fromMillis, $toMillis, and $millis take and yield int64 milliseconds, and
// a non-integral argument is truncated toward zero as the reference does.
//
// $round rounds half to even on the binary float64 value, so $round(2.675,
// 2) is 2.67 (2.675 as a float64 is below the midpoint); the reference
// implementation rounds the decimal spelling and yields 2.68. Which value a
// tie is judged on is not settled; DIVERGENCES.md records the question and
// this package's current answer. $string renders an integer, including an
// integral float64, as its decimal digits, and a decimal as JSON.stringify
// does, which the documentation specifies: the shortest digits that
// round-trip, plain notation for magnitudes of 1e-6 and above and exponent
// notation with an unpadded exponent below, -0 as 0, nested numbers
// likewise. So $string(1e6) is "1000000", $string(0.1 + 0.2) is
// "0.30000000000000004", and $string(1e-7) is "1e-7". The reference
// implementation renders floats to 15 significant digits, so $string(0.1 +
// 0.2) is "0.3" there, and renders an integral float64 of 1e21 or more in
// exponent notation, so $string(1e21) is "1e+21" there and
// "1000000000000000000000" here (see DIVERGENCES.md).
//
// Equality, ordering, and membership across representations are by value:
// int64(1), float64(1), json.Number("1"), and json.Number("1.0") are equal
// and sort together, and comparison never rounds through float64 when both
// operands are exact; an exact integer compared with a decimal is compared
// exactly, so 9007199254740993 > 4503599627370496.5 is decided without
// converting the integer.
//
// # Strings, bytes, and regular expressions
//
// A string's characters are Unicode code points, so $length("😀") is 1,
// $substring indexes code points, and $match reports index in code points.
// Ordering of strings is by code point; the reference implementation orders
// by UTF-16 code unit (see DIVERGENCES.md). Case mapping is full,
// locale-independent Unicode case mapping, so $uppercase("straße") is
// "STRASSE". $sort is stable. $replace follows the documentation and the
// reference: in a string replacement $0 is the whole match, $N is the Nth
// captured group, and $$ is a literal dollar; digits naming more groups than
// exist are read as the longest prefix that names a group followed by the
// remaining digits literally, so $12 with one group is group 1 followed by
// "2", and a group number with no such prefix is the empty string. Go's
// ${name} form is not recognized. Regex literals are compiled by Compile, so
// a malformed literal fails there rather than at evaluation; a regex
// operation is charged the subject's length and the program's size as work.
//
// A []byte is a string for every purpose that observes its content:
// $length, $contains, every string function, and ordering. That content is
// its encoding under the Env's BytesEncoding, produced only when an
// expression observes it, so $length of a []byte is the length of its
// encoding; with no BytesEncoding, observing a []byte's content is an error
// (CodeUnsupportedValue), because the class requires that the binding, not
// the evaluator, choose the encoding. $type of a []byte is "string" and
// equality between two []byte values is by their bytes, neither needing an
// encoding; for an injective encoding such as base64 that agrees with
// equality of the encodings. A []byte is ordered by its encoding, as it is
// ordered against strings. $base64encode of a []byte encodes the bytes
// themselves, not their presentation; $base64decode yields a string and is
// an error when the decoded bytes are not valid UTF-8. Carriage never
// encodes.
//
// $string of an object or array renders JSON text as the documentation
// specifies by reference to JSON.stringify: no whitespace, no HTML escaping,
// U+2028 and U+2029 unescaped (encoding/json escapes them; JSON.stringify
// does not), members in the order this package observes them, numbers as
// above, []byte through the BytesEncoding, and a function as the empty
// string. With prettify true the indentation is two spaces.
//
// # Objects and member order
//
// Member order is representation, not value: two objects are equal when
// they have the same members with equal values, regardless of order or of
// map versus *Object, and only the order-observing operations ($keys,
// $each, $spread, $string, $merge, $sift, object construction, the
// transform operator) see it. A map[string]any, or any map[string]T, has no
// member order, so those operations see its keys in sorted byte order,
// which is code point order for valid UTF-8, at the cost of a sort per
// observation; an *Object avoids the sort. An *Object preserves insertion
// order; Unmarshal produces *Object so that decoded JSON keeps document
// order, and a caller who needs insertion order on other input supplies an
// *Object. An ObjectView's Range order is its order for every such
// operation and must not change between calls. The reference implementation
// orders integer-like keys numerically before all others, a JavaScript
// artifact this package does not reproduce (see DIVERGENCES.md).
//
// # Ownership and concurrency
//
// An Expression is immutable and safe for concurrent use. The input and Env
// must not be modified during an Eval, or while an Evaluation is in use,
// which ends when Close returns; a concurrent map modification is a fatal
// runtime error in Go, not a recoverable one. Returned values may alias the
// input and the Env's bindings: a value in Env.Bindings can appear in any
// result by identity, so in a service every value in Env.Bindings is
// immutable for the life of the process, and a result is cloned before it
// is modified. Values returned by an Evaluation's Select or Complete are
// shared with the Evaluation until Close and must not be modified before
// then. A constructed container is new, but its members may be carried. The
// engine starts no goroutines and holds no process-wide state; the Resolver
// and Now are called concurrently only when the caller evaluates
// concurrently, and must tolerate that when the Env is shared.
//
// # Selective evaluation
//
// Eval evaluates the whole expression. Prepare returns an Evaluation from
// which fields of the result are selected individually. Selection is
// available when the result is an object constructor with distinct literal
// keys, or a block whose final expression is one, and no field calls
// $random or $eval; every other standard function is pure for this purpose,
// $now and $millis included because their timestamp is fixed. In a block,
// the statements before the final expression (the prelude) are evaluated
// once, on the first selection rather than in Prepare, their bindings are
// shared by every selection, and a failure among them is reported by every
// selection; this is the way to guard a transform under selection. Under a
// selective plan, a failure in one field's subexpression does not fail
// another field's, so $error or $assert in a sibling field is not a guard;
// under a whole-evaluation plan (any Reason other than ReasonNone) every
// Select evaluates the whole expression once, so a sibling's failure is
// reported by every Select. Whether a sibling's failure is observed
// therefore depends on the plan, which depends on the expression and, for
// ReasonArrayInput, on the input. Complete fails when any field would. When
// Complete would succeed within the budget, Select returns the same field.
// The plan is fixed at Compile except ReasonArrayInput, which Prepare
// determines from the root; Expression.Fields reports the static part and
// Evaluation.Plan the whole.
//
//	ev, err := expr.Prepare(ctx, in, nil)
//	if err != nil { ... }
//	defer ev.Close()
//	id, present, err := ev.Select(ctx, "id")      // only the "id" field runs
//	all, present, err := ev.Complete(ctx)         // reuses "id", computes the rest
//
// After a budget error (CodeBudget) every later call on the Evaluation fails
// the same way, and after an engine defect (CodeInternal) the Evaluation is
// closed; a language error in one field leaves the others selectable; a
// field whose computation ended with ctx.Err() is not memoized and a later
// Select computes it again.
//
// The timestamp that $now and $millis observe is fixed before its first
// observation and at most once per Eval or Prepare, so those functions are
// pure for selection and cannot time other work; $random and $eval are
// opaque to the planner. Env.Now overrides the clock, for tests, and is not
// called when the expression cannot observe the clock.
//
// # Bounds and cancellation
//
// Every bound in Limits has a finite default, the bounds are part of the
// compiled expression so they apply wherever it is evaluated, and exceeding
// one is an error (CodeBudget) raised before the allocation that would
// exceed it (for a buffer that grows, before each growth). One Eval, or one
// Evaluation across all of its selections and Complete, has one budget, and
// $eval compiles and evaluates within it. A unit of work is a bounded-cost
// step: an operation whose cost scales with a value's size (a string scan,
// a comparison, a hash, encoding, decoding, a regex match, deep equality,
// ordering, $string, $merge, a copy) is charged per byte or per value it
// visits, including values that are carried or supplied by the caller, so
// MaxWork bounds processor time up to a constant factor. The bounds count
// work and bytes, not time: cancellation and deadlines come from ctx and are
// checked at least every 1024 units of work, including inside every
// operation whose cost scales with a value's size. Compile takes no ctx; it
// is bounded by MaxExpressionBytes and MaxDepth. A panic inside the engine
// is a defect and is recovered into an Error (CodeInternal) so the process
// survives it; a panic inside a caller's Resolver, view, BytesEncoding, or
// Now propagates to the caller.
//
// # Environment
//
// The evaluation environment is closed except for the clock, $random (the
// top-level math/rand/v2 generator: unpredictably seeded, not
// cryptographic, and not for tokens), and the Resolver and BytesEncoding
// the caller supplies in Env; see Resolver for what that door means. This
// package offers no way to register functions, and Env.Bindings holds
// values only. Every binding is readable by the expression and can appear
// in any result, so a caller binds only what the expression is entitled to
// see. A binding shadows a built-in of the same name, as in the reference
// implementation, so binding "string" makes $string a value and $string(x)
// a T1006; binding "eval" to nil disables $eval the same way, and that
// idiom is supported. A binding whose value is nil is null, not absent;
// there is no way to bind an absent variable, and ?? falls through on
// absent only, so $x ?? "d" with $x bound to nil is null. $eval is available
// as the documentation defines it, sees the bindings and the enclosing
// scope, and its result is carried like any other. Tail calls are
// eliminated as the documentation describes and do not count against
// MaxRecursion. $now, $fromMillis, and $toMillis use UTC unless the
// documented timezone argument (±HHMM) or the parsed text supplies an
// offset; the host's local zone is never consulted.
//
// # Conformance
//
// This package implements JSONata Version as defined by the documentation at
// https://github.com/jsonata-js/jsonata/tree/5d1473277e0022d8580e00f891b12080eb3edd74/website/versioned_docs
// (DocumentationCommit). Where its behavior departs from the reference
// implementation's test suite, or from any other reference behavior this
// package has found, the departure is recorded with its reason in
// DIVERGENCES.md beside this package.
package jsonata

import (
	"context"
	"io"
	"iter"
	"time"
)

// Version is the JSONata language version this package implements, and
// DocumentationCommit is the commit of the jsonata-js repository whose
// versioned documentation defines it, as pinned by the OpenBindings
// specification.
const (
	Version             = "2.1"
	DocumentationCommit = "5d1473277e0022d8580e00f891b12080eb3edd74"
)

// NoInput is the input for evaluating with no input: $ is absent. It is
// meaningful only as the root input; nested anywhere, including in a
// binding, it is foreign.
var NoInput any = noInput{}

type noInput struct{}

// Limits bounds what an expression may cost. The zero Limits means the
// defaults, and a zero field means the default for that field; a negative
// field is treated as zero. The package-level Compile, MustCompile, Eval,
// Unmarshal, Marshal, NewEncoder, Materialize, and Clone run under
// DefaultLimits; the methods of the same names run under the receiver.
// Limits are part of a compiled Expression and are read back with
// Expression.Limits, so a cache of compiled expressions can key on them;
// Limits is comparable and will stay so, and a cache keys on
// Expression.Limits, which has the defaults filled in, rather than on the
// receiver that compiled it. A carried value counts as one node however
// large it is, and its bytes are not charged to MaxBytes; the work of
// visiting it is charged when an operation visits it.
type Limits struct {
	// MaxExpressionBytes bounds the length of an expression in UTF-8 bytes,
	// applied separately to each source passed to $eval. Default 256 KiB.
	MaxExpressionBytes int
	// MaxDepth bounds the nesting depth of a parsed expression, of a JSON
	// text Unmarshal decodes, and of any traversal of a value by an
	// operation, Marshal, Materialize, or Clone, so a cyclic value exceeds it
	// (CodeBudget) rather than exhausting the stack. Default 128.
	MaxDepth int
	// MaxRecursion bounds evaluation depth, including function recursion and
	// $eval; tail calls do not count. Default 1024.
	MaxRecursion int
	// MaxOutputNodes bounds the size of a result, counting every constructed
	// value and each carried value once. Default 1 << 20.
	MaxOutputNodes int
	// MaxWork bounds the work of one evaluation in bounded-cost steps: nodes
	// evaluated, elements materialized, source bytes parsed (including by
	// $eval), bytes decoded from a json.RawMessage or read from a
	// json.Number, calls into the Resolver and BytesEncoding, and, for every
	// operation whose cost scales with a value's size, each byte or value it
	// visits, including carried and caller-supplied values and including
	// intermediates that never reach the result. Default 8 << 20.
	MaxWork int
	// MaxBytes bounds the bytes allocated by one evaluation for strings,
	// []byte, encoded forms, *big.Int, $eval sources, regex programs, Error
	// values, and strings and []byte the engine observes through a view,
	// including intermediates. It is cumulative over the evaluation: every
	// allocation is charged when made and nothing is credited back when it
	// is dropped. Default 64 MiB.
	MaxBytes int
	// MaxIntegerBits bounds the magnitude of every integer the evaluation
	// holds, including one decoded from text, read from a json.Number, or
	// parsed by $number, enforced on a token's digit count before it is
	// parsed. Every integral float64 is within any value at or above 1024.
	// Default 4096.
	MaxIntegerBits int
}

// DefaultLimits returns the defaults, filled in.
func DefaultLimits() Limits { panic("unimplemented") }

// Compile parses and prepares an expression under the receiver's bounds.
// The returned Expression is immutable and safe for concurrent use; callers
// evaluating many inputs against one expression compile it once. Caching
// compiled expressions is the caller's concern; a cache keyed by the source
// and Expression.Limits is sufficient. A failure is an *Error of
// ClassSyntax with its position, or CodeBudget for a source beyond
// MaxExpressionBytes or MaxDepth.
func (l Limits) Compile(expression string) (*Expression, error) { panic("unimplemented") }

// MustCompile is Compile for an expression fixed at build time. It panics on
// error, as regexp.MustCompile does.
func (l Limits) MustCompile(expression string) *Expression { panic("unimplemented") }

// Eval compiles expression under the receiver's bounds and evaluates it
// once; env may be nil. It is a convenience for one-off use; repeated
// evaluation compiles once with Compile.
func (l Limits) Eval(ctx context.Context, expression string, input any, env *Env) (value any, present bool, err error) {
	panic("unimplemented")
}

// Unmarshal decodes JSON text into admitted values under the receiver's
// bounds: integer tokens as int64 or *big.Int, tokens with a fraction or
// exponent as float64, strings as string, objects as *Object in member
// order, arrays as []any. Surrounding whitespace is allowed and a top-level
// scalar is a complete value. A text that is not one complete JSON value,
// is not valid UTF-8, contains a duplicate member name within an object, or
// contains an escape for an unpaired surrogate is an error
// (CodeMalformedInput) whose Offset is into data and whose Token is empty.
// Nesting beyond MaxDepth, a decoded size beyond MaxBytes, or an integer
// token beyond MaxIntegerBits is CodeBudget. Unmarshal copies data once;
// strings in the result may share that copy, so a small string can keep the
// whole decoded text alive.
func (l Limits) Unmarshal(data []byte) (any, error) { panic("unimplemented") }

// Marshal encodes a result as JSON text under the receiver's MaxBytes and
// MaxDepth, with no trailing newline: *Object in member order, a map in
// sorted key order, any admitted []T or map[string]T likewise, json.Number
// as its token, *big.Int as digits, float64 as $string renders it, a
// float32 as its float64 widening (so a carried float32(0.1) is
// 0.10000000149011612, as $string also renders it), a nil slice as [] and a
// nil map or *Object as {}, []byte through env's BytesEncoding, no HTML
// escaping, and U+2028 and U+2029 unescaped. A foreign value in v, a
// string that is not valid UTF-8, or a []byte when env has no
// BytesEncoding, is an error (CodeUnsupportedValue); Materialize resolves
// foreign values first, or EvalMarshal does both in one pass. env may be
// nil.
func (l Limits) Marshal(v any, env *Env) ([]byte, error) { panic("unimplemented") }

// NewEncoder returns an Encoder that writes results to w as Marshal would
// under the receiver's bounds. An Encoder may be used for any number of
// Encode calls.
func (l Limits) NewEncoder(w io.Writer, env *Env) *Encoder { panic("unimplemented") }

// Materialize returns v with every foreign value it contains resolved
// through env's Resolver into admitted values, recursively, so the result
// contains no value that would need a Resolver to read: an ObjectView
// becomes an *Object and an ArrayView a []any. It is for callers handing a
// result to typed code. A value containing no foreign value is returned by
// identity; an admitted container that contains one is returned as a new
// container of the same kind with the resolved members. The walk is bounded
// by the receiver as an evaluation would be, on a budget of its own.
func (l Limits) Materialize(ctx context.Context, v any, env *Env) (any, error) {
	panic("unimplemented")
}

// Clone returns a deep copy of v that the caller owns: every object as a
// new *Object (in the order Marshal would observe), every array as a new
// []any, strings and []byte copied, numbers carried, foreign values
// resolved through env as Materialize would. The walk is bounded by the
// receiver. It is for a result that must be modified when it may alias the
// input or Env.Bindings.
func (l Limits) Clone(ctx context.Context, v any, env *Env) (any, error) { panic("unimplemented") }

// Compile is DefaultLimits().Compile.
func Compile(expression string) (*Expression, error) { panic("unimplemented") }

// MustCompile is DefaultLimits().MustCompile.
func MustCompile(expression string) *Expression { panic("unimplemented") }

// Eval is DefaultLimits().Eval.
func Eval(ctx context.Context, expression string, input any, env *Env) (value any, present bool, err error) {
	panic("unimplemented")
}

// Unmarshal is DefaultLimits().Unmarshal.
func Unmarshal(data []byte) (any, error) { panic("unimplemented") }

// Marshal is DefaultLimits().Marshal.
func Marshal(v any, env *Env) ([]byte, error) { panic("unimplemented") }

// NewEncoder is DefaultLimits().NewEncoder.
func NewEncoder(w io.Writer, env *Env) *Encoder { panic("unimplemented") }

// Materialize is DefaultLimits().Materialize.
func Materialize(ctx context.Context, v any, env *Env) (any, error) { panic("unimplemented") }

// Clone is DefaultLimits().Clone.
func Clone(ctx context.Context, v any, env *Env) (any, error) { panic("unimplemented") }

// Env is the value-model side of an evaluation: what variables are bound,
// how foreign values are read, how bytes present as strings, and what time
// it is. A nil *Env and a zero Env are the same: no bindings, no Resolver, no
// BytesEncoding, and the real clock. An Env is read-only during evaluation
// and may be shared by any number of concurrent evaluations; its Resolver
// and Now are then called concurrently. An Env is invalid, and Prepare and
// Eval fail with CodeBinding, only for a binding name the language would
// not accept.
type Env struct {
	// Bindings supplies variables. A name is what the language accepts
	// after "$" (letters, digits, and underscore, not empty), without the
	// "$"; any other name is an error (CodeBinding). Values only; a function
	// cannot be bound. The map is not copied, and its values may appear in
	// results by identity. Every binding is readable by the expression.
	Bindings map[string]any
	// Resolver reads values outside the admitted set. Nil means none.
	Resolver Resolver
	// BytesEncoding presents []byte as a string. Nil means none, and an
	// expression that observes a []byte's content then fails
	// (CodeUnsupportedValue). *base64.Encoding values, including
	// base64.StdEncoding and URLEncoding, satisfy the interface unchanged.
	BytesEncoding BytesEncoding
	// Now supplies the timestamp $now and $millis observe. Nil means
	// time.Now. It is called at most once per Eval or Prepare, and not at
	// all when the expression cannot observe the clock; the instant it
	// returns is rendered in UTC whatever its location. A panic in it
	// propagates.
	Now func() time.Time
}

// Encoder writes results as JSON text.
type Encoder struct{ _ struct{} }

// Encode writes v followed by a newline; Encode(nil) writes null, and an
// absent result is the caller's to represent. On an error partial output
// may have been written; a caller who must not emit a partial value uses
// Marshal and writes the bytes itself.
func (e *Encoder) Encode(v any) error { panic("unimplemented") }

// Member returns the member named key of an object result, whether it is a
// map[string]any, an *Object, or a map[string]T of admitted T (the first two
// by a type switch, the last by reflection). ok is false when v is not an
// object or has no such member.
func Member(v any, key string) (value any, ok bool) { panic("unimplemented") }

// Int64 returns v's value when v is an admitted number whose value is an
// integer that fits in int64: any integer type, a *big.Int in range, an
// integral float64 in range, or a json.Number whose value is such an
// integer. ok is false otherwise; a decimal is never truncated.
func Int64(v any) (n int64, ok bool) { panic("unimplemented") }

// Float64 returns v's value when v is an admitted number that converts to
// float64 exactly, by the rule arithmetic applies to operands. ok is false
// otherwise; an integer is never rounded.
func Float64(v any) (f float64, ok bool) { panic("unimplemented") }

// Expression is a compiled JSONata expression. The zero Expression is not
// usable; Compile returns every Expression.
type Expression struct{ _ struct{} }

// String returns the expression source verbatim, satisfying fmt.Stringer as
// regexp.Regexp does.
func (e *Expression) String() string { panic("unimplemented") }

// Limits returns the bounds the expression was compiled under, with
// defaults filled in.
func (e *Expression) Limits() Limits { panic("unimplemented") }

// Reads reports the input members the expression reads when that is
// statically known. Each path is a sequence of member keys from the root; a
// key applies to every element when the value at that point is an array,
// and a path read inside a predicate is included, so items[price > 10].name
// reads items.price and items.name. When known is true the list is sound:
// the expression reads nothing outside it, so a caller may fetch only those
// members; an expression that reads no member reports an empty, non-nil
// list. Any use of $$ inside a function, a wildcard, a descendant or parent
// operator, $lookup with a computed key, $keys, $each, $spread, or $eval
// makes known false, and paths is then nil. A binding read is not an input
// read and does not affect known. The returned slices are fresh.
func (e *Expression) Reads() (paths [][]string, known bool) { panic("unimplemented") }

// Fields reports the static part of the plan: the selectable top-level
// keys, in constructor order, when the result is an object constructor
// with distinct literal keys (or a block ending in one) and no field calls
// $random or $eval. ok is false otherwise. Prepare may still report
// ReasonArrayInput for an array root. A caller validating configured
// selections checks them here, before any input exists.
func (e *Expression) Fields() (keys []string, ok bool) { panic("unimplemented") }

// Eval evaluates the whole expression over input; env may be nil. A carried
// value (one the expression only selected, copied, or rearranged) is
// returned as the same Go value it was, including a value of a type outside
// the admitted set; Marshal refuses such a result, so a caller with a
// Resolver uses EvalMarshal or Materialize.
func (e *Expression) Eval(ctx context.Context, input any, env *Env) (value any, present bool, err error) {
	panic("unimplemented")
}

// EvalMarshal is Eval followed by one pass that resolves foreign values
// through env and encodes the result as Marshal would, all under the
// expression's Limits; env may be nil. An absent result is out == nil,
// present == false, and a nil error.
func (e *Expression) EvalMarshal(ctx context.Context, input any, env *Env) (out []byte, present bool, err error) {
	panic("unimplemented")
}

// EvalJSON is EvalMarshal over Unmarshal(input), the decode also under the
// expression's Limits; env may be nil.
func (e *Expression) EvalJSON(ctx context.Context, input []byte, env *Env) (out []byte, present bool, err error) {
	panic("unimplemented")
}

// Prepare classifies the root (through the Resolver when it is foreign,
// charged to the budget), completes the plan, and returns an Evaluation; it
// evaluates nothing else, not even a prelude. The input and env must not
// be modified until Close returns. env may be nil. It fails for an invalid
// Env, for a root the Resolver rejects, and for ctx.
func (e *Expression) Prepare(ctx context.Context, input any, env *Env) (*Evaluation, error) {
	panic("unimplemented")
}

// Evaluation is one expression over one input, from which fields of the
// result are selected individually. Selections share work: a field selected
// twice, or reached by two overlapping selections, is computed once, and a
// later Complete reuses every field already selected and returns the same
// values by identity. All selections and Complete draw on one budget. An
// Evaluation is safe for concurrent use: a Select of a field another
// goroutine is computing waits for it, and a waiter whose ctx ends returns
// ctx.Err() while the computation continues for the goroutine that started
// it. Under concurrent selections the budget may be exhausted at a different
// point than under the same selections made in sequence.
type Evaluation struct{ _ struct{} }

// Select returns the field of the result at path, where each element is a
// literal key of an object constructor in the expression. When the plan
// permits, only that field's subexpression is evaluated; otherwise the whole
// expression is evaluated once and the field is looked up, and a result
// that is not an object then yields absent. A path that descends below
// constructor granularity is a lookup by member key within the deepest
// selected field, through a view or the Resolver when the field carried a
// foreign value, charged as work; descending into an array yields absent.
// An empty path is Complete. A key not among the constructor's keys is
// absent, as a field whose subexpression yields no value is;
// Expression.Fields distinguishes the two before evaluation. An object
// constructor with duplicate literal keys fails as Complete would (D1009).
// Select is not a guard; a caller for whom the transform's failure is
// meaningful uses Complete or Eval.
func (v *Evaluation) Select(ctx context.Context, path ...string) (value any, present bool, err error) {
	panic("unimplemented")
}

// Complete evaluates the whole expression, reusing fields already selected,
// and returns the entire result.
func (v *Evaluation) Complete(ctx context.Context) (value any, present bool, err error) {
	panic("unimplemented")
}

// Plan reports how selections will be served.
func (v *Evaluation) Plan() Plan { panic("unimplemented") }

// Close ends the Evaluation. It blocks until every Select and Complete in
// flight has returned; when it returns, further calls return an error
// (CodeClosed), the input and Env may be modified again, and returned
// values are no longer shared with the Evaluation. Returned values may
// still alias the input and Env.Bindings, as any result may, so they are
// the caller's to modify only when it owns those, or after Clone. Close is
// idempotent. An Evaluation that is never closed is reclaimed by the
// garbage collector; Close makes the release prompt and the end of the
// sharing window explicit.
func (v *Evaluation) Close() { panic("unimplemented") }

// Plan describes how an Evaluation serves selections.
type Plan struct {
	// Reason is ReasonNone when fields are evaluated individually, and
	// otherwise explains why every Select evaluates the whole expression once
	// and looks the field up.
	Reason Reason
	_      struct{}
}

// Selective reports whether fields are evaluated individually.
func (p Plan) Selective() bool { panic("unimplemented") }

// Fields returns the selectable top-level keys, in constructor order, when
// the plan is selective, and nil otherwise; it is Expression.Fields with
// the input taken into account.
func (p Plan) Fields() []string { panic("unimplemented") }

// Reason explains why a Plan is not selective.
type Reason uint8

const (
	// ReasonNone: fields are evaluated individually.
	ReasonNone Reason = iota
	// ReasonDynamicShape: the result is not an object constructor with
	// distinct literal keys.
	ReasonDynamicShape
	// ReasonOpaque: a field's subexpression calls $random or $eval, whose
	// result the planner cannot relate to a complete evaluation.
	ReasonOpaque
	// ReasonArrayInput: the input, or the view a foreign root resolves to,
	// is an array, so the constructor groups over it and each field's value
	// is evaluated over the whole input.
	ReasonArrayInput
	// ReasonPlanBudget: the expression exceeded the planner's work bound.
	ReasonPlanBudget
)

// String returns the reason's name: "None", "DynamicShape", "Opaque",
// "ArrayInput", or "PlanBudget".
func (r Reason) String() string { panic("unimplemented") }

// Object is an insertion-ordered object: the type of every object the
// expression constructs, and a valid input where key order matters. The zero
// Object is empty and ready to use; a nil *Object is the empty object to an
// evaluation and reads as empty (Len 0, Get not ok, empty Keys and All),
// and Set on it panics, as a nil map does. Object is safe for concurrent
// reads and not for concurrent modification. Get is O(1); Delete and Map
// are O(n). A key that is not valid UTF-8 is stored as given and refused
// (CodeUnsupportedValue) when an evaluation reads the object.
type Object struct{ _ struct{} }

// NewObject returns an empty Object with room for capacity members.
func NewObject(capacity int) *Object { panic("unimplemented") }

// Len returns the number of members.
func (o *Object) Len() int { panic("unimplemented") }

// Get returns the member named key.
func (o *Object) Get(key string) (value any, ok bool) { panic("unimplemented") }

// Set appends a new member or replaces an existing one in place.
func (o *Object) Set(key string, value any) { panic("unimplemented") }

// Delete removes a member, preserving the order of the rest.
func (o *Object) Delete(key string) { panic("unimplemented") }

// Keys iterates the member names in insertion order.
func (o *Object) Keys() iter.Seq[string] { panic("unimplemented") }

// All iterates the members in insertion order.
func (o *Object) All() iter.Seq2[string, any] { panic("unimplemented") }

// Map returns the members as a new Go map sharing the member values. Order
// is not preserved; nested *Object values are not converted.
func (o *Object) Map() map[string]any { panic("unimplemented") }

// MarshalJSON encodes the object with members in insertion order, as Marshal
// would with a nil Env.
func (o *Object) MarshalJSON() ([]byte, error) { panic("unimplemented") }

// UnmarshalJSON replaces the object's members with those of a JSON object,
// preserving order, with values as Unmarshal decodes them and under
// DefaultLimits; it is not for text of untrusted size. A duplicate member
// name is an error.
func (o *Object) UnmarshalJSON(data []byte) error { panic("unimplemented") }

// Resolver resolves a value whose Go type is outside the admitted set, such
// as a protocol buffer message or an application struct, into a value the
// engine can read. Resolve returns either an admitted value or a value
// implementing ObjectView or ArrayView; anything else is an error
// (CodeUnsupportedValue). A Resolver that does not recognize v returns an
// error; that error is passed to the caller wrapped in an *Error
// (CodeResolver), so errors.Is and errors.As see the cause, and ctx's own
// error is returned as ctx.Err(). The engine calls Resolve once per read of
// a foreign value and does not memoize the result; Resolve must return an
// equal view for the same value for the life of an evaluation, and a
// Resolver whose views are costly to build may memoize by identity itself.
// Each call is charged as work, and the strings and []byte the engine
// observes through the returned view are charged to MaxBytes. Members and
// elements a view returns may themselves be foreign and are resolved when
// read; a carried foreign value returns to the caller as the original
// value, not as its view. A view that is pointer-shaped (one pointer field)
// converts to an interface without allocating.
//
// Resolver is the boundary of the closed environment: an expression can
// drive it with any key, in any order, as many times as the work bound
// allows. Implement it as an allowlist over known types, never as
// reflection over arbitrary structs. The Resolver author decides how a
// protobuf int64 presents, which is a binding decision made before the
// value reaches this package. It is called concurrently only when the
// caller evaluates concurrently.
type Resolver interface {
	Resolve(ctx context.Context, v any) (any, error)
}

// ObjectView is an object the engine reads through the view rather than by
// conversion. Get's ok == false is absence, not null, and (nil, true, nil)
// is null; a non-nil error takes precedence over ok. Range reports members
// in the object's order, which must not change between calls, and every
// order-observing operation observes that order. Get and Range must agree
// on membership; the engine trusts both.
type ObjectView interface {
	Get(ctx context.Context, key string) (value any, ok bool, err error)
	Range(ctx context.Context, fn func(key string, value any) bool) error
}

// ArrayView is an array the engine reads through the view rather than by
// conversion. A negative Len is an error (CodeUnsupportedValue).
type ArrayView interface {
	Len(ctx context.Context) (int, error)
	At(ctx context.Context, i int) (value any, err error)
}

// BytesEncoding presents []byte as a string. It is satisfied by
// *encoding/base64.Encoding.
type BytesEncoding interface{ EncodeToString(src []byte) string }

// Code is an error code: the language's where the reference implementation
// defines one for the situation (S0201, T2001, D1001, D3030, ...; the
// documentation catalogues no codes, so the reference's table is the
// catalogue), or an engine code. A language code is raised only in a
// situation the reference defines for it; every refusal this package adds
// is an E code, so Class() == ClassEngine identifies a refusal another
// member of the class might not make.
type Code string

// Class returns the code's class. Codes are classified by a table, not by
// form: $error and $assert raise D codes that class as ClassRaised. An
// unknown code is ClassUnknown.
func (c Code) Class() Class { panic("unimplemented") }

// Class groups error codes by what a caller can do about them.
type Class uint8

const (
	// ClassUnknown: the code is not one this package knows.
	ClassUnknown Class = iota
	// ClassSyntax: the expression does not parse (S codes). Only $eval can
	// raise one after a successful Compile.
	ClassSyntax
	// ClassType: an operand had the wrong type (T codes).
	ClassType
	// ClassEvaluation: a dynamic failure such as a number out of range or a
	// bad function argument (D codes).
	ClassEvaluation
	// ClassRaised: the expression called $error or a failed $assert.
	ClassRaised
	// ClassEngine: a refusal this package adds (E codes): an unsupported
	// value, an exceeded bound, a bad binding, a closed Evaluation, an
	// engine defect, malformed input text, a Resolver failure, or an inexact
	// conversion.
	ClassEngine
)

// Engine codes. Non-finite results, division by zero, and non-finite inputs
// use the language's own D1001.
const (
	CodeUnsupportedValue Code = "E1001" // a value the engine cannot read: foreign with no Resolver, unresolvable, invalid UTF-8, bytes with no BytesEncoding, or a function leaving an evaluation
	CodeBudget           Code = "E1002" // a Limits bound was exceeded
	CodeBinding          Code = "E1003" // an invalid binding name
	CodeClosed           Code = "E1004" // Select or Complete after Close
	CodeInternal         Code = "E1005" // an engine defect, recovered
	CodeMalformedInput   Code = "E1006" // input text is not one complete JSON value, is not valid UTF-8, has a duplicate member name, or an unpaired surrogate escape
	CodeResolver         Code = "E1007" // the Resolver returned an error; the cause is wrapped
	CodeInexact          Code = "E1008" // an integer operand could not convert to float64 exactly
)

// Sentinels for errors.Is. An *Error matches the sentinel with its Code; a
// sentinel's Error() is its code and name.
var (
	ErrUnsupportedValue error = sentinel(CodeUnsupportedValue)
	ErrBudget           error = sentinel(CodeBudget)
	ErrBinding          error = sentinel(CodeBinding)
	ErrClosed           error = sentinel(CodeClosed)
	ErrInternal         error = sentinel(CodeInternal)
	ErrMalformedInput   error = sentinel(CodeMalformedInput)
	ErrResolver         error = sentinel(CodeResolver)
	ErrInexact          error = sentinel(CodeInexact)
)

type sentinel Code

func (s sentinel) Error() string { panic("unimplemented") }

// Error is a compilation or evaluation failure.
//
// Error() renders Code, Message, and Offset, in a form that is not stable
// across versions. Message never includes content derived from the input or
// the bindings, and Token is drawn from the expression source only. The
// offending value, for errors the language defines as carrying one, is in
// Value only and is charged to the output and byte bounds: for $error and a
// failed $assert, Value is the message the expression supplied, as the
// value it was (a string, an *Object, ...), which Error() does not render,
// so a caller who wants it logs or returns Value itself. Value can hold
// payload or binding content, and a logger that walks the struct will see
// it; a service that logs errors or returns them to clients redacts or
// drops Value. An unsupported-value error names the Go type, never its
// contents.
type Error struct {
	Code    Code
	Message string
	// Offset is the byte offset of the offending token into the expression
	// source, for a failure at Compile or during evaluation alike, or into
	// the input text for CodeMalformedInput, or -1 when unknown, as
	// json.SyntaxError.Offset and go/token report positions. Line and
	// Column are 1-based, Column in bytes, or 0.
	Offset, Line, Column int
	// Token is the offending token, when known, at most 64 characters, and
	// empty for CodeMalformedInput.
	Token string
	// Value is the offending value for errors that carry one.
	Value any
}

// Error implements error.
func (e *Error) Error() string { panic("unimplemented") }

// Is reports whether target is the sentinel for e's Code.
func (e *Error) Is(target error) bool { panic("unimplemented") }

// Unwrap returns the Resolver's error for CodeResolver, and nil otherwise.
func (e *Error) Unwrap() error { panic("unimplemented") }
