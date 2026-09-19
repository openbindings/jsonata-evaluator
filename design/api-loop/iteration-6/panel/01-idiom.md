# Iteration-6 panel: Go idiom purist

Cold read of `81e3693`. Lens: Go standard-library idiom and API design.

## Grades

| Criterion | Grade | One-line justification |
|---|---|---|
| Idiomatic fit for Go | B | Strong instincts (comparable zero-value config, nil `*Env`, `iter.Seq`, `ctx.Err()` passthrough, `MustCompile`, `String()`), undercut by triple returns, an `Unmarshal` whose shape no stdlib `Unmarshal` has, and a data struct (`Limits`) moonlighting as the engine object. |
| Ergonomics of the common path | B+ | Compile, decode, eval, marshal is four lines and reads well; but every consumer pays for the `present` bool, the `map` vs `*Object` result duality (`Member` exists because of it), and an `Env` passed to functions that use one field of it. |
| Correctness and footgun risk | B- | The doc itself names the worst footgun (`v, _, err` turns absent into null); add aliasing with a Close-scoped sharing window, budget exhaustion that is nondeterministic under concurrent `Select`, a blocking `Close` with no ctx, and negative limits silently meaning "default". |
| Performance headroom the API permits | A- | Carriage by identity, no copy on admission, views for foreign data, `Reads()` for fetch planning, selective evaluation with memoization, single-pass `EvalMarshal`, work accounting rather than wall clock; the only structural tax is the sort-per-observation on maps and the re-parse of `json.Number` on every read. |
| Concept soundness | B+ | The regex framing is right and Go's `regexp` is its best witness; documentation-as-authority is right in principle but the ledger shows the docs are silent exactly where interop bites, and "integral float64 is an exact integer" invents precision the wire never carried. |
| Overall | B+ | A serious, coherent design by people who have read the stdlib; the changes below are about shape and contract, not direction. |

## What I would change

### 1. Replace the `present` bool with an `Undefined` value, and go back to two-value returns

`(value any, present bool, err error)` is the `bufio.Reader.ReadLine` shape, which the stdlib documents as a primitive most callers should avoid because the middle value gets dropped. Your own package doc concedes the failure mode: `v, _, err := expr.Eval(...)` silently turns absent into null. An API that documents its own footgun should remove it.

You already accept a sentinel value for undefined on the input side (`NoInput`). Make it one value, use the language's own word, and let it be the result too:

```go
// Undefined is the language's absent value: the input for evaluating with
// no input ($ is then absent) and the result of an expression that yields
// nothing. It never appears inside a container.
var Undefined any = undefined{}

func (e *Expression) Eval(ctx context.Context, input any, env *Env) (any, error)
func (v *Evaluation) Select(ctx context.Context, path ...string) (any, error)
func (v *Evaluation) Complete(ctx context.Context) (any, error)
func (e *Expression) EvalMarshal(ctx context.Context, input any, env *Env) ([]byte, error) // nil, nil for Undefined
```

The failure mode inverts in your favor: a caller who forgets the check and hands `Undefined` to `Marshal` gets `CodeUnsupportedValue` (it is foreign), not a silent `null`. It also matches the reference API, where `evaluate()` returns JavaScript `undefined` as a value. `io.EOF` is the precedent for "a distinguished value the caller compares against"; `Object.Get`'s comma-ok is fine to keep because that is the map idiom and absence there is not a footgun.

### 2. Make `Evaluation` single-goroutine, make `Close` non-blocking, and rename `Prepare`

`*regexp.Regexp`, `*template.Template`, and your `*Expression` are the right layer for concurrency safety. `*sql.Rows`, `*json.Decoder`, `*bufio.Scanner`, and every other per-input handle in the stdlib are not concurrent-safe, and nobody misses it. Making `Evaluation` concurrent-safe has bought you: an internal lock, "a waiter whose ctx ends returns while the computation continues for the goroutine that started it", "under concurrent selections the budget may be exhausted at a different point", and a `Close` that blocks with no ctx (so a `Select` stuck in a slow `Resolver` hangs `Close` forever; `http.Server.Shutdown` takes a ctx for exactly this reason). Nondeterministic budget failures are the worst of these: the same expression over the same input can fail or succeed depending on goroutine scheduling, which makes a `CodeBudget` error unreproducible.

