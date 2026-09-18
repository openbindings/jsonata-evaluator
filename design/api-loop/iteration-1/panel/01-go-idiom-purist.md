# Iteration 1 cold read: go idiom purist

> Given only the class README and go/jsonata + go/text at feb3743; told not to read design/. Verbatim.

## Grades

| Criterion | Grade | One-line justification |
|---|---|---|
| Idiomatic fit for Go | B+ | The `regexp` skeleton (Compile/MustCompile/String, package-level Eval), context-first signatures, `iter.Seq` on Object, a structural `BytesEncoding`, and `errors.As` are all right; functional options, a nine-method `Access`, a no-op `Close`, untyped string error codes, and a subpackage named `text` are the parts a core reviewer would send back. |
| Ergonomics of the common path | A- | `Compile` once, `Eval(ctx, input)` per request is exactly the shape people expect; the tax is the three-way return at every call site and the mandatory two-arm switch over `map[string]any` and `*Object` on output. |
| Correctness and footgun risk | B | Aliasing and the "do not mutate while running" window are documented but the window has no defined end; `Access` is asymmetric about failure; decimal tokens round silently while integers refuse; `text.Evaluate` can encode bytes differently from `$string` in the same expression. |
| Performance headroom the API permits | A- | By-reference admission, `Reads` for lazy fetch, memoized selective evaluation, and `Access` over foreign values without conversion leave the implementation room; `any` boxing of every scalar and the two-call `Kind`/`Get` dispatch are the ceiling. |
| Concept soundness | B | The engine-not-port framing is sound and the exact-carriage commitment is the right problem to solve; "the host defines what a number is" is not what the package actually does (it defines a three-representation numeric tower Go does not have), and documentation-as-authority is thinner than the README admits because JSONata's documentation is prose, not a spec. |
| Overall | B+ | A well-reasoned API with the right center of gravity and a handful of surface choices that are ecosystem-idiomatic rather than stdlib-idiomatic. |

## What I would change

**1. Replace both functional-option families with option structs, and make bounds inspectable.**

Current:
```go
func Compile(expression string, opts ...CompileOption) (*Expression, error)
func (e *Expression) Eval(ctx context.Context, input any, opts ...EvalOption) (any, bool, error)
func (e *Expression) With(opts ...EvalOption) *Expression
```

Proposed:
```go
// Limits bounds what a compiled expression may cost. The zero value means the
// package defaults; a nil *Limits does too.
type Limits struct {
    MaxExpressionBytes int // default 256 KiB
    MaxDepth           int // default 128
    MaxRecursion       int // default 1024
    MaxOutputNodes     int // default 1 << 20
    MaxWork            int // default 8 << 20
}

func Compile(expression string, limits *Limits) (*Expression, error)
func MustCompile(expression string, limits *Limits) *Expression
func (e *Expression) Limits() Limits

// Env is the value-model side of an evaluation. The zero value is usable.
type Env struct {
    Bindings      map[string]any
    Access        Access
    BytesEncoding BytesEncoding // nil means base64.StdEncoding
}

func (e *Expression) Eval(ctx context.Context, input any, env *Env) (any, bool, error)
func (e *Expression) Prepare(input any, env *Env) (*Evaluation, error)
```

Why: the standard library has never adopted `func(*config)` options and the most recent word on the question, `slog.NewTextHandler(w, *HandlerOptions)` with nil meaning defaults, went the other way deliberately. `net.Dialer`, `http.Client`, `tls.Config`, `cookiejar.Options`, `x509.VerifyOptions` are the same pattern. The concrete costs of the current shape are not stylistic: the package doc says bounds "travel with the compiled expression", yet there is no way to read them back off an `Expression`, so an operator cannot log or assert what budget a document-supplied transform is running under. A struct is a value you can store in a config, compare, marshal, and put in a test table; a `[]CompileOption` is none of those. `With` then disappears: a caller holding a long-lived `*Env` passes it, and the doc's open question (what happens when `WithAccess` is given both to `With` and again to `Eval`) does not exist. If you want to keep `With` for the `slog.Logger.With` precedent, its precedence over per-call options must be stated; today it is not.

**2. Slim `Access` to one method and let foreign values reveal themselves through small optional interfaces.**

Current: nine methods, every implementer must supply all of them, and only `Number` can fail.

