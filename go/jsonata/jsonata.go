// Package jsonata evaluates JSONata 2.1 expressions over Go values.
//
// The JSONata documentation pinned by DocumentationCommit defines what an
// expression does. This package's value model, built from Go's own types,
// defines what values are and how they combine, the way regexp implements
// regular-expression notation over Go strings rather than porting another
// engine. Integers are exact at any size, floats are float64, and a result
// that cannot be represented exactly is an error, never an approximation.
// No expression, whatever its content, can terminate the process. The
// design is written up at https://github.com/openbindings/jsonata-evaluator.
//
// Status: design. Every function in this package is a signature and a doc
// comment; none is implemented.
//
// Compile once, evaluate many:
//
//	expr, err := jsonata.Compile(`{ "id": user_id, "name": display_name }`, nil)
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
// # Getting values in
//
// Inputs are Go values, not JSON text. A caller holding JSON bytes decodes
// them with Unmarshal, which keeps every integer exact; decoding with
// encoding/json into any turns 9007199254740993 into the float64
// 9007199254740992 before this package sees it. A json.Decoder with
// UseNumber set is also fine.
//
// These are the admitted values: nil, bool, string, []byte, json.Number,
// json.RawMessage, int, int8, int16, int32, int64, uint, uint8, uint16,
// uint32, uint64, float32, float64, *big.Int, *Object, and any []T or
// map[string]T whose T is admitted (so []any, map[string]any, []string,
// map[string]int64, and so on), and any named type whose underlying type is
// one of those. A json.RawMessage is decoded as JSON on first read. A nil
// []T is the empty array and a nil map[string]T is the empty object, because
// Go code routinely leaves a collection unset and $count(items) = 0 should
// hold for it rather than items being null; every nil pointer, including a
// nil *Object and a nil *big.Int, is null. A string that is not valid UTF-8
// is an error (CodeUnsupportedValue).
//
// Anything else is foreign. The expression can read a foreign value only
// through the Resolver in Env; without one it is an error
// (CodeUnsupportedValue). Admission is per value, on first read: a value the
// expression never reads is never checked, and Prepare checks nothing.
// Values are read by reference and never copied on admission.
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
// preserves a value's representation. So $ over a map[string]any input
// returns that map, while { "a": $.a } returns a new *Object. A caller that
// must handle "an object" writes a two-arm switch over map[string]any and
// *Object, or encodes the result with Marshal, which handles both. A foreign
// value that was carried is returned as the original foreign value, never as
// the view the Resolver produced; Materialize converts a result that still
// contains foreign values. A result that is a function value cannot leave an
// evaluation and is an error (CodeUnsupportedValue).
//
// # Errors
//
// A failure is an *Error, matched with errors.As against *Error or with
// errors.Is against a sentinel such as ErrBudget; an *Error matches any
// *Error with the same Code, so errors.Is(err, &Error{Code: "D1001"}) works
// for the language's own codes. Code.Class groups codes by what a caller can
// do about them. Context cancellation and deadlines are not Errors; they
// are returned as ctx.Err(), so errors.Is(err, context.Canceled) holds.
//
// # Numbers
//
// Every admitted numeric representation denotes one numeric value, and every
// operation that observes a number (arithmetic, comparison, ordering,
// membership, sort keys, $type, $string, object keys) is a function of that
// value, never of the representation. Only carriage preserves the
// representation.
//
// Admission widens without loss: int through int64 and uint through uint32
// are int64; uint and uint64 are int64 when they fit and *big.Int otherwise;
// float32 is float64. A float64 that is not finite is an error (D1001). A
// json.Number is classified once on first read: an integral token within
// int64 is int64, an integral token beyond int64 is *big.Int, a token with a
// fraction or exponent is the nearest float64 (that float64 is what a
// decimal is in this model, not a rounding of a result), and a token that is
// not a JSON number is an error (CodeUnsupportedValue). A numeric literal in
// an expression follows the same rule: 1 and 9007199254740993 are int64,
// 18446744073709551616 is *big.Int, and 1.0, 1e2, and 0.5 are float64.
//
// Results are int64, float64, or *big.Int. Integer with integer yields an
// exact integer, int64 when the result fits and *big.Int otherwise, so the
// result never depends on which representation the operands arrived in; a
// *big.Int result whose bit length exceeds Limits.MaxIntegerBits is an error
// (D1001). An integer operand combined with a float64 operand must convert
// to float64 exactly, and is an error (D1001) otherwise, so 9007199254740993
// + 0.5 fails while 0.1 + 0.2 yields the float64 sum. "/" over two integers
// yields the exact integer when the division is exact and otherwise the
// exact quotient rounded once to float64, so 1758000000000000123 / 1000000
// yields 1758000000000.0001 rather than failing. "%" is Go's % for integers
// and math.Mod for floats. Division or remainder by zero and a non-finite
// result are errors (D1001). $sum, $abs, $floor, $ceil, $round at precision
// 0, and $power with a non-negative integer exponent stay in the integer
// domain; $sqrt, $average, and $power otherwise yield float64.
//
// $round rounds half to even on the binary float64 value, so $round(2.675,
// 2) is 2.67 (2.675 as a float64 is below the midpoint); the reference
// implementation rounds the decimal spelling and yields 2.68, a declared
// divergence. $string renders an integer as its decimal digits and a float64
// as the shortest decimal that round-trips, with -0 rendered as 0, as the
// documentation specifies by reference to JSON.stringify; the reference
// implementation renders floats to 15 significant digits, so $string(0.1 +
// 0.2) is "0.3" there and "0.30000000000000004" here, a declared divergence.
//
// Equality, ordering, and membership across representations are by value:
// int64(1), float64(1), json.Number("1"), and json.Number("1.0") are equal
// and sort together, and comparison never rounds through float64 when both
// operands are exact.
//
// # Strings, bytes, and regular expressions
//
// A string's characters are Unicode code points, so $length("😀") is 1,
// $substring indexes code points, and $match reports index in code points.
// Ordering of strings is by code point. Case mapping is full Unicode case
// mapping, so $uppercase("straße") is "STRASSE". $replace follows the
// documentation: in a string replacement $N is the Nth captured group, $$ is
// a literal dollar, and a group number beyond the groups captured is the
// empty string; Go's ${name} form is not recognized. Regex literals are
// compiled by Compile, so a malformed literal fails there rather than at
// evaluation.
//
// A []byte is a string for $type, $length, $contains, and every string
// function; its content, for those purposes and for equality with a string,
// is its encoding under the Env's BytesEncoding, produced only when an
// expression uses the value as a string. Two []byte values are equal and
// ordered by their bytes; ordering a []byte against a string is an error
// (T2010), as ordering a number against a string is. Carriage never encodes.
//
// $string of an object or array renders JSON text as the documentation
// specifies by reference to JSON.stringify: no whitespace, no HTML escaping,
// members in the order this package observes them, numbers as above, []byte
// through the BytesEncoding, and a function as the empty string. With
// prettify true the indentation is two spaces.
//
// # Objects and member order
//
// A map[string]any has no member order, so every operation that observes
// member order ($keys, $each, $spread, $string, $merge, $sift, object
// construction, the transform operator) sees a map's keys in sorted byte
// order, which is code point order for valid UTF-8. An *Object preserves
// insertion order; Unmarshal produces *Object so that decoded JSON keeps
// document order, and a caller who needs insertion order on other input
// supplies an *Object. The reference implementation orders integer-like keys
// numerically before all others, a JavaScript artifact this package does not
// reproduce; declared. Two objects are equal when they have the same members
// with equal values, regardless of member order or of map versus *Object.
//
// # Ownership and concurrency
//
// An Expression is immutable and safe for concurrent use. The input and Env
// are borrowed for the duration of an Eval, and for the life of an
// Evaluation until Close; callers must not mutate them in that window, and a
// concurrent map mutation is a fatal runtime error in Go, not a recoverable
// one. Returned values may alias the input and the Env's bindings: a value
// in Env.Bindings can appear in any result by identity. Values returned by an
// Evaluation's Select or Complete are shared with the Evaluation until Close
// and must not be mutated before then. A constructed container is new, but
// its members may be carried. The engine starts no goroutines and holds no
// process-wide state; the Resolver is called concurrently only when the
// caller evaluates concurrently.
//
// # Selective evaluation
//
// Eval evaluates the whole expression. Prepare returns an Evaluation from
// which fields of the result are selected individually. Selection is
// available when the result is an object constructor with distinct literal
// keys, or a block whose final expression is one, and the selected field's
// subexpression is pure. In a block, the statements before the final
// expression (the prelude) are evaluated once before the first selection,
// their bindings are shared by every selection, and a failure among them
// fails every selection; this is the way to guard a transform under
// selection. A failure in one field's subexpression does not fail another
// field's, so $error or $assert in a sibling field is not a guard under
// selection. When Complete would succeed, Select returns the same field.
// Evaluation.Plan reports whether selection applies and, when it does not,
// why.
//
//	ev, err := expr.Prepare(in, nil)
//	if err != nil { ... }
//	defer ev.Close()
//	id, present, err := ev.Select(ctx, "id")      // only the "id" field runs
//	all, present, err := ev.Complete(ctx)         // reuses "id", computes the rest
//
// Eval fixes the timestamp that $now and $millis observe at its start, and
// Prepare fixes it for the life of the Evaluation, so those functions are
// pure for selection and cannot time other work; $random and $eval are not
// pure. Env.Now overrides the clock, for tests.
//
// # Bounds and cancellation
//
// Every bound in Limits has a finite default, the bounds are part of the
// compiled expression so they apply wherever it is evaluated, and exceeding
// one is an error (CodeBudget) raised before the allocation that would
// exceed it. One Eval, or one Evaluation across all of its selections and
// Complete, has one budget, and $eval compiles and evaluates within it. The
// bounds count work and bytes, not time: cancellation and deadlines come
// from ctx and are checked between operations and, within operations whose
// cost scales with input size, at intervals. Compile takes no ctx; it is
// bounded by MaxExpressionBytes and MaxDepth. A panic inside the engine is a
// defect and is recovered into an Error (CodeInternal) so the process
// survives it; a panic inside a caller's Resolver or BytesEncoding
// propagates to the caller.
//
// # Environment
//
// The evaluation environment is closed except for the clock (which Eval and
// Prepare fix), $random (math/rand/v2, not cryptographic), and the Resolver
// and BytesEncoding the caller supplies in Env; see Resolver for what that
// door means. This package offers no way to register functions, and
// Env.Bindings holds values only. $eval is available as the documentation
// defines it. Tail calls are eliminated as the documentation describes and
// do not count against MaxRecursion. $now, $fromMillis, and $toMillis use UTC
// unless the documented timezone argument (±HHMM) or the parsed text
// supplies an offset; the host's local zone is never consulted.
//
// # Conformance
//
// This package implements JSONata Version as defined by the documentation at
// https://github.com/jsonata-js/jsonata/tree/5d1473277e0022d8580e00f891b12080eb3edd74/website/versioned_docs
// (DocumentationCommit). Where its behavior departs from the reference
// implementation's test suite, the departure is recorded with its reason in
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

