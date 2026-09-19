# Iteration-5 panel: Go idiom purist

Cold read of `7f8b4fe`. Lens: Go standard-library idiom and API design.

## Grades

| Criterion | Grade | One-line justification |
|---|---|---|
| Idiomatic fit for Go | B | Context-first, `errors.Is`/`As`, `iter.Seq`, `MustCompile`, `Stringer`, useful zero values, and no goroutines are all right; the `(value, present, err)` triple, a `Close` that releases nothing, a pointer to an already-zero-usable `Limits`, and Rust vocabulary ("borrow", "foreign", "carriage") are not. |
| Ergonomics of the common path | B- | Compile, Unmarshal, Eval, check `present`, Marshal is five steps with two `nil`s and a three-value return, and the result can be a `map[string]any` or an `*Object` depending on whether the expression built it, so every consumer needs `Member` or a two-arm switch. |
| Correctness and footgun risk | B- | The package documents its own footgun (`v, _, err :=` turns absent into null) instead of designing it away; `json.RawMessage` re-decodes on every read; decimal tokens are rounded at `Unmarshal`, which quietly breaks the class's own "carriage is exact" rule; `Select` is or is not a guard depending on `Plan`. |
| Performance headroom the API permits | A- | Compile-once immutability, by-reference values, `*Object` O(1) get, lazy views, `Reads` for fetch narrowing, selective evaluation with memoization, budgets charged before allocation; nothing in the surface forces a slow engine, and the only structural tax is `any` boxing, which is the stated value model. |
| Concept soundness | B- | Documentation-as-authority is the right stance and the divergence ledger is the right discipline, but "the host defines what a number is" is not what this package does: it defines its own numeric tower (integrality promotion, `1e23 + 1` exact) that neither Go nor JavaScript would produce. |
| Overall | B | A serious, well-reasoned design with a security posture the stdlib would respect, carrying more surface and more coined vocabulary than a Go reviewer would let through, and one semantic hole (decimal carriage) that contradicts the README. |

## What I would change

### 1. Replace `(value any, present bool, err error)` with `(any, error)` and one `Undefined` sentinel in both directions

The input side already has a sentinel value for "no value" (`NoInput`); the output side uses a bool. That is the same JSONata concept, `undefined`, expressed two ways in one package. Pick the sentinel, use it on both sides, and delete `present` from `Eval`, `EvalJSON`, `Select`, `Complete`, and the package-level `Eval`:

```go
// Undefined is the absent value. As an input, $ is absent. As a result, the
// expression produced no value, which is distinct from null (nil).
var Undefined any = undefined{}

func (e *Expression) Eval(ctx context.Context, input any, env *Env) (any, error)
func (e *Expression) EvalJSON(ctx context.Context, input []byte, env *Env) ([]byte, error) // nil, nil for absent
func (v *Evaluation) Select(ctx context.Context, path ...string) (any, error)
```

Why: the triple has no stdlib precedent (comma-ok never carries an error alongside it), and the doc comment concedes callers will drop `present`. With a sentinel, forgetting the check is loud, not silent: `Marshal(Undefined)` errors, `out.(string)` fails, `Member(out, k)` reports not-ok. The caller who wants absent to be null writes `if out == jsonata.Undefined { out = nil }`, one explicit line. `io.EOF` is the precedent for an exported sentinel `var`. `EvalJSON` already distinguishes absent by `out == nil` (null encodes as the bytes `null`, never a nil slice), so `present` there is pure redundancy today.

### 2. Make `Limits` a value with methods, on the `net.Dialer` / `http.Client` pattern

The doc says a zero `Limits` means all defaults, then takes `*Limits` so callers can pass `nil` to mean the same thing. When the zero value is already useful, the pointer buys nothing except a second spelling of "default". Go's answer to "package function with defaults, method for configured use" is the config-struct-with-methods pattern (`net.Dial` vs `(*net.Dialer).DialContext`, `http.Get` vs `(*http.Client).Get`):

```go
func Compile(expr string) (*Expression, error)          // zero Limits
func MustCompile(expr string) *Expression
func Unmarshal(data []byte) (any, error)
func Eval(ctx context.Context, expr string, input any, env *Env) (any, error)

func (l Limits) Compile(expr string) (*Expression, error)
func (l Limits) MustCompile(expr string) *Expression
func (l Limits) Unmarshal(data []byte) (any, error)
func (l Limits) Eval(ctx context.Context, expr string, input any, env *Env) (any, error)
func (l Limits) Materialize(ctx context.Context, v any, env *Env) (any, error)
```