Proposed:
```go
// Access resolves a value outside the admitted set to one inside it, or to a
// value implementing one of the view interfaces below. It is called
// concurrently and must be safe for that.
type Access interface {
    Resolve(v any) (any, error)
}

// Views a resolved value may implement instead of being converted.
type ObjectView interface {
    Get(key string) (any, bool)       // ok == false is absence, not null
    Members() iter.Seq2[string, any]
}
type ArrayView interface {
    Len() int
    At(i int) any
}
```

Why: this is how the standard library handles "your type, our representation" everywhere: `json.Marshaler`, `driver.Valuer`, `fmt.Stringer`, `fs.FS` plus optional `fs.ReadDirFS`, `io.Reader` plus optional `io.WriterTo`. One required method, capability by optional interface. It also fixes three defects in the current contract at once: `String`, `Bool`, `Bytes`, `Len`, `Index` cannot report failure while `Number` can (a protobuf `Any` that fails to unpack has no honest path); `Range` cannot stop on error; and the concurrency requirement is stated on `Evaluation`, not on `Access` where an implementer would look. Separately, ship a reflect-backed `Access` in the package (`jsonata.StructAccess` or similar) that follows `encoding/json`'s tag rules. A Go programmer's first call will be `Eval(ctx, "$.name", myStruct)`, and "CodeUnsupportedValue" is the wrong first experience for a library whose thesis is host-native values. `encoding/json` decided what a struct looks like as JSON a long time ago; reuse that decision.

**3. Type the error codes and make `errors.Is` work.**

Current: `Code string` on `Error`, engine codes as untyped `const` strings, no sentinels.

Proposed:
```go
type Code string

const (
    CodeUnsupportedValue Code = "E1001"
    CodeBudget           Code = "E1002"
    CodeBinding          Code = "E1003"
)

var (
    ErrUnsupportedValue = &Error{Code: CodeUnsupportedValue}
    ErrBudget           = &Error{Code: CodeBudget}
)

func (e *Error) Is(target error) bool // true when target is an *Error with the same Code

// Position is the byte offset of the offending token in the expression
// source, or -1.
```

Why: `regexp/syntax.Error{Code ErrorCode}` with typed constants is the direct precedent, and a typed `Code` lets `switch e.Code` be checked and lets `go doc` group the constants under the type. `fs.PathError` unwrapping to `fs.ErrNotExist` is the precedent for making `errors.Is(err, jsonata.ErrBudget)` work without an `errors.As` dance at every budget check, which is the check an OpenBindings runtime will perform most. "Position ... in characters" is not a unit Go has; `json.SyntaxError.Offset` and `go/token` are bytes. Say bytes, or say runes and explain why. Also fix `CodeBudget`'s comment: `MaxWork`, `MaxOutputNodes`, and `MaxRecursion` are evaluation-time bounds set at compile, not "compile-time bounds".

**4. Drop `Evaluation.Close`, or give it a job.**

Current: `Close()` releases references, an unclosed `Evaluation` is collected anyway, and nothing says what `Select` after `Close` does.

Why: in the standard library `Close` means an external resource: `os.File`, `sql.Rows`, `net.Conn`. `regexp.Regexp`, `template.Template`, `bytes.Buffer` have none, because dropping the pointer is how Go releases memory. A `Close` whose only effect is "makes the release prompt" trains callers to `defer ev.Close()` for nothing and leaves an undefined state behind it. If you keep it, it has to mean something: define it as the end of the "callers must not mutate the input while an evaluation is running" window (the doc currently gives that window no end for a long-lived `Evaluation`), state that `Select` and `Complete` after `Close` return an `*Error`, and return `error` per `io.Closer` so it composes with `errors.Join` in cleanup paths. Otherwise delete it. While here: `Prepare` reads as `database/sql`'s `Prepare`, which binds a statement for reuse across many argument sets, the opposite of what this does (it binds one input). `Bind` collides with bindings; `Begin` is honest about the memoizing, transaction-like object it returns.

**5. Fold package `text` into `jsonata` under `encoding/json` names, and remove the base64 inconsistency.**

Current: `text.Decode([]byte)`, `text.Evaluate(ctx, expr, inputJSON, opts...)`.

Proposed:
```go
// Unmarshal decodes JSON text into admitted values: numbers as json.Number,
// objects as *Object in member order, arrays as []any. Duplicate member
// names are an error.
func Unmarshal(data []byte) (any, error)

// EvalJSON is Eval over Unmarshal(input), with the result encoded by
// encoding/json. []byte is encoded with the Env's BytesEncoding.
func (e *Expression) EvalJSON(ctx context.Context, input []byte, env *Env) ([]byte, bool, error)
```

