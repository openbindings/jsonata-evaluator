# openbindings/jsonata/go: public API

> **Draft 1 for review, 2026-09-18.** A greenfield public API for the Go
> member of the [evaluator class](../contract/HOST-NATIVE-EVALUATORS.md),
> satisfying [REQUIREMENTS.md](REQUIREMENTS.md). The sketch below is a
> compilable stub package: every exported signature and doc comment is as it
> would ship, bodies are `panic("unimplemented")`, and it passes `go vet` and
> `gofmt`, including a compile-time assertion that `*base64.Encoding`
> satisfies `BytesEncoding`. Nothing here is implemented.

## Principles

1. **Shaped like the standard library.** `Compile` / `MustCompile` and an
   immutable, concurrent-safe `*Expression`, as `regexp`. `Prepare` returning
   a handle that must be closed, as `database/sql`. Functional options.
   Cancellation and deadlines only through `context.Context`.
2. **Go values in, Go values out.** The admitted set is stated once in the
   package doc. Nothing is copied on admission. Nothing is serialized.
3. **Absence is a boolean, not a sentinel and not an error.** Every evaluation
   returns `(value, present, err)`. A present `nil` is null.
4. **Selection is a mode you can inspect, not a failure you can hit.** A plan
   that can't select falls back to complete evaluation and says why.
5. **One error type, two code namespaces.** The language's codes where the
   documentation defines them; engine codes only for refusals the language
   leaves to the implementation.
6. **Foreign values through an interface, never through reflection in the
   core.** `Access` is consulted only for types outside the admitted set.
7. **No second way to do anything.** No text executor as the boundary, no
   streaming evaluator, no custom functions, no `EvalBytes`/`EvalMap`
   variants. A `text` subpackage exists for callers holding JSON bytes and
   is documented as a convenience.

## The sketch

`jsonata.go`:

