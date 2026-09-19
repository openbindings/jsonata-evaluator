// Package jsonata evaluates JSONata 2.1 expressions over Go values.
//
// The JSONata documentation pinned by DocumentationCommit defines what an
// expression does. This package's value model, built from Go's own types,
// defines what values are and how they combine, the way regexp implements
// regular-expression notation over Go strings rather than porting another
// engine. Integers are exact up to Limits.MaxIntegerBits, decimals are
// float64, and an operand is never converted inexactly: a value that cannot
// be represented exactly is an error, never an approximation. No expression,
// whatever its content, can terminate the process. The design is written up
// at https://github.com/openbindings/jsonata-evaluator.
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
// uint32, uint64, float32, float64, *big.Int, *Object, any value implementing
// ObjectView or ArrayView, and any []T or map[string]T whose T is admitted
// (so []any, map[string]any, []string, map[string]int64, and so on), and any
// named type whose underlying type is one of those. Classification is by
// exact type first: []byte, which is []uint8, is bytes and never an array of
// numbers; json.RawMessage is decoded as JSON on first read; any other named
// type whose underlying type is []byte is bytes. A nil []byte is the empty
// bytes. A nil []T is the empty array and a nil map[string]T is the empty
// object, because Go code routinely leaves a collection unset and
// $count(items) = 0 should hold for it rather than items being null; every
// nil pointer, including a nil *Object and a nil *big.Int, is null. A string
// that is not valid UTF-8 is an error (CodeUnsupportedValue). A view is read
// through its methods wherever it appears: as the input, in a binding, or
// from a Resolver.
//
// Anything else is foreign. The expression can read a foreign value only
// through the Resolver in Env; without one it is an error
// (CodeUnsupportedValue). A foreign value nested inside an admitted
// container, or bound in Env.Bindings, is resolved when read like any other.
// Admission is per value, on first read: a value the expression never reads
// is never checked, and Prepare checks nothing but the root. Values are read
// by reference and never copied on admission.
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
// preserves a value's representation. So $ over a map[string]any input
// returns that map, while { "a": $.a } returns a new *Object. A caller that
// must handle "an object" uses Member, which reads either, writes a two-arm
// switch over map[string]any and *Object, or encodes the result with
// Marshal, which handles both. A foreign value that was carried is returned
// as the original foreign value, never as the view the Resolver produced;
// Materialize converts a result that still contains foreign values. A result
// that is a function value cannot leave an evaluation and is an error
// (CodeUnsupportedValue).
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
// ClassSyntax carrying Offset, Line, Column, and Token.
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
// json.Number is classified each time it is read (it is a string and holds
// no cache): an integral token within int64 is int64, an integral token
// beyond int64 is *big.Int, a token with a fraction or exponent is the
// nearest float64 (that float64 is what a decimal is in this model, not a
// rounding of a result), a token whose nearest float64 is not finite is an
// error (D1001), and a token that is not a JSON number is an error
// (CodeUnsupportedValue). Unmarshal decodes straight to int64, *big.Int, and
// float64 so that decoded input pays no such cost. A numeric literal in an
// expression follows the same rule: 1 and 9007199254740993 are int64,
// 18446744073709551616 is *big.Int, and 1.0, 1e2, and 0.5 are float64.
//
// A value is an integer when it is integral, whatever its representation: a
// float64 whose value is integral and of magnitude at most 2^53 takes part in
// arithmetic and comparison as that integer, so x / 100.0 is x / 100 and
// json.Number("1.0") + 9007199254740993 is exact. A float64 that is not
// integral, or is integral beyond 2^53, is a decimal.
//
// Results are int64, float64, or *big.Int. Integer with integer yields an
// exact integer, int64 when the result fits and *big.Int otherwise, so the
// result never depends on which representation the operands arrived in; a
// *big.Int result whose bit length exceeds Limits.MaxIntegerBits is an error
// (CodeBudget). An integer operand combined with a decimal operand must
// convert to float64 exactly, and is an error (CodeInexact) otherwise, so
// 9007199254740993 + 0.5 fails while 0.1 + 0.2 yields the float64 sum. "/"
// over two integers yields the exact integer when the division is exact and
// otherwise the exact quotient rounded once to float64, so
// 1758000000000000123 / 1000000 yields 1758000000000.0001 rather than
// failing. The principle is that an operand is never converted inexactly,
// while a non-integral result of exact operands is rounded once, because the
// language defines division to yield fractions and float64 is what a
// fraction is in this model. "%" is Go's % for integers and math.Mod for
// floats. Division or remainder by zero and a non-finite result are errors
// (D1001). $sum, $abs, $floor, $ceil, $round at precision 0, and $power with
// a non-negative integer exponent stay in the integer domain; $sqrt,
// $average, and $power otherwise yield float64. $sum adds left to right.
// $sqrt is correctly rounded; $power with a non-integer exponent is math.Pow
// and may differ from another host's in the last unit.
//
// $round rounds half to even on the binary float64 value, so $round(2.675,
// 2) is 2.67 (2.675 as a float64 is below the midpoint); the reference
// implementation rounds the decimal spelling and yields 2.68, a declared
// divergence. $string renders an integer as its decimal digits and a float64
// as JSON.stringify does, which the documentation specifies: the shortest
// digits that round-trip, plain notation for magnitudes from 1e-6 up to but
// excluding 1e21 and exponent notation with an unpadded exponent otherwise,
// -0 as 0. So $string(1e6) is "1000000", $string(1e21) is "1e+21", and
// $string(1e-7) is "1e-7"; encoding/json renders float64 the same way, and
// strconv's 'g' format does not. The reference implementation renders floats
// to 15 significant digits, so $string(0.1 + 0.2) is "0.3" there and
// "0.30000000000000004" here, a declared divergence.
//
// Equality, ordering, and membership across representations are by value:
// int64(1), float64(1), json.Number("1"), and json.Number("1.0") are equal
// and sort together, and comparison never rounds through float64 when both
// operands are exact; an exact integer compared with a decimal is compared
// exactly, so 9007199254740993 > 9007199254740992.0.
//
// # Strings, bytes, and regular expressions
//
// A string's characters are Unicode code points, so $length("😀") is 1,
// $substring indexes code points, and $match reports index in code points.
// Ordering of strings is by code point; the reference implementation orders
// by UTF-16 code unit, a declared divergence. Case mapping is full Unicode
// case mapping, so $uppercase("straße") is "STRASSE". $sort is stable.
// $replace follows the documentation: in a string replacement $N is the Nth
// captured group, $$ is a literal dollar, and a group number beyond the
// groups captured is the empty string; Go's ${name} form is not recognized.
// Regex literals are compiled by Compile, so a malformed literal fails there
// rather than at evaluation.
//
// A []byte is a string for every purpose that observes its content: $type,
// $length, $contains, every string function, equality, and ordering. That
// content is its encoding under the Env's BytesEncoding, produced only when
// an expression observes it; with no BytesEncoding, observing a []byte's
// content is an error (CodeUnsupportedValue), because the class requires that
// the binding, not the evaluator, choose the encoding. Two []byte values are
// equal by their bytes, which agrees with equality of their encodings, and
// ordered by their encodings, as they are ordered against strings. Carriage
// never encodes.
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
// A map[string]any has no member order, so every operation that observes
// member order ($keys, $each, $spread, $string, $merge, $sift, object
// construction, the transform operator) sees a map's keys in sorted byte
// order, which is code point order for valid UTF-8. An *Object preserves
// insertion order; Unmarshal produces *Object so that decoded JSON keeps
// document order, and a caller who needs insertion order on other input
// supplies an *Object. An ObjectView's Range order is its order for every
// such operation. The reference implementation orders integer-like keys
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
// in Env.Bindings can appear in any result by identity, so a caller who
// shares bindings across evaluations must not mutate results that may alias
// them. Values returned by an Evaluation's Select or Complete are shared with
// the Evaluation until Close and must not be mutated before then. A
// constructed container is new, but its members may be carried. The engine
// starts no goroutines and holds no process-wide state; the Resolver is
// called concurrently only when the caller evaluates concurrently.
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
//	ev, err := expr.Prepare(ctx, in, nil)
//	if err != nil { ... }
//	defer ev.Close()
//	id, present, err := ev.Select(ctx, "id")      // only the "id" field runs
//	all, present, err := ev.Complete(ctx)         // reuses "id", computes the rest
//
// Eval fixes the timestamp that $now and $millis observe at its start, and
// Prepare fixes it for the life of the Evaluation, so those functions are
// pure for selection and cannot time other work; $random and $eval are
// opaque to the planner. Env.Now overrides the clock, for tests.
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
// Env.Bindings holds values only. A binding shadows a built-in of the same
// name, as in the reference implementation, so binding "string" makes
// $string a value and $string(x) a T1006. $eval is available as the
// documentation defines it, sees the bindings and the enclosing scope, and
// its result is carried like any other. Tail calls are eliminated as the
// documentation describes and do not count against MaxRecursion. $now,
// $fromMillis, and $toMillis use UTC unless the documented timezone argument
// (±HHMM) or the parsed text supplies an offset; the host's local zone is
// never consulted.
//
// # Conformance
//
// This package implements JSONata Version as defined by the documentation at
// https://github.com/jsonata-js/jsonata/tree/5d1473277e0022d8580e00f891b12080eb3edd74/website/versioned_docs
// (DocumentationCommit). Where its behavior departs from the reference
// implementation's observable behavior, the departure is recorded with its
// reason in DIVERGENCES.md beside this package.
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
// meaningful only as the root input; nested anywhere it is foreign.
var NoInput any = noInput{}

