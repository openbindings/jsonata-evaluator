# Iteration-4 panel: Go idiom purist

Cold read of `e57816b`. Lens: Go standard-library idiom and API design.

## Grades

| Criterion | Grade | One-line justification |
|---|---|---|
| Idiomatic fit for Go | B+ | Errors, ctx, nil-means-default options, `iter`, and zero values are textbook; `NoInput`, a ceremonial `Close`, compile-time evaluation budgets, and three parallel error taxonomies are not. |
| Ergonomics of the common path | B | Compile / Unmarshal / Eval / Marshal is four calls and a three-value return; the `map[string]any` versus `*Object` bifurcation taxes every consumer of a result, and `Member` is a bandage over it. |
| Correctness and footgun risk | B- | The `present` footgun is acknowledged and shipped; results may carry any of ~16 admitted numeric types; the `Unmarshal` doc contradicts the package doc on `1.0`; "Select is not a guard" is documented but weakens README rule 9. |
| Performance headroom the API permits | A- | By-reference admission, no copy, memoized selection, views, `Reads` for input pruning are all right; missing an `Append`-style encoder, and `json.Number` is re-parsed on every read. |
| Concept soundness | B+ | The regex framing is the correct implementation stance and is Go's own origin story; it breaks exactly where a regex engine never had to go, which is constructing new values and deciding what a number is. |
| Overall | B+ | A serious, well-argued API with roughly 80 exported identifiers where 55 would do; the value-model doctrine is stronger than the surface that exposes it. |

## What I would change

**1. Separate parse bounds from evaluation budgets, and put the budget where the evaluation is.**

Today `Limits` bundles two unrelated things: bounds on the *text* (`MaxExpressionBytes`, `MaxDepth`) and bounds on the *run* (`MaxRecursion`, `MaxOutputNodes`, `MaxWork`, `MaxBytes`, `MaxIntegerBits`), and both are baked into the `*Expression` at `Compile`. The stated reason ("so they apply wherever it is evaluated") is a safety argument that finite zero-value defaults already satisfy. The cost is real: an OpenBindings host that compiles one document transform and serves many tenants cannot vary budget per call without recompiling, so the compiled-expression cache the doc encourages is keyed on budget, not on source. The design has already conceded the point once: `Materialize` takes its own `limits *Limits` because a budget at compile time made no sense there.

Precedent: `sql.Stmt` is prepared once and every `QueryContext` carries its own per-execution controls; `http.Server` timeouts are on the server, `http.MaxBytesReader` is per request; `regexp.Compile` takes no options at all because RE2's cost is a property of the notation, and its one real bound (pattern size) is an internal constant.

I would write:

```go
func Compile(expression string) (*Expression, error)
func MustCompile(expression string) *Expression

// Budget bounds one evaluation. The zero value is the default, which is finite.
type Budget struct {
    MaxRecursion, MaxOutputNodes, MaxWork, MaxBytes, MaxIntegerBits int
}

type Env struct {
    Bindings      map[string]any
    Resolver      Resolver
    BytesEncoding BytesEncoding
    Now           func() time.Time
    Budget        Budget
}
```

Make `MaxExpressionBytes` and `MaxDepth` package constants (a caller who wants a smaller source cap checks `len(expression)` before calling, as with any string). `Expression.Limits()` and `DefaultLimits()` disappear, `MustCompile` regains regexp's single-argument shape, and `Compile` becomes a pure function of the source, which is what a cache wants to key on.

**2. Delete `Close`, `CodeClosed`, and `ErrClosed`.**

The doc is candid that `Close` releases nothing the garbage collector would not: "An Evaluation that is never closed is reclaimed by the garbage collector." What remains is a contract ("the input and Env are no longer borrowed, values already returned are no longer shared") that Go cannot enforce and the doc immediately undercuts ("Returned values may still alias the input and Env.Bindings, as any result may"). `sql.Rows.Close` returns a connection to a pool; `os.File.Close` releases a descriptor. A `Close` whose only observable effect is that later calls fail with a code invented for that purpose is ceremony, and it pulls `defer ev.Close()` into every example for no benefit. Replace the borrow language with "for as long as the Evaluation is in use" and let `ev` go out of scope.