```go
// Package jsonata evaluates JSONata 2.1 expressions over Go values.
//
// It is the Go member of the OpenBindings evaluator class
// (contract/HOST-NATIVE-EVALUATORS.md): the pinned JSONata documentation
// defines what an expression does; Go defines what the values are and how
// they combine. Inputs and outputs are ordinary Go values. No JSON text is
// read or written on the evaluation path; see the text subpackage for a
// convenience over JSON bytes.
//
// # Values
//
// An input is any of: nil, bool, string, []byte, json.Number, int, int8,
// int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32,
// float64, *big.Int, []any, map[string]any, or *Object. Any other value is
// reached through an Access supplied with WithAccess. Values are read by
// reference and are never copied on admission.
//
// An output is a value of the same kinds. A value the expression only
// selected, copied, or rearranged is returned as the value that was carried:
// same type, same identity. An object the expression constructed is an
// *Object, which preserves insertion order as the language requires.
//
// Numbers combine with Go's arithmetic. Integer with integer stays integer
// and overflow is an error; any float64 operand yields float64; *big.Int
// stays big; "/" yields an integer when the division is exact and float64
// otherwise; a non-finite result is an error. Equality, ordering, membership,
// sort keys, and $type are decided by value across every admitted numeric
// type, never by Go type.
//
// # Absence
//
// The language distinguishes an absent result (undefined) from the value
// null. Eval, Select and Complete return (value, present, err): present is
// false for an absent result, and a present nil is null.
//
// # Whole and selective evaluation
//
// Eval evaluates the whole expression. Prepare returns an Evaluation from
// which fields of the result are selected individually: when the result is an
// object constructor with literal keys and the selected field's subexpression
// is pure, only that field is computed. A selected field is always identical
// to the same field of a complete evaluation. Evaluation.Plan reports which
// mode applies and, when selection is unavailable, why.
package jsonata

import (
	"context"
)

// Compile parses and prepares an expression. The returned Expression is
// immutable and safe for concurrent use; callers evaluating many inputs
// against one expression compile it once.
func Compile(expression string, opts ...CompileOption) (*Expression, error) { panic("unimplemented") }

// MustCompile is Compile for an expression fixed at build time. It panics on
// error, as regexp.MustCompile does.
func MustCompile(expression string, opts ...CompileOption) *Expression { panic("unimplemented") }

// Eval compiles and evaluates expression once. It is a convenience for
// one-off use; repeated evaluation compiles once with Compile.
func Eval(ctx context.Context, expression string, input any, opts ...EvalOption) (value any, present bool, err error) {
	panic("unimplemented")
}

// Expression is a compiled JSONata expression.
type Expression struct{ _ struct{} }

// Source returns the expression text as compiled.
func (e *Expression) Source() string { panic("unimplemented") }

// Reads reports the top-level input paths the expression reads when that is
// statically known, and known == false otherwise. A caller that fetches its
// input lazily can use it to fetch only what will be read.
func (e *Expression) Reads() (paths []string, known bool) { panic("unimplemented") }

// Eval evaluates the whole expression over input. Cancellation and deadlines
// come from ctx and are honored cooperatively at compile, evaluation, and
// function boundaries.
func (e *Expression) Eval(ctx context.Context, input any, opts ...EvalOption) (value any, present bool, err error) {
	panic("unimplemented")
}

// Prepare captures input and options and plans selective evaluation without
// evaluating anything. It fails only for invalid options, such as a binding
// name beginning with "$". The Evaluation must be closed.
func (e *Expression) Prepare(input any, opts ...EvalOption) (*Evaluation, error) {
	panic("unimplemented")
}

// Evaluation is one expression over one input, from which fields of the
// result are selected individually. Selections share work: a field selected
// twice, or reached by two overlapping selections, is computed once, and a
// later Complete reuses every field already selected. An Evaluation is safe
// for concurrent use.
type Evaluation struct{ _ struct{} }

// Select returns the field of the result at path, where each element is a
// literal key of an object constructor in the expression. When the plan
// permits, only that field's subexpression is evaluated; otherwise the whole
// expression is evaluated once and the field is looked up. A path that
// descends below constructor granularity is a structural lookup within the
// deepest selected field. The result is identical to the same field of
// Complete.
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

// Close releases the captured input and memoized fields. Values already
// returned remain valid.
func (v *Evaluation) Close() error { panic("unimplemented") }

// Plan describes how an Evaluation serves selections.
type Plan struct {
	// Mode is Selective when fields are evaluated individually and Complete
	// when every Select evaluates the whole expression once and looks up.
	Mode Mode
	// Reason explains a Complete mode: "dynamic-shape", "unsupported-effects",
	// "unsupported-call", "array-input", or "plan-budget". Empty when
	// Selective.
	Reason string
	// Fields lists the selectable top-level keys when Mode is Selective.
	Fields []string
}

// Mode is a Plan's evaluation mode.
type Mode uint8

const (
	// Selective means fields are evaluated individually on demand.
	Selective Mode = iota + 1
	// Complete means the whole expression is evaluated once and fields are
	// looked up in the result.
	Complete
)

// Object is an insertion-ordered object: the type of every object the
// expression constructs, and a valid input where key order matters. It is
// not safe for concurrent mutation.
type Object struct{ _ struct{} }

// NewObject returns an empty Object.
func NewObject() *Object { panic("unimplemented") }

// Len returns the number of members.
func (o *Object) Len() int { panic("unimplemented") }

// Get returns the member named key.
func (o *Object) Get(key string) (value any, ok bool) { panic("unimplemented") }

// Set appends a new member or replaces an existing one in place.
func (o *Object) Set(key string, value any) { panic("unimplemented") }

// Delete removes a member.
func (o *Object) Delete(key string) { panic("unimplemented") }

// Keys returns the member names in insertion order.
func (o *Object) Keys() []string { panic("unimplemented") }

// Range calls fn for each member in order until fn returns false.
func (o *Object) Range(fn func(key string, value any) bool) { panic("unimplemented") }

// Map returns the members as a Go map. Order is not preserved.
func (o *Object) Map() map[string]any { panic("unimplemented") }

// MarshalJSON encodes the object with members in insertion order.
func (o *Object) MarshalJSON() ([]byte, error) { panic("unimplemented") }

// Access reads values whose Go type is outside the admitted set, such as
// protocol buffer messages or application structs. The engine consults it
// only for such values; admitted values never pass through it.
type Access interface {
	// Kind classifies v. KindUnknown makes v an unsupported value.
	Kind(v any) Kind
	// Get returns the member of an object named key.
	Get(v any, key string) (value any, ok bool)
	// Keys returns an object's member names in order.
	Keys(v any) []string
	// Len returns an array's length.
	Len(v any) int
	// Index returns element i of an array.
	Index(v any, i int) (value any, ok bool)
	// Number returns a number as one of the admitted numeric types, exactly.
	Number(v any) any
	// String returns a string's content.
	String(v any) string
	// Bool returns a boolean.
	Bool(v any) bool
	// Bytes returns bytes, by reference.
	Bytes(v any) []byte
}

// Kind classifies a value for Access.
type Kind uint8

const (
	KindUnknown Kind = iota
	KindNull
	KindBool
	KindNumber
	KindString
	KindBytes
	KindArray
	KindObject
)

// CompileOption configures Compile.
type CompileOption func(*compileConfig)

type compileConfig struct{ _ struct{} }

// WithMaxExpressionBytes bounds the length of an expression. The default is
// 256 KiB.
func WithMaxExpressionBytes(n int) CompileOption { panic("unimplemented") }

// WithMaxDepth bounds the nesting depth of a parsed expression.
func WithMaxDepth(n int) CompileOption { panic("unimplemented") }

// EvalOption configures Eval and Prepare.
type EvalOption func(*evalConfig)

type evalConfig struct{ _ struct{} }

// WithBindings supplies variables. Names omit the leading "$" and may not
// begin with one.
func WithBindings(vars map[string]any) EvalOption { panic("unimplemented") }

// WithAccess supplies an Access for values outside the admitted set.
func WithAccess(a Access) EvalOption { panic("unimplemented") }

// WithBytesEncoding sets how []byte presents as a string when an expression
// uses it as one. The default is base64.StdEncoding; base64.URLEncoding and
// the Raw variants satisfy BytesEncoding unchanged. Pure copy never encodes.
func WithBytesEncoding(enc BytesEncoding) EvalOption { panic("unimplemented") }

// BytesEncoding is satisfied by *encoding/base64.Encoding.
type BytesEncoding interface{ EncodeToString(src []byte) string }

// WithMaxOutputNodes bounds the size of a result, counting every value.
func WithMaxOutputNodes(n int) EvalOption { panic("unimplemented") }

// WithMaxRecursion bounds evaluation depth, including function recursion.
func WithMaxRecursion(n int) EvalOption { panic("unimplemented") }

// Error is a compilation or evaluation failure. Use errors.As.
type Error struct {
	// Code is the language's error code where the documentation defines one
	// (S0201, T2001, D3137, ...) or an engine code for refusals the language
	// leaves to the implementation.
	Code string
	// Message is the failure in words.
	Message string
	// Position is the byte offset into the expression source, or -1.
	Position int
	// Token is the offending token, when known.
	Token string
}

// Error implements error.
func (e *Error) Error() string { panic("unimplemented") }

// Engine codes for refusals the language leaves to the implementation. A
// non-finite arithmetic result uses the language's own D1001.
const (
	CodeOverflow         = "E1001" // integer arithmetic exceeded int64 or uint64
	CodeUnsupportedValue = "E1002" // a value outside the admitted set with no Access
	CodeBudget           = "E1003" // an evaluation bound was exceeded
	CodeBinding          = "E1004" // an invalid binding name
)
```

