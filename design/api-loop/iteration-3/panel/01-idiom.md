# Iteration-3 panel: Go idiom purist

Cold read of `b501e30`. Lens: Go standard-library idiom and API design.

## Grades

| Criterion | Grade | One-line justification |
|---|---|---|
| Idiomatic fit for Go | B+ | ctx-first, error-last, nil-options (slog/tls precedent), `iter.Seq`, useful zero values, and honest concurrency prose; marred by a double-nil common path, an `Env` threaded into `Marshal`, a `Prepare` that means the opposite of `sql.DB.Prepare`, and a `Plan` struct that earns nothing. |
| Ergonomics of the common path | B- | `Compile(src, nil)` then `Eval(ctx, in, nil)` then a three-arm switch then `Marshal(out, nil)`: three nils and three `err`s to do the one thing the README says people do; `EvalJSON` rescues it only for byte-in/byte-out callers. |
| Correctness and footgun risk | B | The `v, _, err :=` trap is designed in and merely warned about; empty `Select` path silently becomes `Complete`; the package doc's "integers exact at any size" contradicts `MaxIntegerBits`; `Marshal` and `EvalJSON` disagree about foreign values; the wrapping code for a Resolver error is unstated. |
| Performance headroom the API permits | A- | By-reference admission, carriage by identity, `Reads()` for fetch planning, work-before-allocation budgets, no goroutines, regex compiled at `Compile`: all right. The one structural leak is `json.Number` from `Unmarshal`, which cannot be "classified once" because a string has no identity, so every read of an ID re-parses. |
| Concept soundness | B | The regex analogy is good rhetoric and the wrong lesson is drawn from it: Go's `regexp` implements a *written* syntax spec (RE2), not "whatever Go feels like"; this class needs a written value model at class level, not "the host decides." |
| Overall | B+ | A serious, mostly Go-shaped API with a clear numeric story; five targeted cuts would make it something I'd defend in an x/ proposal. |

## What I would change

**1. Remove both nils from the common path.** Make `Limits` the receiver for the configured case and let the package-level functions use defaults, exactly as `http.Get` is `DefaultClient.Get` and `regexp.Compile` takes a string:

```go
func Compile(expression string) (*Expression, error)
func MustCompile(expression string) *Expression
func (l *Limits) Compile(expression string) (*Expression, error)
func (l *Limits) MustCompile(expression string) *Expression

// Convenience, per regexp.MatchString: no options at all.
func Eval(ctx context.Context, expression string, input any) (value any, present bool, err error)
```

`jsonata.MustCompile(src, nil)` at package scope in every consumer is the kind of thing that gets copied into a thousand codebases and never looks right. `slog.NewJSONHandler(w, nil)` is precedent for nil-options, yes, but slog is a constructor you call once per process; `Compile` and `Eval` are the two calls every user writes. Keep `*Env` nil-able on `Expression.Eval` and `Prepare`; that is the right place for it, and `tls.Config` is the precedent. But the package-level `Eval` should not take an `Env`; `regexp.MatchString` takes no `Regexp` options, and a caller who has an `Env` has already graduated to `Compile`.

**2. Take `*Env` out of `Marshal`, `NewEncoder`, and `Materialize`; unify the foreign-value story.** `Marshal(v, env)` reads one field of `Env` (`BytesEncoding`) and ignores the other three, while `EvalJSON` materializes through `env.Resolver` and `Marshal` errors on the same foreign value. That is two policies for one question. The stdlib shape is `json.Encoder.SetIndent` / `SetEscapeHTML`:

```go
func Marshal(v any) ([]byte, error)                         // base64.StdEncoding, foreign is an error
func NewEncoder(w io.Writer) *Encoder
func (e *Encoder) SetBytesEncoding(enc BytesEncoding)
func Materialize(ctx context.Context, v any, env *Env) (any, error)  // limits from env or defaults
```