**3. Give typed consumers a finite output value model.**

Because carriage preserves representation, a result can contain `json.Number("1.0")`, `int8`, `uint16`, `float32`, `map[string]int64`, a named `[]byte`, or a `*Object`, depending on what the input happened to hold. A consumer doing `switch v := out.(type)` must therefore enumerate the whole admitted set, and `Marshal` is the only function that does so on the caller's behalf. `Materialize` claims to be "for callers handing a result to typed code" but only resolves foreign values; it leaves the numeric and container heterogeneity intact.

`encoding/json` earned its usability by promising exactly what `Unmarshal` into `any` yields: `bool`, `float64`, `string`, `[]any`, `map[string]any`, `nil`. Do the same. Either make `Materialize` produce only `nil`, `bool`, `string`, `[]byte`, `int64`, `*big.Int`, `float64`, `[]any`, and `*Object` (my preference: it is already a bounded walk with a budget), or add a `Canonical` with that contract. This does not violate carriage: carriage is a property of `Eval`; normalization is an opt-in second step, exactly as `Marshal` already is.

**4. Drop `NoInput` and, with it, the in/out asymmetry.**

A package-level `var NoInput any = noInput{}` is a sentinel value of unexported dynamic type that is legal only at one position (the root) and "foreign" everywhere else. There is no stdlib precedent for a sentinel `any`; `io.EOF` is a sentinel `error`, which is a different thing. It exists to mirror the reference implementation's `evaluate()` with no argument, a calling convention no Go caller has. Meanwhile the output side uses `present bool`. Pick one mechanism: the bool is the right one (map lookup, type assertion), so the input side should simply not offer undefined-as-input until a caller needs it. YAGNI is a Go proverb in spirit if not in letter.

**5. Trim the surface: three error taxonomies, `Encoder`, `Plan.Selective`, `Member`.**

- Errors can be classified by `e.Code == CodeBudget`, by `errors.Is(err, ErrBudget)`, and by `e.Code.Class() == ClassEngine`. Codes must exist because the language defines them, and `errors.Is` sentinels are the Go idiom (`fs.ErrNotExist` over `syscall.Errno` is the exact precedent). `Class` is the third axis; if it stays, it should be the *only* grouping mechanism callers are steered to for language codes, and the eight sentinels should be reduced to the ones a caller actually branches on (`ErrBudget`, `ErrUnsupportedValue`, `ErrInexact`).
- `NewEncoder`/`Encoder`: `Marshal` plus `w.Write` covers it, and the proposed `Encode` is *weaker* than `json.Encoder` (which buffers and writes whole; yours documents partial writes). If a streaming form is wanted for performance, the modern shape is `AppendJSON(dst []byte, v any, env *Env) ([]byte, error)`, following `strconv.Append*` and `slog`, not an `Encoder` struct.
- `Plan.Selective()` is `p.Reason == ReasonNone`. A method that restates a field comparison is surface without information.
- `Member(v, key)` exists only because results are sometimes `map[string]any` and sometimes `*Object`. Change 3 makes it unnecessary for consumers who normalize; for the rest, `Object.Get` and a map index suffice.

## What I would keep