`text/text.go`:

```go
// Package text evaluates JSONata expressions over JSON text. It is a
// convenience over package jsonata for callers that already hold JSON bytes:
// input is decoded with exact number tokens, evaluated over the resulting Go
// values, and the result encoded. It adds nothing the native API lacks and
// is not the evaluation boundary.
package text

import (
	"context"

	jsonata "github.com/openbindings/jsonata/go"
)

// Evaluate decodes inputJSON, evaluates expression over it, and encodes the
// result. An absent result encodes as no bytes with a nil error.
func Evaluate(ctx context.Context, expression string, inputJSON []byte, opts ...jsonata.EvalOption) ([]byte, error) {
	panic("unimplemented")
}
```

## Usage

Whole evaluation:

```go
expr := jsonata.MustCompile(`{ "id": task_id, "title": task_title, "done": is_done }`)
out, ok, err := expr.Eval(ctx, input)
switch {
case err != nil:
    return err
case !ok:
    // absent result (JSONata undefined)
case out == nil:
    // null
}
```

Selective evaluation:

```go
ev, err := expr.Prepare(input, jsonata.WithBindings(vars))
if err != nil {
    return err
}
defer ev.Close()

if p := ev.Plan(); p.Mode == jsonata.Complete {
    log.Printf("selection unavailable: %s", p.Reason)
}
id, ok, err := ev.Select(ctx, "id")          // only the "id" subexpression runs
title, ok, err := ev.Select(ctx, "title")    // shares nothing with "id", computes "title"
all, ok, err := ev.Complete(ctx)             // reuses "id" and "title", computes the rest
```

Bytes from a binding that declares base64url:

```go
out, ok, err := expr.Eval(ctx, body, jsonata.WithBytesEncoding(base64.URLEncoding))
```

A protobuf message without conversion:

```go
out, ok, err := expr.Eval(ctx, msg, jsonata.WithAccess(protoaccess.Reflect{}))
```

Errors:

```go
var jerr *jsonata.Error
if errors.As(err, &jerr) {
    switch jerr.Code {
    case jsonata.CodeOverflow:
        // integer arithmetic exceeded int64/uint64
    case "T2001":
        // language error: operand type
    }
}
if errors.Is(err, context.DeadlineExceeded) {
    // timed out
}
```

## Decisions and the alternatives rejected