Then `EvalJSON` is documented as "Materialize then Marshal," and `Marshal` alone is documented as refusing foreign values, and nobody has to remember which one is which. If you must keep an options carrier on `Marshal`, it should be a `MarshalOptions`-shaped thing (as encoding/json v2 does), not the evaluation environment.

**3. Flatten the selective-evaluation surface, fix the empty-path sentinel, and rename `Prepare`.** `Plan` is a struct with one exported field, a hidden field, a `Selective()` method that is `Reason == ReasonNone`, and a `Fields()` whose behavior when not selective is unstated. Collapse it onto the handle, the way `regexp.Regexp` exposes `NumSubexp` and `SubexpNames` as flat methods:

```go
func (e *Expression) Begin(input any, env *Env) (*Evaluation, error)
func (v *Evaluation) Reason() Reason               // ReasonNone means fields are evaluated individually
func (v *Evaluation) Fields() iter.Seq[string]     // empty when Reason() != ReasonNone
func (v *Evaluation) Select(ctx context.Context, path ...string) (value any, present bool, err error)
```

`Prepare` is the wrong word by stdlib precedent: `sql.DB.Prepare` compiles once and binds many inputs, which is what your `Compile` already does. This method binds *one* input and returns a handle you `Close`; that is `sql.DB.Begin` returning a `*Tx`. `Begin` also reads naturally next to `Complete`. And delete "an empty path is Complete": a variadic whose empty case silently performs the expensive thing is a trap for anyone writing `ev.Select(ctx, fields...)` with a filtered-to-empty slice. Make it an error (`CodeBinding` is close enough; or a dedicated code) or make the first key non-variadic: `Select(ctx, key string, rest ...string)`.

**4. Tighten the error surface.** Four specific items. (a) Drop the `errors.Is(err, &Error{Code: "D1001"})` recommendation from the package doc; constructing a throwaway struct pointer as an `Is` target is not a Go idiom anyone should learn, and `var e *Error; errors.As(err, &e) && e.Code == "D1001"` already works and is what every reader expects. If you want a shorthand, `func Is(err error, code Code) bool` is honest and cheap. (b) State what wraps a Resolver error and with which `Code`. The doc says "wrapped," full stop. (c) `Unmarshal` rejecting malformed JSON with `CodeUnsupportedValue` is a category error: bad JSON text is a syntax failure of the *input*, not an unsupported value, and encoding/json already has `*json.SyntaxError` with an `Offset`. Wrap it so `errors.As(err, &se)` works, and give it a class that is not "engine refusal." (d) The sentinels are `*Error` with exported mutable fields; `jsonata.ErrBudget.(*Error).Code = ""` compiles. `io.EOF` is an `errors.New` precisely so this cannot happen. Either give the sentinels an unexported concrete type whose `Is` matches by code, or accept and document the hazard.

**5. Make `Unmarshal` produce the model's numbers, not `json.Number`.** The package doc promises "a json.Number is classified once on first read," but `json.Number` is a `string` stored in an `any` inside `[]any` or `*Object`; it has no identity, so there is nowhere to cache the classification without mutating the container, which the same doc forbids. In practice every read of `user_id` in a loop re-parses the token. Since the class's own rule 4 defines exact carriage by *numeric value*, and the documentation's `JSON.stringify` renders `1.0` as `1` anyway, there is no spelling to preserve:

```go
// Unmarshal decodes JSON text into admitted values: integral numbers
// as int64 or *big.Int, other numbers as float64, objects as *Object
// in member order, arrays as []any.
func Unmarshal(data []byte) (any, error)
```

Keep `json.Number` in the *admitted* set for callers who arrive via `json.Decoder.UseNumber`, and keep the "on first read" rule for them, but do not make your own decoder pay that tax. While you are there, fix the first paragraph of the package doc: "integers are exact at any size" is false by `MaxIntegerBits`; say "exact up to Limits.MaxIntegerBits."

## What I would keep