```go
// Evaluation is one expression over one input. It is not safe for
// concurrent use; an Expression is.
func (v *Evaluation) Close() // never blocks; later calls return ErrClosed
```

A caller who wants parallel fields can `Begin` twice. Which brings up the name: in `database/sql`, `Prepare` returns a `*Stmt` that is input-independent and reusable across many executions. Yours binds one input and must be closed. That is `sql.DB.Begin` returning `*Tx`, not `Prepare`. `expr.Begin(ctx, in, env) (*Evaluation, error)` says what it does; `Prepare` promises reuse it does not deliver.

### 3. Restructure the package documentation so `go doc jsonata` is an overview, not a specification

`go doc` prints the package comment before a single symbol; a reader gets roughly 380 lines of numeric semantics before seeing `Compile`. `text/template` and `regexp/syntax` are the long-doc precedents, and both are structured as short sections a reader can skip. Three concrete fixes:

- The package comment is the "Compile once, evaluate many" block plus one paragraph per section heading, each under eight lines, with the sentence "The value model is specified in full below" pointing at a `doc.go` that carries the Numbers / Strings / Objects / Bounds sections as they stand.
- State each rule once. Aliasing of results with input and `Env.Bindings` appears in the package comment twice, in `Env`, in `Evaluation`, and in `Close`. Panic propagation appears three times. Pick the owning symbol and cross-reference it.
- Strip the README's vocabulary. "The class requires that the binding, not the evaluator, choose the encoding", "member", "carriage" as a noun: a `go doc` reader has no README. "Carried" is defined inline once and then used as jargon; define it in one place under a `# Carriage` heading and link there.

Also: constant comments in the `ReasonNone:` colon form are gofmt-legal but not what `go doc` renders well; the stdlib form is `// ReasonNone means fields are evaluated individually.`

### 4. Rename `Unmarshal` to `Decode` and add the missing `NewDecoder`

Every `Unmarshal` in the stdlib (`json`, `xml`, `asn1`, `encoding.TextUnmarshaler`) takes a destination and returns `error`. `jsonata.Unmarshal(data) (any, error)` has the same name and a different arity; someone reading code that imports both packages will misread it. The stdlib verb for "text in, value out, error out" is `Parse` or `Decode` (`url.Parse`, `pem.Decode`, `csv.Reader.Read`, `jsontext.Decoder.ReadValue`). You also ship `NewEncoder` with no `NewDecoder`, which is the half of the pair an untrusted-input service actually wants (`json.NewDecoder(http.MaxBytesReader(...))` is the idiom).

```go
func Decode(data []byte) (any, error)
func NewDecoder(r io.Reader) *Decoder
func (d *Decoder) Decode() (any, error)
func (l Limits) Decode(data []byte) (any, error)
func (l Limits) NewDecoder(r io.Reader) *Decoder
```

`Marshal(v, ...) ([]byte, error)` matches `json.Marshal`'s shape exactly and can stay, or become `Encode` for symmetry; the non-negotiable part is that `Unmarshal` stops lying about its shape. `Object.MarshalJSON` / `UnmarshalJSON` are interface names and are untouched.

### 5. Stop passing `*Env` where only `BytesEncoding` is used, and shrink the `Limits` receiver to the untrusted entry points

`Marshal(v, env)` and `NewEncoder(w, env)` accept `Bindings`, `Resolver`, and `Now` and ignore all three. A reader has to open the doc to learn that. Take the one thing you use:

```go
func Marshal(v any, enc BytesEncoding) ([]byte, error)
func NewEncoder(w io.Writer, enc BytesEncoding) *Encoder
```

The `net.Dialer` analogy for `Limits` holds for `Compile`, `Eval`, and `Decode`: those accept untrusted bytes and the bounds are the point. It does not hold for `Marshal`, `NewEncoder`, `Materialize`, and `Clone`: `Limits{}.Clone(ctx, v, env)` reads as nonsense because the noun describes data, not a role. `Dialer` works because a Dialer dials; `http.Client`'s five methods all send a request. Compose the encode-side bounds the way the stdlib does: `Marshal` detects cycles and errors as `json.Marshal` does, and a caller who must cap bytes uses `NewEncoder` over a limiting `io.Writer`. If you decide the in-memory byte cap on `Marshal` is non-negotiable for your threat model, then the receiver is doing a job that deserves a name that says so, and `Limits` is not it.

## What I would keep