This fixes three things at once: every example stops reading `Compile(s, nil)`; `MustCompile` becomes the one-argument form it should be for package-level vars; and `Unmarshal`, which is the ingestion point for untrusted payloads, becomes tunable (today its `MaxDepth` and `MaxBytes` are frozen at `DefaultLimits()` unless you go through `EvalJSON`). Keep `Limits` comparable and keep `Expression.Limits()` returning the filled-in value; `DefaultLimits()` can stay as the filled-in form for cache keys.

### 3. Stop rounding decimal tokens at `Unmarshal`; carry them exactly

README rule 4 says a value an expression only selects arrives unchanged, "same numeric value". The package doc then says a decimal token "becomes its nearest float64 when it is decoded", so `{"amount": 0.1000000000000000055511151231257827}` round-trips through `Unmarshal` and `$` and `Marshal` as `0.1`. Integers got `*big.Int` to honor rule 4; decimals got nothing. That is an asymmetry the README's own rule forbids, and it is exactly the kind of value a payload transform is supposed to leave alone.

`json.Number` is already admitted and is the type encoding/json built for this. The cheap fix that keeps the fast path:

```go
// Unmarshal decodes integer tokens as int64 or *big.Int, decimal tokens as
// float64 when the shortest float64 rendering reproduces the token's value,
// and otherwise as json.Number so that the token is carried exactly.
```

`0.5`, `2.675`, `1e2` all become `float64` (their shortest rendering is value-identical). Only tokens a float64 cannot carry stay as `json.Number`, are classified on first arithmetic read as the doc already specifies for `json.Number`, and are rendered by `Marshal` "as its token", which the doc already promises. Decoded input pays the classification cost only on the values that need it.

### 4. Delete `Evaluation.Close`, `CodeClosed`, `ErrClosed`, and the "borrow" vocabulary

`Close` "makes the release prompt" but the doc also says an unclosed `Evaluation` is simply garbage collected and an in-flight call completes normally. So `Close` releases no resource, cancels nothing, and its only observable effect is to make later calls fail with a code that exists only because `Close` exists. `bytes.Buffer`, `strings.Reader`, `json.Decoder`, and `regexp.Regexp` hold caller memory and have no `Close`; `sql.Rows` has one because a connection is behind it. If there is nothing behind it, do not ship it. Replace with one sentence on `Prepare`: "The input and env must not be modified while the Evaluation is in use." Go has no borrows; it has "must not be modified" and "may retain", and the stdlib phrasing for the guarantee is "safe for concurrent use by multiple goroutines". Use those words.

### 5. Tighten the selective surface: collapse `Plan`, unify `Fields`, narrow `Select`, rename `Prepare`

- `Plan` is a struct with one exported field, a `_ struct{}` to discourage literals, a `Selective()` method that is `Reason == ReasonNone`, and a `Fields()` returning `iter.Seq[string]` while `Expression.Fields()` returns `([]string, bool)`. Replace the type with `func (v *Evaluation) Reason() Reason` and `func (v *Evaluation) Fields() ([]string, bool)`, matching `Expression.Fields`. Two methods named `Fields` in one package with different shapes will be a permanent papercut.
- `Select(ctx)` with no path meaning `Complete` is two spellings of one operation. Require at least one key. The "descends below constructor granularity into member lookup" clause makes `Select` do two jobs; the second is `Member`'s job, and mixing them means a caller cannot tell from a result whether the planner served it or a lookup did. Restrict `path` to constructor keys.
- `Prepare` collides with `database/sql`, where `Prepare` binds the statement before any input exists. Here the input is the argument. `Begin`, or `NewEvaluation`, says what happens.

## What I would keep