type noInput struct{}

// Limits bounds what a compiled expression may cost. A zero field means the
// default for that field; a nil *Limits means all defaults. Limits are part
// of the compiled Expression and are read back with Expression.Limits, so a
// cache of compiled expressions can key on them; Limits is comparable and
// will stay so. A carried value counts as one node however large it is, and
// its bytes are not charged.
type Limits struct {
	// MaxExpressionBytes bounds the length of an expression in UTF-8 bytes,
	// applied separately to each source passed to $eval. Default 256 KiB.
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
	// Error values, including intermediates and values a Resolver returns.
	// Default 64 MiB.
	MaxBytes int
	// MaxIntegerBits bounds the magnitude of a *big.Int result. Default 4096.
	MaxIntegerBits int
}

// DefaultLimits returns the defaults.
func DefaultLimits() Limits { panic("unimplemented") }

// Env is the value-model side of an evaluation: what variables are bound,
// how foreign values are read, how bytes present as strings, and what time
// it is. A nil *Env and a zero Env are the same: no bindings, no Resolver, no
// BytesEncoding, and the real clock. An Env is read-only during evaluation
// and may be shared by any number of concurrent evaluations.
type Env struct {
	// Bindings supplies variables. Names omit the leading "$" and may not
	// begin with one; a name that does is an error (CodeBinding) from
	// Prepare, or from Eval when Eval is called directly. Values only; a
	// function cannot be bound. The map is borrowed, not copied, and its
	// values may appear in results by identity.
	Bindings map[string]any
	// Resolver reads values outside the admitted set. Nil means none.
	Resolver Resolver
	// BytesEncoding presents []byte as a string. Nil means none, and an
	// expression that observes a []byte's content then fails
	// (CodeUnsupportedValue). *base64.Encoding values, including
	// base64.StdEncoding and URLEncoding, satisfy the interface unchanged.
	BytesEncoding BytesEncoding
	// Now supplies the timestamp $now and $millis observe. Nil means
	// time.Now. It is called once per Eval or Prepare; the instant it
	// returns is rendered in UTC whatever its location.
	Now func() time.Time
}