Why: the parent package already imports `encoding/json` (`json.Number`, `Object.MarshalJSON`), so the subpackage buys no dependency isolation, and `text` is a name with no information in it that also reads as a sibling of `text/template`. `Decode` on a `[]byte` contradicts `encoding/json`'s vocabulary, where `Unmarshal` takes bytes and `Decoder` takes a stream. `Evaluate` versus `Eval` is a verb split with no meaning. The substantive bug: `text.Evaluate` says bytes are encoded with `base64.StdEncoding`, while `WithBytesEncoding` may have made `$string(bytes)` inside the same expression produce URL encoding; the same value renders two ways in one output. Also state whether `SetEscapeHTML(false)` is applied; `json.Marshal`'s default escaping of `<`, `>`, `&` is a surprise for API payloads.

## What I would keep

- **The `regexp` skeleton.** `Compile`, `MustCompile` that panics, `String()` returning the source, "compile once, evaluate many, safe for concurrent use". A Go reader knows this contract before reading a line of doc.
- **`(value, present, err)`.** It is an unusual triple, but it is forced: nil must mean JSON null because that is what `encoding/json` produces, so absence needs a second channel, and an error value for absence would be wrong (absence is a normal result). `strings.Cut`'s `(before, after, found)` is the closest precedent for a bool alongside values. Keep the name `present`; `ok` would be confused with "no error".
- **Context-first, cooperative cancellation, `ctx.Err()` returned bare.** Matches `net/http` and `database/sql`; `errors.Is(err, context.Canceled)` working without unwrapping is right.
- **Finite defaults on every bound, unconditionally.** "No expression, whatever its content, can terminate the process" is the correct promise for a library evaluating document-supplied input, and having the bounds be properties of the untrusted artifact rather than of the call site is the right ownership.
- **Closed environment, no function registration.** Refusing to offer a `RegisterFunction` is a design decision people will try to reverse; it should not be reversed. It is what makes "an expression cannot reach host state" a property of the package rather than of its callers.
- **Refuse rather than round for integers.** `int64` overflow as D1001 instead of wrap or promotion, and refusing an integer that is not exactly representable as `float64` rather than rounding it, is exactly the behavior a large-ID payload needs.
- **Carriage by identity.** Returning the input map itself for `$` costs nothing and makes exactness a tautology. Documenting the aliasing consequence is the right price.
- **`Object` as a concrete type** with `Keys() iter.Seq[string]`, `All() iter.Seq2`, zero value usable, `Get` O(1), and JSON marshal/unmarshal in member order. This tracks `maps.Keys`/`maps.All` in Go 1.23 and is what the language lacks.
- **`BytesEncoding` as a one-method structural interface** satisfied by `*base64.Encoding` unchanged. This is how Go interfaces are supposed to be discovered.
- **`Reads()` returning a shared slice with a "do not modify" note.** `regexp.SubexpNames` does the same; it is the right tradeoff for something computed once.
- **`Plan` as plain data** with an enumerated `Reason`. Explaining why selection is unavailable is worth more than the enum costs.

## Things the doc comments leave unclear