- `ctx` first everywhere that evaluates, `Compile` without one, cancellation returned as bare `ctx.Err()` so `errors.Is(err, context.Canceled)` holds. This matches `database/sql` and `net/http` and is what callers already write.
- `*Expression` immutable and safe for concurrent use, `MustCompile` panicking exactly as `regexp.MustCompile` does, `String()` returning the source verbatim. This is the `regexp` shape and it is the right one.
- Values by reference, no wrapper `Value` type, results are ordinary Go values. The moment this package introduces a `jsonata.Value` handle it becomes the intermediate tree the README says it refuses.
- `*Error` with `Code`, table-driven `Class`, sentinels satisfied through `Is`, `Unwrap` only for the resolver cause, and above all: `Message` never contains input content, with the offending value carried separately in `Value`. That is a better security property than most of the stdlib's own error types have.
- Limits attached to the compiled expression rather than to the call. A hostile expression compiled tight cannot be evaluated loose by a downstream caller who forgot. Finite defaults for every bound, and "raised before the allocation that would exceed it", are the correct contract for untrusted input.
- The closed environment with no function registration, `Resolver` as a single-method allowlist door that is explicitly told "never reflection over arbitrary structs", and `BytesEncoding` satisfied structurally by `*base64.Encoding`. Small interfaces, and the biggest one has two methods.
- `Object`: zero value ready, nil-safe reads, `Set` on nil panics like a nil map, `Keys`/`All` returning `iter.Seq`/`iter.Seq2` with the `maps` package's names, `MarshalJSON`/`UnmarshalJSON`. This is how a Go 1.23 ordered map should look.
- `Env.Now func() time.Time` (`tls.Config.Time` is the precedent), fixed once per evaluation so `$now` is pure under selection.
- Refuse rather than approximate (`CodeInexact`), exact integers via `*big.Int`, panics inside the engine recovered into `CodeInternal` (encoding/json does the same internally) while panics in caller code propagate.
- No goroutines, no process-wide state. Say it exactly that way in the package doc; it is a feature.
- `Reads()` as a static, sound-or-unknown fetch hint with fresh slices. Small, honest, useful.
- nil slice as empty array and nil map as empty object, even though `encoding/json` says `null`. The argument (Go code leaves collections unset, `$count(items) = 0` should hold) is correct for this package's job. It just needs to be stated on `Marshal`'s own doc, not only in the package overview, because `Marshal` is where a reader coming from `json.Marshal` will be surprised.

## Things the doc comments leave unclear

- **Method docs depend on package-doc vocabulary.** `Expression.Eval` says "a result that carries a foreign value" and a `go doc jsonata.Expression.Eval` reader has no definition of "carries" or "foreign". Every method doc must stand alone; define "carried" once where `Eval` is, and use "unsupported type" for "foreign".
- **Which regular-expression dialect does `Compile` accept for `/.../` literals?** The doc says literals are compiled at `Compile`; `DIVERGENCES.md` says the dialect is a pending ruling. A reader cannot know whether `(?<=x)`, backreferences, or `\p{Script=Greek}` compile, fail at `Compile`, or fail at evaluation. This is the single largest unanswered semantic question in the surface and it is user-visible on day one.
- **Is `Select` a guard or not?** Under a selective plan, a sibling field's `$error` does not fail `Select`; under a non-selective plan `Select` "evaluates the whole expression once", so it does. The doc says "Select is not a guard" without saying the behavior flips with `Plan`. Callers will discover the flip when a document changes shape.
- **Is `Marshal` bound by any `Limits`?** `Materialize` and `EvalJSON` say which bounds apply; `Marshal` and `Encoder.Encode` say nothing about `MaxBytes`, so a carried 10 GB `[]byte` is presumably encoded without a budget.
- **Why does `Marshal` take an `*Env`, and where is `NewDecoder`?** `Marshal` needs one field of `Env` (`BytesEncoding`); handing it the whole environment invites callers to wonder whether `Bindings` or `Resolver` affect encoding (the doc for `Marshal` says a foreign value is an error, so the answer is no, but the signature says maybe). And the package offers an `Encoder` for output streams but no `Decoder` for input streams, which is the common direction for "decoded API payloads".
- **`json.RawMessage` semantics.** "Decoded on each read, charged per byte" means an expression that touches a RawMessage field inside a predicate over N items decodes it N times. The doc tells the caller to pre-decode instead of the package memoizing per evaluation (or refusing the type). A reader cannot tell whether admitting it is a convenience or a trap; it is a trap.
- **What makes an `Env` invalid** beyond a binding name starting with `$`? `Prepare` "fails for an invalid Env" and nothing enumerates the conditions.
- **Zero and nil `*Expression` / `*Evaluation`.** `regexp` leaves the zero `Regexp` undefined too, but `Object` goes out of its way to be nil-safe, so a reader will expect a stated rule for the other two.
- **`Error()` format.** "Renders Code, Message, and Offset" but not `Line`/`Column`, and no example of the string. State whether the format is stable; the stdlib's position is that error strings are not.
- **What are `MaxWork` units?** "Nodes evaluated, elements materialized, source bytes parsed, calls into the Resolver, comparisons in a sort" are summed into one integer with default `8 << 20`. A caller cannot relate that to a payload size or an expression. Even one sentence of calibration ("a simple field selection over a 1 MB document costs roughly N") would make the knob usable.
- **`time.Time` and `json.Marshaler` implementers are foreign.** A Go program's structs are full of both; `encoding/json` handles them and this package errors without a `Resolver`. That is a defensible consequence of the closed environment but nothing in `Eval`'s doc warns the reader coming from `encoding/json`.
- **`float32` widening.** A carried `float32(0.1)` renders as `0.10000000149011612`. It is "exact" by the model and it is not what anyone who wrote `0.1` meant. Say it on `Marshal`, where it will bite.