- **`(value any, present bool, err error)`.** It is the map-lookup idiom applied to a language that genuinely has a fourth outcome. A sentinel value would leak, an `ErrAbsent` would conflate absence with failure the way `io.EOF` regrettably does. The doc's honesty about `v, _, err` is the right way to ship it.
- **`ctx` on every evaluation, none on `Compile`, and `ctx.Err()` returned bare** so `errors.Is(err, context.Canceled)` holds. This is `database/sql`'s contract exactly.
- **`nil *Env` equals zero `Env`**, and nil-pointer-means-defaults for options. `slog.HandlerOptions` and `tls.Config` are the precedent.
- **`Env.Now func() time.Time`**, with the instant fixed per `Eval`/`Prepare`. No process-wide clock, no init-time state, testable.
- **The closed environment**: no function registration, `Bindings` holds values only, `Resolver` as a single-method interface named by its verb with the allowlist guidance in its doc. This is the security posture the OpenBindings use case demands and it is stated where a `go doc Resolver` reader will see it.
- **`BytesEncoding` as a one-method structural interface satisfied unchanged by `*base64.Encoding`.** This is Go interfaces used the way they were meant to be.
- **`Error` design**: `Message` never carries input content, `Offset`/`Line`/`Column` match `json.SyntaxError` and `go/token`, `Unwrap` returns only the Resolver's cause, `Is` matches by code. Correct and restrained.
- **Admission by exact type first**, `[]byte` before `[]uint8`-as-array, nil slice as empty array, nil map as empty object, nil pointer as null, and UTF-8 validity enforced. Each is the choice a Go programmer expects, and each is justified in one sentence.
- **Values by reference, no copy on admission, admission checked per value on first read.** This is what makes the performance grade possible.
- **`Object` with a useful zero value, `iter.Seq` accessors, and `MarshalJSON`/`UnmarshalJSON`** so it composes with `encoding/json` without ceremony.
- **The panic boundary**: engine panics recovered into `CodeInternal`, caller-callback panics propagated. `encoding/json` and `text/template` do the same for their own internals.
- **Immutable, concurrent-safe `Expression`; no goroutines; no global state.** The regexp contract.
- **Regex literals compiled at `Compile`** so malformed patterns fail early.
- **Refuse rather than approximate** (`CodeInexact`, D1001 for non-finite, budget before allocation). For untrusted transforms this is the only defensible policy.

## Things the doc comments leave unclear

1. **`Unmarshal` versus the package doc on numeric tokens.** `Unmarshal` says "integral numbers as int64 or *big.Int, other numbers as float64." The package doc's `json.Number` rule says "a token with a fraction or exponent is the nearest float64," so `1.0` and `1e2` are `float64`. Which does `Unmarshal` follow for `1.0`, `1e2`, `-0`, `1E400`? Carriage of `$.x` where `x` was `1.0` returns a different Go type under each reading.
2. **Whose `ctx` governs a shared field?** `Evaluation` is concurrent-safe and "a Select of a field another goroutine is computing waits for it." If the computing goroutine's context is cancelled mid-field, what does the waiter receive, is the field's partial work retried on the next `Select`, and is the budget it consumed refunded?
3. **Is an `Evaluation` usable after an error?** After `Select("a")` returns `CodeBudget`, or `CodeInternal`, may `Select("b")` proceed? "One budget across all selections" suggests the budget is spent; the doc does not say whether the Evaluation is poisoned.
4. **Does `Complete` fail if a previously selected field failed?** "A failure in one field's subexpression does not fail another field's" is stated for `Select`; `Complete` necessarily needs every field, so presumably it fails, but a reader has to infer it.
5. **Why does an `Env` binding make `Reads` unknown?** Bindings are values only and cannot read the input, so `$foo` in an expression cannot widen the set of input members read. Either there is a reason (a binding may alias the input?) or the rule is over-conservative.
6. **Is a nil `*Object` safe for reads?** Evaluation treats it as null; as a Go value, does `Get`/`Len`/`Keys` on a nil receiver return empty like a nil map, or panic? `Object` "zero value is empty and ready to use" speaks to `Object{}`, not `(*Object)(nil)`.
7. **`Object.Map` cost and aliasing.** Is it a fresh map every call? Does it share values (obviously) and is it O(n)?
8. **`Member` on `map[string]T` for arbitrary admitted `T`** implies reflection; a doc reader cannot tell whether `Member` is a cheap two-arm switch or a reflective lookup.
9. **`Limits` as a cache key.** `Limits{}` and `DefaultLimits()` compile to the same expression but are different comparable values; a cache "keyed by the source and the Limits" will hold two entries. (Moot under change 1.)
10. **`Select` below constructor granularity.** "A lookup by member key within the deepest selected field": is that lookup through `ObjectView`/`Resolver` when the field carried a foreign value, and is it charged to the budget?
11. **Valid binding names.** "Names omit the leading `$` and may not begin with one" says what is forbidden, not what is permitted (JSONata identifier rules? any non-empty string?).
12. **The process-survival promise.** "No expression, whatever its content, can terminate the process" is stronger than Go can guarantee: concurrent map mutation by the caller (which the doc itself names as fatal) and stack exhaustion are not recoverable. `MaxRecursion` 1024 makes the second unlikely; say "no expression can panic the calling goroutine" instead.
13. **`ReasonArrayInput` for a root that is an `ArrayView` or foreign.** Presumably the root is classified through the Resolver at `Prepare` ("charged to the budget"); the doc for `Reason` does not say views count.
14. **`sentinel.Error()` text**, and whether `errors.Is(err, ErrBudget)` is ever true for a wrapped `CodeResolver` error whose cause was itself a `*jsonata.Error` from a nested evaluation.
15. **The package doc's length.** Roughly 290 lines print before the first symbol under `go doc jsonata`. `text/template` is the precedent for a long package doc, but that one is the language reference; here the language reference lives at docs.jsonata.org and the doc is a value-model specification that overlaps `DIVERGENCES.md`. The Numbers, Strings, and Objects sections are normative and should stay; the sentences that restate ledger rows ("a declared divergence") could point to the ledger instead.