// Limits bounds what a compiled expression may cost. A zero field means the
// default for that field; a nil *Limits means all defaults. Limits are part
// of the compiled Expression and are read back with Expression.Limits, so a
// cache of compiled expressions can key on them. A carried value counts as
// one node however large it is.
type Limits struct {
	// MaxExpressionBytes bounds the length of an expression, including a
	// source passed to $eval. Default 256 KiB.
	MaxExpressionBytes int
	// MaxDepth bounds the nesting depth of a parsed expression. Default 128.
	MaxDepth int
	// MaxRecursion bounds evaluation depth, including function recursion and
	// $eval; tail calls do not count. Default 1024.
	MaxRecursion int
	// MaxOutputNodes bounds the size of a result, counting every constructed
	// value and each carried value once. Default 1 << 20.
	MaxOutputNodes int
	// MaxWork bounds the work of one evaluation: nodes evaluated, elements
	// materialized, source bytes parsed (including by $eval), calls into the
	// Resolver and BytesEncoding, comparisons in a sort, and values visited
	// by a descendant operator, including intermediates that never reach the
	// result. Default 8 << 20.
	MaxWork int
	// MaxBytes bounds the bytes allocated by one evaluation for strings,
	// []byte, encoded forms, *big.Int, $eval sources, regex programs, and
	// Error values, including intermediates. Default 64 MiB.
	MaxBytes int
	// MaxIntegerBits bounds the magnitude of a *big.Int result. Default 4096.
	MaxIntegerBits int
}