- **`context.Context` on `Eval`/`Select`/`Complete`, none on `Compile`.** Compile is CPU-bounded by `MaxExpressionBytes` and `MaxDepth`; giving it a ctx would be theater. This is the `regexp` line and it is correct.
- **`(value, present, err)`.** I expected to dislike this and do not. `bufio.Reader.ReadLine` is the anti-precedent, `strings.Cut` is the precedent, and absent is a first-class language outcome that is *not* exceptional in a transform (every optional field produces one), so `sql.ErrNoRows`-style would put `errors.Is` in front of every real error check. The three-arm switch in the doc is the honest shape. Keep it, and keep the warning.
- **Limits baked into the compiled `Expression`, zero field means default, `Expression.Limits()` reads them back.** That is the `http.Server` zero-value discipline applied to a security boundary, and it makes a compile cache keyable. Do not let anyone move limits to per-Eval.
- **`Env.Now func() time.Time`, called once per evaluation.** Exact `tls.Config.Time` precedent, and the "fixed at start so `$now` is pure under selection" consequence is a genuinely nice piece of design.
- **Carriage by identity, and the frank `map[string]any` / `*Object` two-arm switch.** The alternative is wrapping every input map in a view, which allocates on the hot path to buy uniformity nobody asked for. `Marshal` handles both. Defend this. (I would add one tiny helper, `func Lookup(obj any, key string) (any, bool)` covering both shapes, so callers who do not marshal do not each write the switch.)
- **`Object` with `Get`/`Set`/`Delete`/`Len`/`Keys`/`All`, `iter.Seq` returns, zero value ready, complexity documented, and `MarshalJSON`/`UnmarshalJSON` so it plugs into encoding/json unchanged.** `All` matching `maps.All` is the right name.
- **`BytesEncoding interface{ EncodeToString([]byte) string }`.** One method, satisfied by `*base64.Encoding` without adaptation. Textbook Go interface.
- **`Resolver` as an explicit door, with the "allowlist, never reflection" instruction in the doc, and views instead of conversion.** The doc comment says what the door costs. Keep the sentence about it being the boundary of the closed environment.
- **No function registration, no host state, `$eval` inside the same budget, tail calls exempt from `MaxRecursion`.** Correct for untrusted input.
- **Engine panics recovered to `CodeInternal`; caller panics (Resolver, BytesEncoding) propagate.** That is exactly the `net/http` handler line, stated correctly.
- **`ctx.Err()` returned bare, not wrapped in `*Error`.** `errors.Is(err, context.Canceled)` must work and it does.
- **`Error()` never renders input-derived content; `Value` is charged to the byte budget; unsupported-value errors name the Go type only.** Rare to see a library think about log injection at design time.
- **`Offset`/`Line`/`Column` with the `go/token` conventions and `Column` in bytes.** Right, and the doc says why.
- **`Close()` returning nothing.** There is no failure mode, and not satisfying `io.Closer` is a feature here; `iter.Pull`'s `stop` is the precedent.
- **`Reads() (paths, known)`.** Small, sound, and the single most valuable performance affordance in the file for the OpenBindings use case.

## Things the doc comments leave unclear

