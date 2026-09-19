# Iteration 2 cold read: go idiom purist

> Given only the class README and go/jsonata at 4cb2f5d; told not to read design/. Verbatim.

## Grades

| Criterion | Grade | One-line justification |
|---|---|---|
| Idiomatic fit for Go | B | The regexp-shaped core (Compile/MustCompile/String, immutable concurrent `Expression`, zero-value `Limits`, nil-ok `Env`) is exactly right; it is undercut by a library-defined ordered object type, Rust borrow vocabulary, a `Close` with nothing to close, an interface named by noun instead of method, and code-point error offsets. |
| Ergonomics of the common path | B- | Compile once then `Eval` is clean, but every consumer of a result must write a two-arm `map[string]any` / `*Object` switch and a three-value return, ceremony no stdlib decoder asks for. |
| Correctness and footgun risk | B- | Refusal discipline, bounded budgets, and the error design are strong; `v, _, err :=` silently turns absent into null, unresolved views can reach `json.Marshal` and encode as `{}`, `[]string` inputs are refused, and the `uint` widening rule in the doc is wrong on 64-bit. |
| Performance headroom the API permits | A- | By-reference carriage, no goroutines, comparable `Limits` for cache keys, `Reads` for fetch pruning, lazy views, shared work across selections; only an `Append`-style encoder is missing. |
| Concept soundness | B+ | Right thesis for a Go engine and the right numeric split; the regex analogy breaks precisely at computed numbers and cross-host portability, and `Object` contradicts the host-native claim. |
| Overall | B+ | A serious, well-reasoned API that a core reviewer would accept after one round; the round would be about removing things, not adding them. |

## What I would change

### 1. One object type on the way out: drop `*Object` and let Go's map be the object

This is the biggest call and I would make it against the authors' own README. Rule 2 says host-native members evaluate over "the host's ordinary values, by reference." Go's ordinary JSON object is `map[string]any`. `*Object` is not an ordinary Go value; it is the JavaScript value model (insertion-ordered objects) re-imported through a library type, which is the exact move the README says this class refuses to make. Rule 6 already licenses the consequence: divergence is permitted "where the value model cannot represent a reference value," and an ordered object is precisely what a Go map cannot represent. Every `$keys` / `$string` / `$each` fixture that depends on insertion order becomes a declared divergence pinned to its fixture, which is the mechanism the class built for this.

What it buys: `Object`, `NewObject`, ten methods, `Unmarshal` (a `json.Decoder` with `UseNumber` suffices), `ReasonNone`-style special cases for two container types, and the documented "two-arm switch" all disappear. `reflect.DeepEqual` in consumer tests works. Sorted-key output matches what `encoding/json` does to every Go map already, so the wire result under OpenBindings is deterministic and unsurprising to any Go developer. The package doc already defines the observed order of a map as sorted code-point order; make that the only order.

Concrete wording for the Values section: "An object is a `map[string]any`. Member order is not a property of an object; operations that enumerate members do so in sorted byte order. This is a declared divergence from the reference implementation, listed in the member ledger." If the authors cannot accept sorted order for constructed objects, the fallback is still one output type: keep `*Object` internally able to wrap a map without copying, guarantee "every object in a result is an `*Object`," and let `Map()` return the wrapped map by identity. Two output types for one JSON kind is the one thing I would not ship.

### 2. Delete `Close`, `CodeClosed`, and `ErrClosed`

An `Evaluation` holds no OS resource, spawns no goroutine, and is "reclaimed by the garbage collector" if never closed, so by the doc's own account `Close` exists only to end a borrow. Go has no borrows; `regexp.Regexp`, `template.Template`, and `strings.Replacer` hold state across calls and none of them close. The state machine (`CodeClosed` on use-after-close) is three exported symbols and one error class spent policing a convention the runtime cannot enforce.

Replace the Ownership paragraph with the `bytes.Buffer.Bytes` style of statement: "Values returned by Select are reused by later Select and Complete calls on the same Evaluation. Do not mutate a returned value while you still intend to call the Evaluation; copy it instead." Then `Prepare` returns a plain value and the doc no longer needs the words "borrowed" or "shared until."

### 3. Name the resolver interface by its method: `Access` becomes `Resolver`

```go
type Resolver interface {
    Resolve(ctx context.Context, v any) (any, error)
}

type Env struct {
    Bindings      map[string]any
    Resolver      Resolver
    BytesEncoding BytesEncoding
}

func Materialize(ctx context.Context, v any, r Resolver) (any, error)
```

Single-method interfaces are named for the method (`io.Reader`, `http.Handler`, `slog.Handler`, `driver.Valuer`); `Access` is a noun that describes nothing a reader can call. The rename also fixes the doc's grammar ("the Env's Access" reads as a field access, not a type). For symmetry, `BytesEncoding` is acceptable as-is only because it is written to be satisfied by `*base64.Encoding` unchanged; say so in the doc comment and leave it.

### 4. Report error positions in bytes as `Offset`, matching every stdlib position

