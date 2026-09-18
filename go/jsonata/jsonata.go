// Package jsonata evaluates JSONata 2.1 expressions over Go values.
//
// It is the Go member of the OpenBindings evaluator class (see the README at
// the repository root): the pinned JSONata documentation defines what an
// expression does; Go defines what the values are and how they combine.
// Inputs and outputs are ordinary Go values. No JSON text is read or written
// on the evaluation path; Unmarshal and EvalJSON exist for callers that hold
// JSON bytes and add nothing the native path lacks.
//
// # Values
//
// An input is any of: nil, bool, string, []byte, json.Number, int, int8,
// int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32,
// float64, *big.Int, []any, map[string]any, or *Object, or any named type
// whose underlying type is one of those. A nil pointer is null. A nil []any
// is the empty array and a nil map[string]any is the empty object, so that
// an unset Go collection does not become null and change the meaning of a
// predicate over it. A string that is not valid UTF-8 is refused
// (CodeUnsupportedValue). Any other value is reached through the Env's
// Access; without one it is an error (CodeUnsupportedValue) when first used.
// Values are read by reference and are never copied on admission.
//
// A map[string]any has no member order, so every operation that observes
// member order ($keys, $each, $spread, $string, $merge, $sift, object
// construction, the transform operator) sees a map's keys in sorted byte
// order, which is code point order for valid UTF-8. An *Object preserves
// insertion order; a caller who needs insertion order on input supplies an
// *Object, which Unmarshal produces.
//
// An output is a value of the same kinds. A value the expression only
// selected, copied, or rearranged is returned as the value that was carried:
// same type, same identity. So $ over a map[string]any input returns that
// map, while { "a": $.a } returns an *Object. A caller that must handle "an
// object" writes a two-arm switch over map[string]any and *Object, or
// encodes the result, which handles both. A value that Access resolved is
// returned as the resolved value; Materialize converts a result that still
// contains foreign values.
//
// # Ownership
//
// The input and the Env are borrowed for the duration of an Eval, and for
// the life of an Evaluation until Close. Callers must not mutate them in
// that window; a concurrent map mutation is a fatal runtime error in Go, not
// a recoverable one. Returned values may alias the input and the Env's
// bindings: a value bound in Env.Bindings can appear in any result by
// identity. Values returned by an Evaluation's Select or Complete are shared
// with the Evaluation until Close and must not be mutated before then. A
// constructed container (an *Object or []any the expression built) is new,
// but its members may be carried.
//
// # Numbers
//
// Every admitted numeric representation denotes one numeric value, and every
// operation that observes a number (arithmetic, comparison, ordering,
// membership, sort keys, $type, $string, object keys) is a function of that
// value, never of the representation. Only a pure copy preserves the
// representation.
//
// Admission widens without loss: int through int64 are int64; uint through
// uint32 are int64; uint64 is int64 when it fits and *big.Int otherwise;
// float32 is float64. A float64 that is not finite is refused (D1001). A
// json.Number is classified once on first use: an integral token within
// int64 is int64, an integral token beyond int64 is *big.Int, a token with a
// fraction or exponent is the nearest float64, and a token that is not a
// JSON number is an error (D3030). The float64 nearest to a decimal token is
// the model's representation of that decimal, not an approximation of a
// result; the refusal rules below apply to results.
//
// A numeric literal in an expression follows the same rule: 1 and
// 9007199254740993 are int64, 18446744073709551616 is *big.Int, and 1.0, 1e2
// and 0.5 are float64.
//
// Results are int64, float64, or *big.Int. Integer with integer yields an
// exact integer: int64 when the result fits and *big.Int otherwise, so the
// result never depends on which representation the operands arrived in. A
// *big.Int result whose bit length exceeds Limits.MaxIntegerBits is an error
// (D1001). Any float64 operand yields float64; the operation is an error
// (D1001) when its exact result is not representable as float64 or is not
// finite, so 9007199254740993 / 2 succeeds and 9007199254740993 + 0.5 does
// not. "/" yields an integer when both operands are integers and the
// division is exact, and float64 otherwise. "%" is Go's % for integers and
// math.Mod for floats. Integer division or remainder by zero is an error
// (D1001). $round rounds the value half to even; the value is the binary
// float64, so $round(2.675, 2) is 2.67, since 2.675 as a float64 is below
// the midpoint. $string renders an integer as decimal digits and a float64
// as the shortest representation that round-trips, with -0 rendered as 0,
// which is what encoding/json produces.
//
// Equality, ordering, and membership across representations are by value:
// int64(1), float64(1), json.Number("1") and json.Number("1.0") are equal and
// sort together. Comparison never rounds through float64 when both operands
// are exact. Two objects are equal when they have the same members with
// equal values, regardless of member order or of map versus *Object.
//
// # Strings and bytes
//
// A string's characters are Unicode code points, so $length("😀") is 1,
// $substring indexes code points, and $match reports index in code points.
// Ordering of strings is by code point. Case mapping uses Unicode simple case
// mapping, so $uppercase("ß") is "ß". $replace follows the documentation: in
// a string replacement $N is the Nth captured group, $$ is a literal dollar,
// and a group number beyond the groups captured is the empty string; Go's
// ${name} form is not recognized. A regex literal that does not compile
// fails at Compile.
//
// A []byte is a string whose content is its encoded form under the Env's
// BytesEncoding (base64.StdEncoding by default). It is a string for $type,
// and it is compared and ordered by that encoded form, against strings and
// against other []byte values alike. The encoding is produced only when an
// expression uses the value as a string; a pure copy never encodes.
//
// $string of an object or array renders JSON text as the documentation
// specifies by reference to JSON.stringify: no whitespace, no HTML escaping,
// members in the order this package observes them, numbers as above, and
// []byte through the BytesEncoding. With prettify true the indentation is
// two spaces. A result that is a function value is an error (T1006).
//
// # Absence
//
// The language distinguishes an absent result (undefined) from the value
// null. Eval, Select and Complete return (value, present, err): present is
// false for an absent result, a present nil is null, and on a non-nil error
// both value and present are their zero values. A binding whose value is nil
// is null, not absent, so $x ?? d does not fall back for it while $x ?: d
// does; omit the binding to make $x absent.
//
// # Whole and selective evaluation
//
// Eval evaluates the whole expression. Prepare returns an Evaluation from
// which fields of the result are selected individually: when the result is an
// object constructor with distinct literal keys, or a block whose final
// expression is one, and the selected field's subexpression is pure, only
// that field is computed. Every statement of a block prelude is evaluated
// before the first selection, its bindings are shared by every selection,
// and a failure in the prelude fails every selection. A failure in one
// field's subexpression does not fail another field's, so $error or $assert
// in a sibling field is not a guard under selection; a caller for whom a
// transform's failure is meaningful uses Complete or Eval. When Complete
// would succeed, Select returns the same field. Evaluation.Plan reports
// which mode applies and, when selection is unavailable, why.
//
// Eval fixes the timestamp that $now and $millis observe at its start, and
// Prepare fixes it for the life of the Evaluation, so those functions are
// pure for selection and cannot time other work; $random and $eval are not
// pure.
//
// # Bounds
//
// No expression, whatever its content, can terminate the process. Every
// bound in Limits has a finite default, the bounds are part of the compiled
// expression so they apply wherever it is evaluated, and exceeding one is an
// error (CodeBudget) raised before the allocation that would exceed it, not
// after. One Eval, or one Evaluation across all of its selections and
// Complete, has one budget. $eval compiles and evaluates within it. Parsing
// is charged per source byte; every Access call, every BytesEncoding call,
// every comparison in a sort, and every value visited by a descendant
// operator is charged as work; every string, []byte, encoded form, *big.Int,
// $eval source, and Error value is charged as bytes. The bounds count work
// and bytes, not time: cancellation and deadlines come from ctx and are
// checked between operations and, within operations whose cost scales with
// input size, at intervals. A cancelled or expired ctx is returned as
// ctx.Err(), so errors.Is(err, context.Canceled) holds. Compile itself is
// bounded by MaxExpressionBytes and MaxDepth and takes no ctx.
//
// The engine spawns no goroutines and holds no process-wide state. A panic
// inside the engine is a defect and is recovered into an Error
// (CodeInternal) so that the process survives it; a panic inside a caller's
// Access or BytesEncoding propagates to the caller.
//
// # Environment
//
// The evaluation environment is closed except for the clock (which Eval and
// Prepare fix), $random (math/rand/v2, not cryptographic), and the Access and
// BytesEncoding the caller supplies in Env. An expression can call the
// Access with any key, in any order, as many times as the work bound allows;
// Access is therefore the boundary of the closed environment and should be
// an allowlist over known types, never reflection over arbitrary structs.
// This package offers no way to register functions. $eval is available as
// the documentation defines it. $fromMillis and $toMillis use UTC unless a
// picture string specifies an offset; the host's local zone is never
// consulted.
package jsonata