| Decision | Why | Rejected |
| --- | --- | --- |
| `Compile` + immutable `*Expression`; no executor object | `regexp`'s shape. Caching is the caller's (the SDK holds one per binding). One fewer type with lifetime. | An `Executor` holding a bounded compile cache and default limits (today's shape). Useful, but it's a cache, and caches belong to callers. Can be added later as a separate type without changing anything here. |
| `(value, present, err)` | Comma-ok is how Go says "absent". Undefined is a successful evaluation with no value, so it cannot be an error. | A sentinel `Undefined` value (leaks into data structures). `ErrUndefined` (conflates success with failure). A `Result` struct (a new type for one bit). |
| `Prepare` takes no `ctx` | It plans over the compiled AST, bounded and fast, and evaluates nothing. Only `Select` and `Complete` do work that can be cancelled. | `Prepare(ctx, …)` for symmetry. Would imply work that isn't there. |
| `Select(ctx, path ...string)` | The unit of selection is a literal key of an object constructor. Array indices are never selectable units, only lookups into an already-computed value, which the caller can do. Typed, no runtime type check on path elements. | `path ...any` accepting ints, as the JS engine does. Looser, and it invites the caller to think indices are evaluated lazily when they aren't. |
| `Plan` is a value from a method, not an error | Fallback to complete evaluation is normal and correct. It is reported, never raised. | Returning an error from `Prepare` when selection is unavailable. |
| `*Object` exported, with `NewObject`/`Set` | Constructed objects need insertion order; callers building input where `$keys` order matters need the same type. `Map()` is named for what it loses. | An interface so the SDK could supply its own ordered type. More surface, no caller has asked. |
| `Access` only for non-admitted types | Native values never pay an interface call. Exactness is by construction: `Number` returns an admitted numeric, so the engine's seams handle it like any other. | Reflection in the core. Slower for everyone and reaches into structs the caller may not intend to expose. A reflection-based `Access` can live in its own package. |
| `BytesEncoding` as `interface{ EncodeToString([]byte) string }` | `*base64.Encoding` satisfies it unchanged, so the binding adapter passes `base64.URLEncoding` and no new type exists. Encode-only, because bytes flow in and pass through; the language never constructs bytes. | An enum of encodings (a new type, and a closed list). |
| Two option types | Compile-time and evaluation-time settings can't be confused at the call site; the compiler rejects `Compile(expr, WithBindings(v))`. | One `Option` type, or an options struct. |
| Timeouts only via `ctx` | Go idiom. A `WithTimeout` option would be a second way to say the same thing. | `WithTimeout(d)`. |
| `Error` with `Code`, `Position`, `Token`; `errors.As` | One type to match on. Language codes where the documentation has them (`T2001`, `D3137`); `E1xxx` only for overflow, unsupported value, budget, and bad binding, which the language leaves open. A non-finite result reuses the language's `D1001`. | Typed error variables per condition (dozens of them). Wrapping language errors in engine errors (two things to match). |
| `Reads()` | Cheap introspection the fast-path analyzer already computes. A binding that can fetch its input lazily wants it. Honest about when it doesn't know. | Omitting it (loses a real optimization for no gain). |
| Package-level `Eval` | `regexp.MatchString` precedent for one-off use in tests and scripts. | Omitting it (callers write the two-liner anyway). |
| `text` subpackage | Callers holding JSON bytes get one function. Separated so nobody mistakes it for the boundary. | Keeping text on the main type (today's shape). |

## What this removes from today's module

The text executor as the public boundary (`executor.go`, `json_executor.go`,
`json_admission.go`), `StreamEvaluator` and its options, custom-function
environments (`CustomFunc`, `NewCustomEnv`, `EvalWithCustomFuncs`), the
`EvalBytes`/`EvalMap`/`EvalBytesWithVars` variants, the `JSONExecutorOptions`
struct, and `NumericWorkLimits` (no exact-decimal policy to bound).

The `syntax` package and its `Validate` stay exactly as they are: syntax-only,
no evaluator loaded, for OBI-D-18.

## Open questions

1. **`Select` path type.** `...string` (constructor keys only) as sketched, or
   `...any` admitting indices for structural lookup convenience?
2. **`Evaluation` concurrency.** Sketched as safe for concurrent use (a mutex
   around the memo). The alternative is single-goroutine with a documented
   rule, which is cheaper and matches how the SDK will call it.
3. **`Access` scope.** Per-call via `WithAccess` as sketched, or also a
   package-level registration for a Go type (register once for
   `protoreflect.Message`). Per-call keeps the closed environment visible at
   every call site; a registry is more convenient and easier to forget.
4. **Package-level `Eval`.** Keep, or drop to discourage recompiling in a loop.
5. **Naming.** `Evaluation` for the prepared handle, or `Prepared`.
6. **A compile cache.** Omitted here as the caller's concern. If `ob` wants
   one, `jsonata.Cache` as a separate additive type, or leave it to the SDK.