```go
type Error struct {
    Code    Code
    Message string
    // Offset is the byte offset into the expression source of the offending
    // token, or -1 when unknown.
    Offset int
    Token  string
    Value  any
}
```

`json.SyntaxError.Offset`, `go/token.Position.Offset`, `go/scanner`, `strconv.NumError`, and `regexp/syntax` are all byte-based. The stated reason ("editors and the reference report columns in characters") is not true of the editor protocol (LSP is UTF-16 code units by default) and does not bind a Go library. A caller who wants characters computes `utf8.RuneCountInString(src[:off])` in one line; the reverse direction, characters to bytes, forces a scan. Drop `Line` and `Column` or derive them on request; three parallel integers on one declaration line is a `go doc` smell on its own.

### 5. Fix the `Plan` enums: remove the redundant `Mode`, make zero values consistent, stop sharing slices

`Mode` is a function of `Reason` (`ReasonNone` if and only if `ModeSelective`), so the struct carries two fields that can disagree. `Mode` starts at `iota + 1` and `Reason` at `iota`, so one zero value is invalid and the other is meaningful; a Go reviewer will ask why on the first read.

```go
type Plan struct {
    // Reason is ReasonNone when fields are evaluated individually, and
    // otherwise explains why every Select evaluates the whole expression.
    Reason Reason
    fields []string
}

func (p Plan) Selective() bool           { return p.Reason == ReasonNone }
func (p Plan) Fields() iter.Seq[string]  { ... }
```

`Fields`, `Expression.Reads`, and `Plan.Fields` all return internal slices with a "must not modify" clause. `regexp.SubexpNames` does that too, but that is a 2011 API; new code returns an `iter.Seq` or a clone. Pick the iterator; it also lets `Reads` stop allocating a `[][]string` for expressions nobody inspects.

## What I would keep

- `Limits` as a comparable struct whose zero value means defaults, fixed at `Compile` and readable back via `Expression.Limits()`. Bounds traveling with the untrusted thing rather than with the call site is the right model for document-supplied transforms, and a comparable struct as a cache key is exactly what a caller wants.
- `Compile(expression string, limits *Limits)` with nil meaning defaults. I would normally push for the `net.Dial` / `net.Dialer` split, but `slog.NewJSONHandler(w, nil)` and `jpeg.Encode(w, m, nil)` are the same shape and nobody complains; keep it.
- `Expression` immutable and safe for concurrent use, `MustCompile` panicking for build-time constants, `String()` returning source. This is `regexp` and readers will recognize it instantly.
- The `(value any, present bool, err error)` triple. It is heavy, but absence is a real language value that is not `nil`, and a sentinel `Undefined` value would leak into containers and comparisons. The triple keeps absence at the top level where the language keeps it. Keep it; document the `_` footgun (below).
- Cancellation returned as bare `ctx.Err()` so `errors.Is(err, context.Canceled)` holds; `Compile` taking no `ctx` because it is bounded by construction. Both match `database/sql` and `net`.
- No goroutines, no process-wide state, panics recovered into `CodeInternal` only for the engine's own defects and re-raised for caller code. `encoding/json` does exactly this and it is the correct place to draw the line.
- No function registration. The closed environment is the security property; the temptation to add a `Funcs` map (as `text/template` does) will come every quarter and should lose every time.
- `Error` with `Is` matching by `Code`, sentinels typed as `error`, and a `Message` that never carries input content. The undocumented consequence that `errors.Is(err, &jsonata.Error{Code: "D1001"})` works for language codes is a feature; write it down.
- `[]byte` as a string under a caller-chosen encoding, produced lazily. This is the one place the binding's authority reaches into the engine and it is scoped exactly right.
- `Reads()` on the compiled expression. It is the API that makes selective fetching possible upstream and it costs nothing at evaluation time.
- `EvalJSON` as a convenience rather than a second evaluation path, and `Eval` as a package function mirroring `regexp.MatchString`.
- Key naming that mirrors `maps`: `Keys` and `All` returning `iter.Seq` / `iter.Seq2`, if `Object` survives at all.

## Things the doc comments leave unclear