- **When does "while an evaluation is running" end for an `Evaluation`?** `Eval` has a clear window. `Prepare` captures the input by reference and is safe for concurrent use; the only candidate end is `Close`, which is optional. The contract for mutating input after `Select` returns is undefined.
- **`Select` or `Complete` after `Close`.** Panic, error, or stale success? Not stated.
- **Precedence between `With` and per-call options.** `With(WithAccess(a))` then `Eval(ctx, in, WithAccess(b))`: override, error, or first wins? Does `With` chain additively, and does `With`'s result still answer `String()` and `Reads()` identically?
- **Is `$now` fixed per `Eval` call?** The doc fixes it at `Prepare` for selective evaluation; nothing says whether one `Eval` observes a single timestamp or the clock at each call, which the JSONata documentation does specify (single timestamp per evaluation).
- **Decimal tokens round silently.** "A token with a fraction or exponent is float64": `json.Number("0.1000000000000000055511151231257827")` loses digits on first arithmetic use with no refusal, while an integer that does not fit `float64` refuses. Membership rule 8 says unrepresentable values are failures. Either the rule has an exception for decimals (state it) or the classification is wrong.
- **`json.Number("1.0")` versus `1`.** Classified as `float64`, so is `$type` "number" identical? Yes presumably, but does `{ "1.0": x }` and `{ "1": x }` produce one key or two? "Object keys ... are a function of that value" suggests one; `$string(1.0)` yields "1", so the key is "1". Say so.
- **Where the `Kind` check happens for admitted types.** `Access` is consulted "only for such values"; is a `*Object` inside an `Access`-returned value routed back through `Access` or recognized directly? The doc says returned values "may themselves be foreign and are routed back", but not that admitted ones are short-circuited.
- **`Access.Index` returning `ok == false`.** Out of range? Absent element? The doc defines `ok` only for `Get`.
- **`Evaluation.Plan().Fields`.** Shared or copied? Same question as `Reads`, answered there, not here.
- **`Object.Delete` cost and `Object.UnmarshalJSON` into a non-empty object.** `Get` is stated O(1); `Delete` with order maintenance is not stated. `encoding/json` merges into an existing map; does `UnmarshalJSON` merge or replace?
- **`Object` identity across `Eval`.** If the input contains an `*Object` and the expression constructs `{ "a": $.obj }`, is the inner value the same pointer (carriage) or a copy? The Values section implies carriage; the Object doc says "the type of every object the expression constructs", which could be read as "the engine constructs its own".
- **What `Eval` does with an invalid binding name.** `Prepare` "fails only for invalid options"; `Eval` is silent on the point.
- **The unit of `Position`** (bytes, runes, or UTF-16 units to match jsonata-js), and whether `Token` is source text or a normalized form.
- **`MaxExpressionBytes` versus `$eval`.** `$eval` "compiles its argument under the same bounds": does the argument's length count against the outer expression's byte budget, its own, or both?
- **`Mode` and `Kind` have no `String`; `Reason` does.** A reader will assume the omission is meaningful.

## On the concept

The regex framing is sound in the one place it needs to be: the object of the exercise is an engine, not a transliteration, and Go's `regexp` is the proof that a notation can be reimplemented over host strings with its own declared divergences (no backreferences) and still be the same notation. The README's rule 6, divergences declared and pinned to the fixture they depart from, is the strongest idea in the document and is exactly the RE2 posture. Where the analogy breaks is at the value layer. A regex's domain is a sequence of code points, which every host models the same way to within case-folding details, and its result is positions and substrings, which are host-native by construction. JSONata's domain is JSON values, which hosts model differently precisely where it matters (numbers), and its result includes constructed objects, for which Go has no ordered type at all. The package had to mint `*Object` before it evaluated its first literal, and it admits `[]byte`, which JSON does not have. So "over the host's own values" is true on the way in and false on the way out; the honest statement is "over a stated value model that is a superset of Go's decoded-JSON values", which is what membership rule 2 quietly says.

"The host defines what a number is" is the claim I would push back on hardest, not because the decision is wrong but because the description is. Go does not say that `1/2` is `0.5`, that `int64 + float64` refuses when the integer is not representable, that `uint64` above `MaxInt64` becomes `*big.Int`, or that equality across `int64`, `float64`, and `json.Number` is by mathematical value. Those are a designed numeric tower with promotion and refusal rules, and it is a good one: it solves the actual motivating problem (the JavaScript reference silently destroys IDs above 2^53) with the right instinct (refuse, never round) and mostly stays consistent, except that decimal tokens round silently on first use while integers refuse, which the concept does not explain. Call it what it is: this evaluator defines a value model, chosen to be exact where Go can be exact. Framing it as "the host decides" invites the reader to expect Go semantics (wrapping, `int` promotion, `NaN`) that the package deliberately does not give them, and it undersells the design work that went in. For a language whose job is to move values between systems, a stated, exact value model is a stronger claim than host deference would be.

Documentation as authority is defensible only with the caveat the README does not state: JSONata's documentation is informal prose with examples, not a grammar and not a semantics, and the reference test suite is the de facto specification everyone else reads. The package's own API depends on that: `S0201`, `T2001`, `D1001`, `D1009`, `D3030` are error codes whose authority is the reference implementation's source, and the `Select` contract pins "fails as Complete would (D1009)" to it. That is fine, but it means the real authority ordering is documentation where it speaks, the reference suite where it does not, and the value model beneath both, and the README's two-tier "documentation, then value model" elides the middle tier that carries most of the weight. There is also a tension with the project that houses this: OpenBindings requires Go and TypeScript SDKs to be observably identical at the boundary, while the class explicitly says members differ wherever their value models differ and calls that correct. The "portable core" paragraph is the bridge, and it is an observation rather than a rule; if document authors are meant to stay inside it, something in the OpenBindings layer, not this README, needs to make that a commitment.