## On the concept

The regex-engine framing buys the right permission and hides the hard part. It is correct that Go should write its own JSONata over its own values the way it wrote `regexp` over its own strings, and it is correct that "documentation is the authority, divergences are declared" is the only way to do that honestly. But `regexp` has no value model at all: one input type, one output type, and the whole difficulty is in the notation. JSONata's whole difficulty is the value model. The analogy therefore says nothing about numbers, strings, bytes, or member order, which is where every line of this package's contested design lives. And the analogy carries a warning the README does not hear: Go's `regexp` is famously not PCRE, that divergence is a well-known interop pain wherever regexes are exchanged as data (JSON Schema `pattern`, for instance), and OpenBindings transforms are precisely regexes-as-data, portable artifacts evaluated by whichever SDK a consumer has. The README's "portable core" is the correct response, but it is filed as "an observation, not a rule". `regexp` shipped RE2 as a stated, guaranteed subset. The regex analogy argues for making the portable core a stated subset with a conformance test, not a paragraph about what authors have happened to write so far.

Documentation-as-authority holds up because the commit is pinned, and only because of that. JSONata's documentation is prose and examples, not a grammar or a semantics; the package doc itself admits the docs catalogue no error codes, and `DIVERGENCES.md` already needs a fourth authority ("interpretation") for `$round`. So the real rule is a three-tier one: documentation where it speaks, the reference where the documentation is silent and the reference's behavior is not a JavaScript artifact, the value model otherwise. That is a sound rule. It should be written down as the rule, with the tiers named, rather than left implicit in a table's authority column, because every future divergence argument is really an argument about which tier a behavior belongs to.

"The host defines what a number is" is the claim I would push back on hardest, because this package does not do it. Go's `float64` says `1e23 + 1` is `1e23`; this package says it is `99999999999999991611393`. Go says `x / 100.0` is a float; this package says it is an exact integer when `x` divides. Go has no rule that a `float64` behaves as an integer when integral, no `CodeInexact` refusal for `9007199254740993 + 0.5`, and no promotion to `*big.Int`. What ships here is a designed numeric tower (exact integers of bounded width, binary decimals, integrality-based promotion, refusal on inexact operand conversion) that uses Go types as its representation. That tower is defensible and arguably excellent for moving IDs between systems, which is the job. But it is the package's, not the host's, and the doc should say so, because the framing predicts the wrong surprises: a Go author will not expect `1e23 + 1` to be exact, a JavaScript author will not expect `9007199254740993 + 0.5` to fail, and both will have been told the host decides. The place the analogy truly breaks is here: a regex engine can let the host own characters because characters are not the payload; a transform engine cannot let each host own numbers when numbers are the payload and the same expression must produce the same 64-bit ID in Go and in a JavaScript member that decoded it through `JSON.parse`. Class rule 4 (carriage is exact) is the right floor, and it quietly obligates every member, including a future JS one, to a decoder that is not the host's default. Say that out loud; it is the class's real commitment, and it is stronger and better than "the host defines what a number is".