- **`uint` widening is wrong.** "uint through uint32 are int64" but `uint` is 64 bits on every platform this will run on, so a `uint` above `math.MaxInt64` cannot be `int64`. It needs the `uint64` rule. As written the doc promises something the implementation cannot do.
- **`present` discarded means null.** Nothing at `Eval` or `Select` warns that `v, _, err :=` collapses absent into `nil`, which is the single distinction the Absence section exists to preserve. One sentence on each method: "Callers must inspect present; a nil value with present true is null."
- **Views reaching `json.Marshal`.** A result "that Access resolved is returned as the resolved value," so a result can contain a caller's `ObjectView` implementation; `json.Marshal` on that renders `{}` with no error. Does `EvalJSON` materialize before encoding? The package doc implies the caller must call `Materialize`; the method doc says nothing. Say which, on `Eval` and `EvalJSON`, not only in the package preamble.
- **Typed slices and maps are refused.** `[]string`, `[]int64`, `[]float64`, `map[string]string`, and `[]map[string]any` are the most common shapes in typed Go code and every one of them is `CodeUnsupportedValue` without a resolver. The admitted list is precise, but a skimming reader will not infer the exclusion. Either lead the Values section with that sentence, or offer a caller-side one-shot `Admit(v any) (any, error)` that converts by `reflect.Kind` (this is outside the closed environment; the expression never drives it).
- **Is `Resolve` memoized?** "Consults it once per foreign value" and "an expression can call the Access with any key, in any order, as many times as the work bound allows" are in tension. Say whether identity is cached per evaluation.
- **Are resolver errors wrapped?** If `Resolve` returns `myErr`, does the engine return it bare, wrap it so `errors.Is(err, myErr)` holds, or replace it with `CodeUnsupportedValue`? The `Access` doc says "anything else is an error (CodeUnsupportedValue)" about return values, not about returned errors. This matters for callers who use their own sentinels.
- **`MaxOutputNodes` versus O(1) carriage.** The bound "counts every value" of the result; a carried 1M-node subtree either gets walked (so carriage is not O(1)) or does not count (so the bound is not a bound). Which?
- **`Select` below constructor granularity into an array.** "By member key only," so what does `Select("items", "0")` return when `items` is an array: absent, an error, or a lookup of a member named `"0"`?
- **Absent result's `value`.** "present is false for an absent result" does not state that `value` is nil in that case; the error case is specified, the absent case is not.
- **`Code.Class()` for a malformed code** returns the zero `Class`, which is not a declared constant. Document it or add `ClassUnknown`.
- **No way to pin the clock.** `Prepare` and `Eval` fix `$now` from the wall clock and nothing can override it, so a test of any expression using `$now` is nondeterministic. `tls.Config.Time func() time.Time` is the stdlib precedent for exactly this; add `Env.Time`.
- **`Prepare` reads as compilation.** Every Go reader knows `sql.Prepare`; this `Prepare` binds an input. `Bind` or `With` would not mislead.
- **`Complete` reads as an adjective.** "Evaluation.Complete" could be a predicate. `Result` or `Whole` would not.
- **`json.Number("1e400")`.** "The nearest float64" is `+Inf`; the refusal rule is stated for `float64` inputs, not for numeric tokens. Presumably D1001 on first use; say so.
- **Package doc length.** At 180 lines, `go doc jsonata` is a design document. The Numbers and Strings sections are conformance material that belongs in the member README (referenced from the package doc, as `regexp` references `regexp/syntax`); Bounds belongs on `Limits`; Environment belongs on `Env`. Also pick one constant-comment style: "ReasonNone: the plan is selective" and "ModeSelective means fields are evaluated" are two.
- **`Materialize` is unbounded.** It walks the caller's own value through the resolver with no `Limits`; say that explicitly so nobody feeds it an expression result plus an untrusted resolver expecting the budget to hold.

## On the concept

"JSONata evaluator as regex engine" is a sound framing for the implementation stance and an honest correction to how every other port was built. Nobody expects Go's `regexp` to reproduce PCRE's bugs, and the argument that a JSON transformation language deserves the same treatment is right: write it for the host, over the host's values, and declare what you do not do. The number split is the strongest part. Carriage exact, arithmetic host-defined, refusal instead of approximation: that is precisely how `encoding/json` already treats numbers in Go, and for the OpenBindings use (large integer IDs crossing a boundary intact) `int64` plus `*big.Int` is strictly better than the reference's float64-only model. "The host defines what a number is" is the right call because the alternative, "JavaScript defines what a number is," is the thing that loses IDs above 2^53 today.

Where the analogy breaks is in what the notation outputs and who writes it. A regex returns positions and substrings of its input; it never computes a value, so "the host defines the primitives" costs nothing observable. JSONata computes values, and the documentation is silent on exactly the primitives that produce them, so two members that both "implement the documentation" legitimately disagree on `0.1 + 0.2`, on `$round`, on `$string` of an object, and on any integer above 2^53. Regex engines get away with this because regexes are written per host; OpenBindings transforms are written once, into a document, and evaluated by whichever host the consumer happens to be. That is the opposite of the regex situation, and the README's "portable core" is described as "an observation, not a rule." For a language whose purpose here is moving values between systems, the portable core has to become a rule somewhere, presumably in OpenBindings Core rather than in this class, or the analogy licenses divergence in exactly the place portability is the product.

Documentation as authority holds up in principle (POSIX and ECMAScript both rank spec above any implementation) but JSONata's documentation is user documentation, not a specification, and the thing this class actually runs is the reference test suite. In practice the authority is "the suite, minus declared divergences," and the README would be more honest saying so; the word "documentation" is doing work the artifact cannot bear. The place the thesis is actually betrayed, though, is not numbers, it is objects. Having argued that the host owns the value model, the Go member then reaches for a JavaScript-shaped ordered object because the reference's fixtures assume one. That is the port instinct the README was written to reject, and Go's own answer, an unordered map with sorted enumeration and a declared divergence, is sitting in the README's rule 6 waiting to be used.