- Is a zero `Env{}` equivalent to a nil `*Env`? Every field's nil case is described, so presumably yes; say it in the type doc.
- `Reads()` when `known` is false: is `paths` nil, empty, or a partial list? "Sound when known" says nothing about the other branch.
- `Plan.Fields()` when the plan is not selective: empty sequence, nil, or panic?
- What wraps a `Resolver` error, and under which `Code` and `Class`? "Passed to the caller wrapped" does not say in what.
- Two goroutines call `Select` for the same field concurrently: does the second block on the first, or compute it twice? "Computed once" implies blocking; the concurrency section should say so, because a blocking Select changes deadline behavior.
- `Close` while a `Select` is in flight on another goroutine: does the in-flight call complete, return `CodeClosed`, or is that a data race?
- `Env.Bindings` precedence: can a binding shadow a built-in (`Bindings["string"]`) or `$` / `$$`? Is that an error (`CodeBinding`) or a silent shadow?
- `Object.Set` with a key that is not valid UTF-8: the string rule says invalid UTF-8 is an error, but `Set` has no error return. Panic, silent, or error on first read?
- `Materialize` on a `map[string]any` containing one foreign value: does the map come back by identity (mutated?) or as a fresh `map[string]any` or as an `*Object`? "Admitted values are returned by identity" and "an ObjectView becomes an *Object" do not compose to an answer.
- `Unmarshal`: top-level scalars accepted? Trailing whitespace? Does the error wrap `*json.SyntaxError`?
- `MaxBytes` versus carried values: a carried 100 MB string is presumably not "allocated," but `$string($)` over it is. A sentence on carried inputs would stop the first bug report.
- `Env.Now` returning a non-UTC time: normalized to UTC before `$now` renders, or rendered in that zone?
- Does `Expression.String()` return the source verbatim, or a canonical re-rendering? `regexp` returns verbatim; the doc says "the expression source," which probably means verbatim, but the word "returns" does not.
- `Prepare` "fails only for an invalid Env," yet a bad binding name is "an error from Prepare or Eval." Which one, for the same Env?
- `Reason.String()` values are not listed; a caller logging `ev.Plan().Reason` cannot grep for what they will see.
- The doc says `$eval` "compiles and evaluates within" the budget and `MaxExpressionBytes` includes "a source passed to $eval." Per-source or cumulative across nested `$eval` calls?

## On the concept

"A JSONata evaluator is to JSON values what a regex engine is to strings" is a strong framing, and it is stronger than the README realizes, because it argues against the README's own conclusion. Go did not write `regexp` by saying "the host decides what a character is and what `*` means is the notation's." Go wrote `regexp` by adopting a *written* syntax specification (RE2), publishing the subset it implements, and shipping a syntax document that is itself normative for Go. The value model of a regex engine is thin (characters, case folding) precisely because the spec pins nearly everything. JSONata's value model is not thin: numbers, ordering across representations, member order, string indexing, regex dialect. The DIVERGENCES ledger shows this: three of seven rows cite "the documentation is silent" as the authority, one row is "pending ruling," and in each silence the decision is being made by this member. That is fine, but it is not "the host decides"; it is "the class decides and has not written it down yet." The Go member already has a de facto value model that is a *JSON* model, not a Go model: exact integers, IEEE binary64 for anything with a fraction or exponent, code-point strings, sorted-or-insertion member order. Nothing in that list is Go-specific. Write it as a class-level normative document, the way RE2 syntax is, and the regex analogy becomes exact rather than suggestive.

Documentation as authority over the reference implementation holds up in principle; it is how RFCs and language specs work, and the `$string`/`JSON.stringify` divergence is a good example of the documentation being both specific and right. Where it wobbles is that JSONata's documentation is informal, is thin in exactly the places that matter for interoperability, and its examples are generated from the reference implementation, so "the documentation" contains outputs that contradict "the documentation" whenever the doc's prose and the doc's example disagree. `$round(2.675, 2)` will land on a user's screen as `2.67` under a doc page that shows `2.68`, and the ledger's justification is "the documentation is silent on the basis." A user reading docs.jsonata.org does not experience silence; they experience an example. The class needs a rule for which wins, prose or example, and it needs to say so in the same breath as "documentation is the authority."

"The host defines what a number is" is the wrong call for a language whose job is moving values between systems, and the project already knows it: the container's own SDK-parity rule requires the Go and TS members to agree at the observable boundary, and the README quietly demotes that to "a quality commitment, not a property of the class." For a regex engine, host-defined semantics are tolerable because the pattern is the artifact and the strings stay home. For a transform language embedded in an interface specification, the *result* is the artifact and it crosses hosts by construction; a transform author writes one expression and expects one answer from every conformant evaluator. The analogy breaks exactly there. What saves the design is that the Go member's actual choices (exact integers, binary64, code points, refuse rather than approximate) are the choices a class-level JSON value model would make anyway, and a TS member with BigInt can match them. So the fix is not to change the engine; it is to promote what the engine already does from "the host's" to "the class's," so that the next member is held to it rather than invited to differ.