import (
	"context"
	"iter"
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
// cache of compiled expressions can key on them.
type Limits struct {
	// MaxExpressionBytes bounds the length of an expression. Default 256 KiB.
	MaxExpressionBytes int
	// MaxDepth bounds the nesting depth of a parsed expression. Default 128.
	MaxDepth int
	// MaxRecursion bounds evaluation depth, including function recursion and
	// $eval. Default 1024.
	MaxRecursion int
	// MaxOutputNodes bounds the size of a result, counting every value.
	// Default 1 << 20.
	MaxOutputNodes int
	// MaxWork bounds the work of one evaluation, counting nodes evaluated,
	// elements materialized, and calls into Access and BytesEncoding,
	// including intermediates that never reach the result. Default 8 << 20.
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
// how foreign values are read, and how bytes present as strings. A nil *Env
// means no bindings, no Access, and base64.StdEncoding. An Env is read-only
// during evaluation and may be shared by any number of concurrent
// evaluations.
type Env struct {
	// Bindings supplies variables. Names omit the leading "$" and may not
	// begin with one; a name that does is an error (CodeBinding). The map is
	// borrowed, not copied, and its values may appear in results by identity.
	Bindings map[string]any
	// Access reads values outside the admitted set. Nil means none.
	Access Access
	// BytesEncoding presents []byte as a string. Nil means base64.StdEncoding;
	// base64.URLEncoding and the Raw variants satisfy the interface unchanged.
	BytesEncoding BytesEncoding
}

// Compile parses and prepares an expression under limits. The returned
// Expression is immutable and safe for concurrent use; callers evaluating
// many inputs against one expression compile it once. Regex literals are
// compiled here, so a regex that does not compile fails at Compile. Caching
// compiled expressions is the caller's concern; a cache keyed by the source
// and the Limits is sufficient.
func Compile(expression string, limits *Limits) (*Expression, error) { panic("unimplemented") }

// MustCompile is Compile for an expression fixed at build time. It panics on
// error, as regexp.MustCompile does.
func MustCompile(expression string, limits *Limits) *Expression { panic("unimplemented") }

// Eval compiles expression under default limits and evaluates it once. It is
// a convenience for one-off use; repeated evaluation compiles once with
// Compile.
func Eval(ctx context.Context, expression string, input any, env *Env) (value any, present bool, err error) {
	panic("unimplemented")
}

// Unmarshal decodes JSON text into admitted values: numbers as json.Number,
// objects as *Object in member order, arrays as []any. A text that is not
// one complete JSON value, or that contains a duplicate member name within
// an object, is an error.
func Unmarshal(data []byte) (any, error) { panic("unimplemented") }

// Materialize returns v with every foreign value it contains resolved
// through a into admitted values, recursively, so the result contains no
// value that would need an Access to read. It is for callers handing a
// result to an encoder or to typed code. Admitted values are returned as
// they are, by identity.
func Materialize(ctx context.Context, v any, a Access) (any, error) { panic("unimplemented") }

// Expression is a compiled JSONata expression.
type Expression struct{ _ struct{} }

// String returns the expression source, satisfying fmt.Stringer as
// regexp.Regexp does.
func (e *Expression) String() string { panic("unimplemented") }

// Limits returns the bounds the expression was compiled under.
func (e *Expression) Limits() Limits { panic("unimplemented") }

// Reads reports the input members the expression reads when that is
// statically known. Each path is a sequence of member keys from the root; a
// key applies to every element when the value at that point is an array.
// When known is true the list is sound: the expression reads nothing outside
// it, so a caller may fetch only those members. Any use of $$ inside a
// function, a wildcard, a descendant or parent operator, $lookup with a
// computed key, $keys, $each, $spread, $eval, or a binding makes known
// false. The slice is computed at Compile and shared; callers must not
// modify it.
func (e *Expression) Reads() (paths [][]string, known bool) { panic("unimplemented") }

// Eval evaluates the whole expression over input.
func (e *Expression) Eval(ctx context.Context, input any, env *Env) (value any, present bool, err error) {
	panic("unimplemented")
}

// EvalJSON is Eval over Unmarshal(input), with the result encoded by
// encoding/json: *Object in member order, json.Number as its token, []byte
// through the Env's BytesEncoding, and no HTML escaping. An absent result is
// present == false with no bytes and a nil error.
func (e *Expression) EvalJSON(ctx context.Context, input []byte, env *Env) (out []byte, present bool, err error) {
	panic("unimplemented")
}

// Prepare borrows input and env, fixes the timestamp $now and $millis
// observe, and plans selective evaluation without evaluating anything. It
// fails only for an invalid Env. The input is admitted lazily, so an
// unsupported value surfaces from the first Select or Complete. Close the
// Evaluation when done.
func (e *Expression) Prepare(input any, env *Env) (*Evaluation, error) { panic("unimplemented") }

// Evaluation is one expression over one input, from which fields of the
// result are selected individually. Selections share work: a field selected
// twice, or reached by two overlapping selections, is computed once, and a
// later Complete reuses every field already selected. All selections and
// Complete draw on one budget. An Evaluation is safe for concurrent use;
// its Env's Access is called concurrently too.
type Evaluation struct{ _ struct{} }

// Select returns the field of the result at path, where each element is a
// literal key of an object constructor in the expression. When the plan
// permits, only that field's subexpression is evaluated; otherwise the whole
// expression is evaluated once and the field is looked up. A path that
// descends below constructor granularity is a structural lookup within the
// deepest selected field, by member key only. An empty path is Complete. A
// key not among the constructor's keys is absent. An object constructor with
// duplicate literal keys fails as Complete would (D1009). After Close, Select
// returns an error (CodeClosed).
func (v *Evaluation) Select(ctx context.Context, path ...string) (value any, present bool, err error) {
	panic("unimplemented")
}

// Complete evaluates the whole expression, reusing fields already selected,
// and returns the entire result. After Close it returns an error (CodeClosed).
func (v *Evaluation) Complete(ctx context.Context) (value any, present bool, err error) {
	panic("unimplemented")
}

// Plan reports how selections will be served.
func (v *Evaluation) Plan() Plan { panic("unimplemented") }

// Close ends the Evaluation: the input and Env are no longer borrowed, values
// already returned are no longer shared with it and may be mutated, and
// further Select or Complete calls return an error (CodeClosed). An
// Evaluation that is never closed is reclaimed by the garbage collector;
// Close makes the release prompt and the end of the borrow explicit.
func (v *Evaluation) Close() { panic("unimplemented") }

// Plan describes how an Evaluation serves selections.
type Plan struct {
	// Mode is ModeSelective when fields are evaluated individually and
	// ModeComplete when every Select evaluates the whole expression once and
	// looks up.
	Mode Mode
	// Reason explains a ModeComplete plan. It is ReasonNone when Mode is
	// ModeSelective.
	Reason Reason
	// Fields lists the selectable top-level keys when Mode is ModeSelective.
	// The slice is shared; callers must not modify it.
	Fields []string
}

// Mode is a Plan's evaluation mode.
type Mode uint8

const (
	// ModeSelective means fields are evaluated individually on demand.
	ModeSelective Mode = iota + 1
	// ModeComplete means the whole expression is evaluated once and fields
	// are looked up in the result.
	ModeComplete
)

// String returns the mode's name.
func (m Mode) String() string { panic("unimplemented") }

// Reason explains why a Plan is ModeComplete.
type Reason uint8

const (
	// ReasonNone: the plan is selective.
	ReasonNone Reason = iota
	// ReasonDynamicShape: the result is not an object constructor with
	// literal keys.
	ReasonDynamicShape
	// ReasonImpure: a field's subexpression calls $random or $eval.
	ReasonImpure
	// ReasonUnsupportedCall: a field's subexpression calls a function the
	// planner has not qualified as selectable.
	ReasonUnsupportedCall
	// ReasonArrayInput: the input is an array, so the constructor maps over
	// it and the result is an array.
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

// Access resolves a value whose Go type is outside the admitted set, such
// as a protocol buffer message or an application struct, into a value the
// engine can read. The engine consults it once per foreign value; admitted
// values never pass through it. Resolve returns either an admitted value or
// a value implementing ObjectView or ArrayView; anything else is an error
// (CodeUnsupportedValue). Members and elements a view returns may themselves
// be foreign and are resolved when read. Each call is charged as work.
//
// Access is the boundary of the closed environment: an expression can drive
// it with any key, in any order, as many times as the work bound allows.
// Implement it as an allowlist over known types. It is called concurrently.
type Access interface {
	Resolve(ctx context.Context, v any) (any, error)
}

// ObjectView is an object the engine reads through the view rather than by
// conversion. Get's ok == false is absence, not null.
type ObjectView interface {
	Get(ctx context.Context, key string) (value any, ok bool, err error)
	// Range calls fn for each member in the object's order until fn returns
	// false or an error occurs.
	Range(ctx context.Context, fn func(key string, value any) bool) error
}

// ArrayView is an array the engine reads through the view rather than by
// conversion.
type ArrayView interface {
	Len(ctx context.Context) (int, error)
	At(ctx context.Context, i int) (value any, err error)
}

// BytesEncoding is satisfied by *encoding/base64.Encoding.
type BytesEncoding interface{ EncodeToString(src []byte) string }

// Code is an error code: the language's where the documentation defines one
// (S0201, T2001, D1001, D3030, ...) or an engine code for refusals the
// language leaves to the implementation.
type Code string

// Class returns the code's class, derived from its form.
func (c Code) Class() Class { panic("unimplemented") }

// Class groups error codes by what a caller can do about them.
type Class uint8

const (
	// ClassSyntax: the expression does not parse (S codes). Only $eval can
	// raise one after a successful Compile.
	ClassSyntax Class = iota + 1
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
	CodeUnsupportedValue Code = "E1001" // a value outside the admitted set with no Access, or one the Access cannot resolve
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

// Error is a compilation or evaluation failure. Use errors.As, or errors.Is
// against a sentinel. Context cancellation and deadlines are not Errors;
// they are returned as ctx.Err().
//
// Error() renders Code, Message, and Position only. Message never includes
// content derived from the input or the bindings; the offending value, for
// errors the language defines as carrying one (such as $error's argument),
// is in Value only, and is charged to the output and byte bounds. Token is at
// most 64 characters. An unsupported-value error names the Go type, never
// its contents.
type Error struct {
	Code    Code
	Message string
	// Position is the offset into the expression source in characters (code
	// points), or -1; Line and Column are 1-based, or 0. Characters rather
	// than bytes because editors and the reference report columns in
	// characters.
	Position, Line, Column int
	// Token is the offending token, when known.
	Token string
	// Value is the offending value for errors that carry one.
	Value any
}

// Error implements error.
func (e *Error) Error() string { panic("unimplemented") }

// Is reports whether target is an *Error with the same Code.
func (e *Error) Is(target error) bool { panic("unimplemented") }