// DefaultLimits returns the defaults.
func DefaultLimits() Limits { panic("unimplemented") }

// Env is the value-model side of an evaluation: what variables are bound,
// how foreign values are read, how bytes present as strings, and what time
// it is. A nil *Env means no bindings, no Resolver, base64.StdEncoding, and
// the real clock. An Env is read-only during evaluation and may be shared by
// any number of concurrent evaluations.
type Env struct {
	// Bindings supplies variables. Names omit the leading "$" and may not
	// begin with one; a name that does is an error (CodeBinding) from
	// Prepare or Eval. Values only; a function cannot be bound. The map is
	// borrowed, not copied, and its values may appear in results by
	// identity.
	Bindings map[string]any
	// Resolver reads values outside the admitted set. Nil means none.
	Resolver Resolver
	// BytesEncoding presents []byte as a string. Nil means base64.StdEncoding;
	// base64.URLEncoding and the Raw variants satisfy the interface unchanged.
	BytesEncoding BytesEncoding
	// Now supplies the timestamp $now and $millis observe. Nil means
	// time.Now. It is called once per Eval or Prepare.
	Now func() time.Time
}

// Compile parses and prepares an expression under limits; nil means
// DefaultLimits. The returned Expression is immutable and safe for
// concurrent use; callers evaluating many inputs against one expression
// compile it once. Caching compiled expressions is the caller's concern; a
// cache keyed by the source and the Limits is sufficient.
func Compile(expression string, limits *Limits) (*Expression, error) { panic("unimplemented") }