## On the concept

"JSONata evaluator as regex engine" is a sound implementation stance, and it is literally how Go's `regexp` came to exist: RE2 is a notation re-implemented over Go strings, not PCRE ported, and it declares what it does not support. The framing also predicts its own reception, which the authors should take seriously. Go's regexp divergences (no backreferences, no lookaround) are the single most-complained-about property of the package, and users do not experience them as "a declared divergence," they experience them as "Go's regex is broken." JSONata embeds a regex sublanguage, so the analogy recurses: the ledger's "regular-expression dialect: pending ruling" will almost certainly resolve to RE2 for a host-native Go member, at which point `$match` with a lookahead fails at `Compile` while it works in every JavaScript host. That is the analogy working exactly as designed, and it will be the first issue filed.

Documentation as authority holds up where the documentation speaks, and the ledger already shows the seams where it does not: the `$round` row is honest that it is an "interpretation ... pending a ruling," and the member-order and string-ordering rows are "value model: the documentation is silent." JSONata's docs are tutorial-grade prose, not a specification; the reference test suite is the de facto spec, and membership rule 1 verifies against that suite. So the operative authority is "documentation, with the suite as tie-breaker, except where a declared divergence says otherwise." That is a perfectly good position, but it means `DIVERGENCES.md` is where the language is actually being specified for this member, one silence at a time. Treat the ledger as a normative, versioned artifact rather than a defect list, because that is what it is.

"The host defines what a number is" is the right call for this project only because rules 4 and 8 exist. Without exact carriage and refuse-rather-than-approximate, host-native numerics would be a portability disaster for a language whose job here is moving values between systems; with them, the Go member is strictly better than the reference at the thing OpenBindings cares about, which is that `9007199254740993` arrives as `9007199254740993`. But this is also where the analogy breaks. A regex engine's output is substrings of its input, values the caller already held; it never has to decide what a *new* string is. A JSONata evaluator constructs values, so the host must define the result model, and the Go member's answer (`int64`/`float64`/`*big.Int`, `*Object`) will differ from a TypeScript member's answer (`number`/`bigint`, plain objects). "Two members agree wherever their value models agree" then becomes the load-bearing caveat of a project whose thesis is cross-host portability of transforms, and the README's "portable core" paragraph is the honest admission that the guaranteed-portable subset is the subset where nobody does arithmetic. The sharpest consequence: `9007199254740993 + 0.5` is `CodeInexact` in Go and a silently rounded number in JavaScript. Error-versus-value is the worst kind of divergence for a document-supplied transform, because a document author cannot test for it in one host and trust the other. If refuse-rather-than-approximate is a class rule (it is, rule 8), then `CodeInexact` should be a class-level obligation with a class-level code, not a Go-only engine refusal, so that a member which rounds silently is non-conformant rather than merely different.