- **`Limits` as a comparable value type whose zero value means the defaults, read back from `Expression.Limits()` with defaults filled in.** The cache-key story is exactly right and the "will stay comparable" promise is the kind of commitment a stdlib package makes. `DefaultLimits()` as a function rather than a mutable var is correct too (`http.DefaultClient` is the cautionary precedent).
- **Struct configuration, no functional options, `nil *Env` accepted.** `slog.HandlerOptions`, `tls.Config`.
- **`Compile` without ctx, `MustCompile` that panics, `String()` returning source.** The `regexp` shape, applied faithfully.
- **Cancellation is `ctx.Err()`, never wrapped into `*Error`.** `errors.Is(err, context.Canceled)` holding is the contract callers expect; do not let anyone talk you into wrapping it.
- **Carriage by identity.** `regexp.FindSubmatch` and the whole `bytes` package return slices that alias the input; this is Go's norm and the only way to keep large payloads cheap. Keep `Clone` as the escape hatch.
- **`BytesEncoding` as a one-method interface that `*base64.Encoding` satisfies unchanged.** Textbook implicit satisfaction.
- **`Env.Now func() time.Time`.** `tls.Config.Time` is the exact precedent; cite it in the doc.
- **`Object` with `Keys() iter.Seq[string]` and `All() iter.Seq2[string, any]`,** matching `maps.Keys`/`maps.All`; nil `*Object` reads as empty and `Set` panics, matching nil map semantics.
- **Recovering engine panics into `CodeInternal` while letting caller panics propagate.** `text/template` and `encoding/json` both do exactly this for their own defects.
- **Bounding by work and bytes, not time,** with ctx as the only clock. Correct: wall-clock budgets are unreproducible.
- **No function registration.** `template.FuncMap` is where most template-injection incidents come from. A closed environment with a single, allowlisted `Resolver` door is the right shape for document-supplied expressions.
- **`Error` carrying `Offset`, `Line`, `Column`, `Token`, with `Message` never containing input content and the payload isolated in `Value`.** `json.SyntaxError.Offset` and `go/token` are the right models, and the redaction stance is stated where a logger author will see it.
- **Admitting `json.Number` and `json.RawMessage`.** This is what lets `json.Decoder.UseNumber()` callers arrive without a conversion pass.
- **`Reads()`.** A static read-set that a caller can trust to fetch only what an expression touches is a rare and valuable thing to offer; keep the "known" bit conservative.

## Things the doc comments leave unclear

- **Regular-expression dialect.** "Regex literals are compiled by Compile" and nothing else. A `go doc` reader cannot learn whether `(?<=x)` or `\1` compiles. `DIVERGENCES.md` says pending. The README's own analogy answers this (see below); `Compile`'s doc must state the syntax by reference to `regexp/syntax`.
- **Negative limits.** "A negative field is treated as zero" means negative silently becomes the default. Is that a deliberate refusal to offer "unlimited", or an accident? Say which. Note this inverts `http.Server`, where a zero timeout means none; readers from that world will expect `MaxWork: 0` to mean unbounded.
- **Two "unknown" sentinels in `Error`.** `Offset` is -1 when unknown; `Line` and `Column` are 0. Pick one convention or say why they differ.
- **`Class` has no `String()`; `Reason` does.** An `Error` printed with `%v` in a log shows a bare number for the class.
- **`Expression.Fields() ([]string, bool)` versus `Plan.Fields() []string`.** Same name, different shapes. And for an empty constructor `{}`, does `Expression.Fields` return `([]string{}, true)`? `Reads` promises "empty, non-nil"; `Fields` is silent.
- **`Object.MarshalJSON` "as Marshal would with a nil Env"** cannot be fully true through `encoding/json`, which re-escapes `<`, `>`, `&`, U+2028, and U+2029 in `MarshalJSON` output; and a nil Env means any `[]byte` member makes `json.Marshal(obj)` fail. Both should be stated.
- **`Object.UnmarshalJSON` "is not for text of untrusted size"** while running under `DefaultLimits`, which are finite. Either the limits bound it (then say so) or they do not (then say why).
- **"Shared with the Evaluation until Close."** The Evaluation never mutates a returned value; what is meant is that `Complete` returns the same object by identity, so a caller's mutation of a selected field shows up in `Complete`'s result. Say that plainly; "shared" invites readers to imagine the engine writing to it.
- **When `Now` is called for an `Evaluation`.** "At most once per Eval or Prepare", but `Prepare` "evaluates nothing else", and `Now` is "not called when the expression cannot observe the clock". Is the instant fixed in `Prepare` or on first `Select`? Under a whole-evaluation plan with two `Select`s, is the second evaluation's timestamp the first one's?
- **`Resolver` "must return an equal view for the same value".** Equal by what? Same pointer? Views comparing equal with `==`? Deep-equal membership?
- **`ObjectView` "Get and Range must agree on membership; the engine trusts both."** What happens when they disagree is unspecified; say "the result is unspecified" or "the engine may return either".
- **`Encoder` and `Evaluation` zero values, and whether `Encoder` is safe for concurrent `Encode`.** `json.Encoder` is silent too, but this package is otherwise careful about concurrency contracts.
- **`Version` reads like a module version.** A package const named `Version` holding `"2.1"` next to a repository that tags `go/vX.Y.Z` will be misread; `LanguageVersion` is unambiguous.
- **`Member`, `Int64`, `Float64` as bare package functions** read like constructors (`big.NewInt`, `sql.NullInt64`). The doc clarifies, but a `go doc` index line does not. Not fatal; worth a second look.