// MustCompile is Compile for an expression fixed at build time. It panics on
// error, as regexp.MustCompile does.
func MustCompile(expression string, limits *Limits) *Expression { panic("unimplemented") }

// Eval compiles expression under DefaultLimits and evaluates it once; env
// may be nil. It is a convenience for one-off use; repeated evaluation
// compiles once with Compile.
func Eval(ctx context.Context, expression string, input any, env *Env) (value any, present bool, err error) {
	panic("unimplemented")
}

// Unmarshal decodes JSON text into admitted values: numbers as json.Number,
// objects as *Object in member order, arrays as []any. A text that is not
// one complete JSON value, or that contains a duplicate member name within
// an object, is an error (CodeUnsupportedValue).
func Unmarshal(data []byte) (any, error) { panic("unimplemented") }

// Marshal encodes a result as JSON text the way EvalJSON does: *Object in
// member order, a map in sorted key order, json.Number as its token, *big.Int
// as digits, []byte through env's BytesEncoding, and no HTML escaping. A
// foreign value in v is an error; Materialize it first. env may be nil.
func Marshal(v any, env *Env) ([]byte, error) { panic("unimplemented") }

// NewEncoder returns an Encoder that writes results to w as Marshal would.
func NewEncoder(w io.Writer, env *Env) *Encoder { panic("unimplemented") }

// Encoder writes results as JSON text.
type Encoder struct{ _ struct{} }

// Encode writes v followed by a newline.
func (e *Encoder) Encode(v any) error { panic("unimplemented") }

// Materialize returns v with every foreign value it contains resolved
// through r into admitted values, recursively, so the result contains no
// value that would need a Resolver to read: an ObjectView becomes an *Object
// and an ArrayView a []any. It is for callers handing a result to typed
// code. Admitted values are returned by identity. The walk is bounded by
// limits (nil means DefaultLimits) as an evaluation would be.
func Materialize(ctx context.Context, v any, r Resolver, limits *Limits) (any, error) {
	panic("unimplemented")
}

// Expression is a compiled JSONata expression.
type Expression struct{ _ struct{} }