// Compile parses and prepares an expression under limits; nil means
// DefaultLimits. The returned Expression is immutable and safe for
// concurrent use; callers evaluating many inputs against one expression
// compile it once. Caching compiled expressions is the caller's concern; a
// cache keyed by the source and the Limits is sufficient. A failure is an
// *Error of ClassSyntax with its position.
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

// Unmarshal decodes JSON text into admitted values: integral numbers as
// int64 or *big.Int, other numbers as float64, strings as string, objects as
// *Object in member order, arrays as []any. Surrounding whitespace is
// allowed and a top-level scalar is a complete value. A text that is not one
// complete JSON value, contains a duplicate member name within an object, or
// contains an escape for an unpaired surrogate is an error
// (CodeMalformedInput) whose Offset is into data.
func Unmarshal(data []byte) (any, error) { panic("unimplemented") }

// Marshal encodes a result as JSON text: *Object in member order, a map in
// sorted key order, json.Number as its token, *big.Int as digits, float64 as
// $string renders it, a float32 as its float64 widening, []byte through
// env's BytesEncoding, no HTML escaping, and U+2028 and U+2029 unescaped.
// A foreign value in v, or a []byte when env has no BytesEncoding, is an
// error (CodeUnsupportedValue); Materialize resolves foreign values first.
// env may be nil.
func Marshal(v any, env *Env) ([]byte, error) { panic("unimplemented") }