## On the concept

The regex-engine framing is sound, and Go is its best evidence: `regexp` implements RE2 semantics natively over Go strings, refuses backreferences and lookaround by design, states so in `regexp/syntax`, and nobody calls it a port of anything. The framing has a consequence the authors have not yet cashed in: it settles their own pending question. A Go member of a class defined by "each host writes its own engine and declares what it does not support" uses `regexp/syntax`, refuses unsupported constructs at `Compile`, and gets linear-time matching over untrusted expressions as a bonus; leaving the dialect "pending" while asserting the analogy is the one place the design is inconsistent with itself. Where the analogy breaks is more interesting. A regex notation has no value model of its own; strings are whatever the host says, and the notation only asks "does this character match". JSONata has arithmetic, and its documentation was written by people who assumed IEEE doubles without ever saying so. The divergence ledger is the tell: a regex engine's divergences are about which syntax it accepts, visible in the expression text; this engine's divergences are about what `1e23 + 1` equals, invisible in the expression and dependent on the input. That is a different kind of divergence, and users will not experience it as "the Go engine doesn't support lookahead". They will experience it as "the same transform gives a different number here".

Documentation as authority is the right principle (a language is its specification, not its first interpreter) and the ledger applies it honestly, labeling each entry by which authority decided. But the same ledger shows the documentation is silent or ambiguous precisely where interop matters: number model, key order, string units, rounding ties, regex dialect. Seven of fifteen entries are decided by "value model", which means "the documentation did not say, so we did". That is a legitimate ruling, but it should be presented as one: the class *chose* host-over-reference for the silent cases, and for a language whose entire existing corpus of expressions was written and tested against the reference, that is a bet that authors value exactness over matching JavaScript. For OpenBindings it is the right bet, because the alternative silently corrupts 64-bit IDs and "refuse rather than approximate" is the only defensible stance for a transform over decoded API payloads. Just do not let the README imply the documentation led you there; the documentation is silent, and the value model is a design, not a discovery.

"The host defines what a number is" is also less true than the README says, and that matters for a language whose job is moving values between systems. Go's own definition of a JSON number is `float64` (`encoding/json` into `any`), and this package explicitly rejects that; what it ships is a value model *designed* over Go primitives (int64 ∪ big.Int ∪ float64, with cross-representation equality by mathematical value). That is a fine model, but it is the package's, not the host's, and the doc should say "built from Go's types" rather than "Go's". The one rule I would reopen is "a float64 whose value is integral takes part as that exact integer, whatever produced it". The wire token `1e23` is the decimal 10^23, which float64 cannot hold; every JSON producer and consumer on earth treats it as an approximation, and this model instead treats the rounding artifact 99999999999999991611392 as exact, renders it as 23 digits, and adds 1 to it exactly. Provenance was thrown away on purpose ("exactness is a property of the value"), but the wire is exactly where provenance lived. A regex engine never invents a character that was not in the string; this is the place where the evaluator invents precision that was not on the wire. Carriage-is-exact and integer-arithmetic-is-exact are the load-bearing commitments for the OpenBindings use case, and both survive without promoting integral floats to big integers; the `CodeInexact` refusal on mixed arithmetic beyond 2^53 already shows the authors know where the decimal boundary is. I would draw it there.