// String returns the expression source, satisfying fmt.Stringer as
// regexp.Regexp does.
func (e *Expression) String() string { panic("unimplemented") }

// Limits returns the bounds the expression was compiled under, with
// defaults filled in.
func (e *Expression) Limits() Limits { panic("unimplemented") }

// Reads reports the input members the expression reads when that is
// statically known. Each path is a sequence of member keys from the root; a
// key applies to every element when the value at that point is an array.
// When known is true the list is sound: the expression reads nothing outside
// it, so a caller may fetch only those members. Any use of $$ inside a
// function, a wildcard, a descendant or parent operator, $lookup with a
// computed key, $keys, $each, $spread, $eval, or a binding makes known
// false. The returned slices are fresh.
func (e *Expression) Reads() (paths [][]string, known bool) { panic("unimplemented") }

// Eval evaluates the whole expression over input; env may be nil.
func (e *Expression) Eval(ctx context.Context, input any, env *Env) (value any, present bool, err error) {
	panic("unimplemented")
}

// EvalJSON is Eval over Unmarshal(input), with the result encoded as Marshal
// encodes it; env may be nil. An absent result is out == nil, present ==
// false, and a nil error. A result containing foreign values is
// materialized through the Env's Resolver before encoding.
func (e *Expression) EvalJSON(ctx context.Context, input []byte, env *Env) (out []byte, present bool, err error) {
	panic("unimplemented")
}

// Prepare borrows input and env, fixes the timestamp $now and $millis
// observe, and plans selective evaluation without evaluating anything; env
// may be nil. It fails only for an invalid Env. Close the Evaluation when
// done.
func (e *Expression) Prepare(input any, env *Env) (*Evaluation, error) { panic("unimplemented") }

// Evaluation is one expression over one input, from which fields of the
// result are selected individually. Selections share work: a field selected
// twice, or reached by two overlapping selections, is computed once, and a
// later Complete reuses every field already selected. All selections and
// Complete draw on one budget. An Evaluation is safe for concurrent use.
type Evaluation struct{ _ struct{} }

// Select returns the field of the result at path, where each element is a
// literal key of an object constructor in the expression. When the plan
// permits, only that field's subexpression is evaluated; otherwise the whole
// expression is evaluated once and the field is looked up. A path that
// descends below constructor granularity is a lookup by member key within
// the deepest selected field; descending into an array yields absent. An
// empty path is Complete. A key not among the constructor's keys is absent.
// An object constructor with duplicate literal keys fails as Complete would
// (D1009). Select is not a guard; a caller for whom the transform's failure
// is meaningful uses Complete or Eval.
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

// Close ends the Evaluation: the input and Env are no longer borrowed, values
// already returned are no longer shared with it and may be mutated, and
// further Select or Complete calls return an error (CodeClosed). Close is
// idempotent. An Evaluation that is never closed is reclaimed by the garbage
// collector; Close makes the release prompt and the end of the borrow
// explicit.
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

// Fields iterates the selectable top-level keys when the plan is selective.
func (p Plan) Fields() iter.Seq[string] { panic("unimplemented") }

// Reason explains why a Plan is not selective.
type Reason uint8

const (
	// ReasonNone: fields are evaluated individually.
	ReasonNone Reason = iota
	// ReasonDynamicShape: the result is not an object constructor with
	// literal keys.
	ReasonDynamicShape
	// ReasonImpure: a field's subexpression calls $random or $eval.
	ReasonImpure
	// ReasonUnsupportedCall: a field's subexpression calls a function the
	// planner has not qualified as selectable.
	ReasonUnsupportedCall
	// ReasonArrayInput: the input is an array, so the constructor groups over
	// it and each field's value is evaluated over the whole input.
	ReasonArrayInput
	// ReasonPlanBudget: the expression exceeded the planner's work bound.
	ReasonPlanBudget
)

// String returns the reason's name.
func (r Reason) String() string { panic("unimplemented") }

// Object is an insertion-ordered object: the type of every object the
// expression constructs, and a valid input where key order matters. The zero
// Object is empty and ready to use. Object is not safe for concurrent
// mutation. Get is O(1); Delete is O(n).
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