// NewEncoder returns an Encoder that writes results to w as Marshal would.
// An Encoder may be used for any number of Encode calls.
func NewEncoder(w io.Writer, env *Env) *Encoder { panic("unimplemented") }

// Encoder writes results as JSON text.
type Encoder struct{ _ struct{} }

// Encode writes v followed by a newline. On an error partial output may have
// been written; a caller who must not emit a partial value uses Marshal and
// writes the bytes itself.
func (e *Encoder) Encode(v any) error { panic("unimplemented") }

// Materialize returns v with every foreign value it contains resolved
// through env's Resolver into admitted values, recursively, so the result
// contains no value that would need a Resolver to read: an ObjectView
// becomes an *Object and an ArrayView a []any. It is for callers handing a
// result to typed code. A value containing no foreign value is returned by
// identity; an admitted container that contains one is returned as a new
// container of the same kind with the resolved members. The walk is bounded
// by limits (nil means DefaultLimits) as an evaluation would be, on a
// budget of its own.
func Materialize(ctx context.Context, v any, env *Env, limits *Limits) (any, error) {
	panic("unimplemented")
}

// Member returns the member named key of an object result, whether it is a
// map[string]any, an *Object, or a map[string]T of admitted T. ok is false
// when v is not an object or has no such member.
func Member(v any, key string) (value any, ok bool) { panic("unimplemented") }

// Expression is a compiled JSONata expression.
type Expression struct{ _ struct{} }

// String returns the expression source verbatim, satisfying fmt.Stringer as
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
// computed key, $keys, $each, $spread, $eval, or a reference to a name that
// is not bound in the expression itself (an Env binding) makes known false,
// and paths is then nil. The returned slices are fresh.
func (e *Expression) Reads() (paths [][]string, known bool) { panic("unimplemented") }

// Eval evaluates the whole expression over input; env may be nil.
func (e *Expression) Eval(ctx context.Context, input any, env *Env) (value any, present bool, err error) {
	panic("unimplemented")
}

// EvalJSON is Eval over Unmarshal(input), then Materialize through env, then
// Marshal; env may be nil. An absent result is out == nil, present == false,
// and a nil error.
func (e *Expression) EvalJSON(ctx context.Context, input []byte, env *Env) (out []byte, present bool, err error) {
	panic("unimplemented")
}

// Prepare borrows input and env, fixes the timestamp $now and $millis
// observe, classifies the root (through the Resolver when it is foreign,
// charged to the budget), and plans selective evaluation without evaluating
// anything else; env may be nil. It fails for an invalid Env, for a root
// the Resolver rejects, and for ctx. Close the Evaluation when done.
func (e *Expression) Prepare(ctx context.Context, input any, env *Env) (*Evaluation, error) {
	panic("unimplemented")
}

// Evaluation is one expression over one input, from which fields of the
// result are selected individually. Selections share work: a field selected
// twice, or reached by two overlapping selections, is computed once, and a
// later Complete reuses every field already selected and returns the same
// values by identity. All selections and Complete draw on one budget. An
// Evaluation is safe for concurrent use: a Select of a field another
// goroutine is computing waits for it.
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
// already returned are no longer shared with it, and further Select or
// Complete calls return an error (CodeClosed); a call in flight on another
// goroutine completes. Returned values may still alias the input and
// Env.Bindings, as any result may, so they are the caller's to mutate only
// when it owns those. Close is idempotent. An Evaluation that is never
// closed is reclaimed by the garbage collector; Close makes the release
// prompt and the end of the borrow explicit.
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

// Fields iterates the selectable top-level keys when the plan is selective,
// and is empty otherwise.
func (p Plan) Fields() iter.Seq[string] { panic("unimplemented") }

// Reason explains why a Plan is not selective. Which functions the planner
// qualifies as selectable is this package's, may grow between versions, and
// never changes a value.
type Reason uint8