// Map returns the members as a Go map. Order is not preserved; nested
// *Object values are not converted.
func (o *Object) Map() map[string]any { panic("unimplemented") }

// MarshalJSON encodes the object with members in insertion order.
func (o *Object) MarshalJSON() ([]byte, error) { panic("unimplemented") }

// UnmarshalJSON replaces the object's members with those of a JSON object,
// preserving order, with numbers as json.Number and nested objects as
// *Object. A duplicate member name is an error.
func (o *Object) UnmarshalJSON(data []byte) error { panic("unimplemented") }

// Resolver resolves a value whose Go type is outside the admitted set, such
// as a protocol buffer message or an application struct, into a value the
// engine can read. Resolve returns either an admitted value or a value
// implementing ObjectView or ArrayView; anything else is an error
// (CodeUnsupportedValue). An error Resolve returns is passed to the caller
// wrapped, so errors.Is and errors.As see it; a ctx error is returned as
// ctx.Err(). The engine calls Resolve once per read of a foreign value and
// charges each call as work. Members and elements a view returns may
// themselves be foreign and are resolved when read; a carried foreign value
// returns to the caller as the original value, not as its view.
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
// conversion. Get's ok == false is absence, not null. Range reports members
// in the object's order, and $keys observes that order.
type ObjectView interface {
	Get(ctx context.Context, key string) (value any, ok bool, err error)
	Range(ctx context.Context, fn func(key string, value any) bool) error
}

// ArrayView is an array the engine reads through the view rather than by
// conversion.
type ArrayView interface {
	Len(ctx context.Context) (int, error)
	At(ctx context.Context, i int) (value any, err error)
}

// BytesEncoding presents []byte as a string. It is satisfied by
// *encoding/base64.Encoding.
type BytesEncoding interface{ EncodeToString(src []byte) string }

// Code is an error code: the language's where the documentation defines one
// (S0201, T2001, D1001, D3030, ...) or an engine code for refusals the
// language leaves to the implementation.
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
	// ClassEngine: a refusal the language leaves to the implementation (E
	// codes): an unsupported value, an exceeded bound, a bad binding, a
	// closed Evaluation, or an engine defect.
	ClassEngine
)

// Engine codes. Integer overflow and non-finite results use the language's
// own D1001.
const (
	CodeUnsupportedValue Code = "E1001" // a value the engine cannot read: foreign with no Resolver, unresolvable, malformed, or a function leaving an evaluation
	CodeBudget           Code = "E1002" // a Limits bound was exceeded
	CodeBinding          Code = "E1003" // an invalid binding name
	CodeClosed           Code = "E1004" // Select or Complete after Close
	CodeInternal         Code = "E1005" // an engine defect, recovered
)

// Sentinels for errors.Is. An *Error matches a sentinel with the same Code.
var (
	ErrUnsupportedValue error = &Error{Code: CodeUnsupportedValue}
	ErrBudget           error = &Error{Code: CodeBudget}
	ErrBinding          error = &Error{Code: CodeBinding}
	ErrClosed           error = &Error{Code: CodeClosed}
	ErrInternal         error = &Error{Code: CodeInternal}
)

// Error is a compilation or evaluation failure.
//
// Error() renders Code, Message, and Offset only. Message never includes
// content derived from the input or the bindings; the offending value, for
// errors the language defines as carrying one (such as $error's argument),
// is in Value only, and is charged to the output and byte bounds. Token is at
// most 64 characters. An unsupported-value error names the Go type, never
// its contents.
type Error struct {
	Code    Code
	Message string
	// Offset is the byte offset into the expression source of the offending
	// token, or -1 when unknown, as json.SyntaxError.Offset and go/token
	// report positions. Line and Column are 1-based, Column in bytes, or 0.
	Offset, Line, Column int
	// Token is the offending token, when known.
	Token string
	// Value is the offending value for errors that carry one.
	Value any
}

// Error implements error.
func (e *Error) Error() string { panic("unimplemented") }

// Is reports whether target is an *Error with the same Code.
func (e *Error) Is(target error) bool { panic("unimplemented") }