const (
	// ReasonNone: fields are evaluated individually.
	ReasonNone Reason = iota
	// ReasonDynamicShape: the result is not an object constructor with
	// literal keys.
	ReasonDynamicShape
	// ReasonOpaque: a field's subexpression calls $random or $eval, whose
	// result the planner cannot relate to a complete evaluation.
	ReasonOpaque
	// ReasonUnsupportedCall: a field's subexpression calls a function the
	// planner has not qualified as selectable.
	ReasonUnsupportedCall
	// ReasonArrayInput: the input is an array, so the constructor groups over
	// it and each field's value is evaluated over the whole input.
	ReasonArrayInput
	// ReasonPlanBudget: the expression exceeded the planner's work bound.
	ReasonPlanBudget
)

// String returns the reason's name: "None", "DynamicShape", "Opaque",
// "UnsupportedCall", "ArrayInput", or "PlanBudget".
func (r Reason) String() string { panic("unimplemented") }

// Object is an insertion-ordered object: the type of every object the
// expression constructs, and a valid input where key order matters. The zero
// Object is empty and ready to use. Object is not safe for concurrent
// mutation. Get is O(1); Delete is O(n). A key that is not valid UTF-8 is
// stored as given and refused (CodeUnsupportedValue) when an evaluation
// reads the object.
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

// MarshalJSON encodes the object with members in insertion order, as Marshal
// would with a nil Env.
func (o *Object) MarshalJSON() ([]byte, error) { panic("unimplemented") }

// UnmarshalJSON replaces the object's members with those of a JSON object,
// preserving order, with values as Unmarshal decodes them. A duplicate
// member name is an error.
func (o *Object) UnmarshalJSON(data []byte) error { panic("unimplemented") }

// Resolver resolves a value whose Go type is outside the admitted set, such
// as a protocol buffer message or an application struct, into a value the
// engine can read. Resolve returns either an admitted value or a value
// implementing ObjectView or ArrayView; anything else is an error
// (CodeUnsupportedValue). An error Resolve returns is passed to the caller
// wrapped in an *Error (CodeResolver), so errors.Is and errors.As see the
// cause; ctx's own error is returned as ctx.Err(). The engine calls Resolve
// once per read of a foreign value and charges each call as work and the
// bytes it returns to MaxBytes. Members and elements a view returns may
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
// in the object's order, and every order-observing operation observes that
// order. Get and Range must agree on membership; the engine trusts both.
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

// Code is an error code: the language's where the documentation or the
// reference implementation defines one for the situation (S0201, T2001,
// D1001, D3030, ...), or an engine code. A language code is raised only in a
// situation the language defines for it; every refusal this package adds is
// an E code, so Class() == ClassEngine identifies behavior another member of
// the class might not share.
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
	CodeUnsupportedValue Code = "E1001" // a value the engine cannot read: foreign with no Resolver, unresolvable, bytes with no BytesEncoding, or a function leaving an evaluation
	CodeBudget           Code = "E1002" // a Limits bound was exceeded
	CodeBinding          Code = "E1003" // an invalid binding name
	CodeClosed           Code = "E1004" // Select or Complete after Close
	CodeInternal         Code = "E1005" // an engine defect, recovered
	CodeMalformedInput   Code = "E1006" // input text is not one complete JSON value, has a duplicate member name, or an unpaired surrogate escape
	CodeResolver         Code = "E1007" // the Resolver returned an error; the cause is wrapped
	CodeInexact          Code = "E1008" // an integer operand could not convert to float64 exactly
)

// Sentinels for errors.Is. An *Error matches the sentinel with its Code.
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
// Error() renders Code, Message, and Offset only. Message never includes
// content derived from the input or the bindings; the offending value, for
// errors the language defines as carrying one (such as $error's argument,
// which is in Value and not in Message), is in Value only, and is charged to
// the output and byte bounds. A logger that walks the struct will see Value.
// Token is at most 64 characters. An unsupported-value error names the Go
// type, never its contents.
type Error struct {
	Code    Code
	Message string
	// Offset is the byte offset of the offending token into the expression
	// source, or into the input text for CodeMalformedInput, or -1 when
	// unknown, as json.SyntaxError.Offset and go/token report positions.
	// Line and Column are 1-based, Column in bytes, or 0.
	Offset, Line, Column int
	// Token is the offending token, when known.
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
