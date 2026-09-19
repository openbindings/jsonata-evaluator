# Iteration-5 panel: security and sandbox reviewer

Cold read of `7f8b4fe`. Rotating lens: security and sandbox (the expression is hostile).

## Grades

| Criterion | Grade | One-line justification |
|---|---|---|
| Idiomatic fit for Go | A- | ctx-first, errors.Is/As, iter.Seq, borrow semantics stated; only `Close` is ambiguous about whether it blocks, which matters because the failure mode is a fatal map race. |
| Ergonomics of the common path | B+ | `EvalJSON` is the one safe path for a service; `Eval` plus `Marshal` requires the caller to know about aliasing, foreign values, deadlines, and unbounded serialization. |
| Correctness and footgun risk | B- | Aliasing into shared bindings, carried-but-never-validated values, unbounded `Marshal`, `Error.Value` carrying payload content: each is documented, none is safe by default. |
| Performance headroom the API permits | A- | No copies, no goroutines, views, limits on the Expression, sticky budgets; the same zero-copy choice is what makes the amplification and cycle attacks possible. |
| Concept soundness | B+ | Sound and observable in results; but the documentation is silent on every security-relevant behavior, and the one pending ledger row (regex dialect) is the one that decides ReDoS. |
| Overall | B+ | Better than any JSONata engine I know of on closure and budgets; the budget is a work bound, not a CPU bound, and that gap is exploitable today. |

## Threat model and attack surface

**Assets.** (1) Process availability: a long-lived service with N concurrent evaluations; one fatal error (stack overflow, concurrent map write, OOM) takes down every tenant. (2) Payload confidentiality across requests: the input of one evaluation and the bindings shared by all. (3) Whatever the binding layer puts in `Env.Bindings`, which in OpenBindings includes context and may include credentials. (4) Whatever a Resolver can reach (application structs, protobufs, an ORM graph). (5) Log and error-response hygiene.

**Expression author** (interface document; fully hostile): controls the expression source up to 256 KiB, every `$eval` source, every regex literal, every key passed to a view's `Get`, every foreign value's read count, the shape and multiplicity of the result, and `$error` messages. Cannot reach host state, cannot register functions, cannot enumerate bindings, cannot observe time inside an evaluation (clock fixed once), cannot catch errors. Can read any binding by name.

**Payload author** (the upstream service; hostile): controls the bytes `Unmarshal` sees: nesting, integer digit count, string length, invalid UTF-8, duplicate keys. Controls the strings a regex runs over. Becomes an expression author whenever the expression does `$eval($$.something)`.

**Resolver and view author** (the integrator; trusted but buggy): views can be cyclic, lazy, infinite (`Len` = 1e18), inconsistent (`Get` versus `Range`), non-idempotent, slow, or panicking. The engine "trusts both" and re-resolves on every read without memoizing.

**Caller** (trusted): can violate the borrow (mutate during Eval), share a mutable binding across goroutines, mutate a result that aliases a binding, or call `Marshal` on a cyclic result.

## Findings

### 1. `MaxWork` is not a CPU bound: one-unit charges for operations whose cost is proportional to an uncharged value. Severity: high.

`MaxWork` charges "nodes evaluated" and "calls," but the cost of a node is proportional to the size of the values it touches, and carried or input values are explicitly not charged. Any string function, equality, ordering, `$string`, `$merge`, the transform copy, and (per finding 5) regex, scan their operands.

Attack, with `blob` a 10 MB string in the payload:

```
$count([1..1000000].$contains($$.blob, "☃"))
```

Roughly 1M work units, 10 TB of scanning. Or, using carriage to amplify without any large input:

```
($a := [1..200000].$$; $a = $a)
```

`$a` is 200k references to the root (200k nodes, no bytes charged); deep equality then visits 200k × |input| values. `$sort` over long strings, `$distinct` over aliased structures, and `$string` (bounded by MaxBytes on output but not on scanning input it then rejects) all have the same shape.

Doc: **addressed incorrectly.** The package doc says "the bounds count work and bytes, not time," which is honest, but `Limits.MaxWork` says it "bounds the work of one evaluation" and enumerates what counts, which reads as a bound. The only real defense is a ctx deadline, and nothing tells a service that a deadline is mandatory.

Fix: define one unit of work as an O(1) step and charge every size-proportional operation per byte or per value visited: string scan, compare, hash, encode, decode, regex step (see 5), deep equality, ordering, `$string`, `$merge`, transform copy, `Materialize`, and the `Marshal` pass of `EvalJSON`. Wording for `MaxWork`: "Every unit of work is a bounded-cost step: an operation whose cost scales with a value's size is charged per byte or per value it visits, including values that are carried or supplied by the caller. MaxWork therefore bounds CPU up to a constant factor; ctx bounds wall time." Until that holds, add to the package doc: "A service must supply a deadline; Limits do not bound CPU."

### 2. Value walks are not depth-bounded, and Go stack overflow is fatal, not a panic. Severity: high.

`MaxRecursion` bounds evaluation depth; `MaxDepth` bounds parse and `Unmarshal`. Nothing bounds the depth of a walk over a caller-supplied or view-supplied value. Go's "goroutine stack exceeds 1000000000-byte limit" is `fatal error`, unrecoverable, and the package promise "no expression can panic the calling goroutine" does not cover it.

Attack: a Resolver over an ORM or protobuf graph with a parent link (realistic, not contrived), or a `map[string]any` the caller built with a self-reference, then any of:

```
$string($)
$ = $
$ ~> |**|{}|
```

Each level is one Resolver call (charged as one unit), so `MaxWork` = 8M permits 8M frames; at a few hundred bytes per frame that is several GB of stack. Result: process death, every tenant. `Marshal` on a cyclic result has the same outcome with no budget at all.

Doc: **silent.** The ownership section mentions concurrent mutation as outside the promise; cycles and depth are not mentioned.

Fix: `MaxDepth` governs every traversal of a value (equality, ordering, `$string`, the descendant and transform operators, `$merge`, `Materialize`, `Marshal`, `Encoder`), raising `CodeBudget` before recursing past it. Add to the package promise: "A fatal runtime error (stack overflow, concurrent map access, out of memory) is not a panic and cannot be recovered; this package prevents the first and third by bounding depth and bytes, and the second only if the caller keeps the borrow." Cyclic values then become E1002 rather than a crash, and the doc can say so.

### 3. Integer tokens are unbounded at every decode point, and big-integer decimal parsing is quadratic. Severity: high.

`MaxIntegerBits` is described as bounding "a *big.Int result." `Unmarshal` decodes any integer token to `*big.Int`; `math/big`'s decimal `SetString` is quadratic in digit count; `Unmarshal` takes no ctx.

Attack: payload `{"id": 1111…1}` with 20 million digits (20 MB, under the 64 MiB byte bound). Decode time is on the order of minutes per request, not cancellable. The same applies to a `json.Number` the caller decoded with `UseNumber` (which the doc calls "also fine"), re-parsed on every read and not charged; to an expression literal of 256K digits (cheaper, but still refused-after-parsing rather than before); and to `$number($$.digits)` over a large payload string.

Doc: **silent.** `Unmarshal` names depth and size bounds only; `MaxIntegerBits` names results only.

Fix: refuse any integer token, `json.Number`, literal, or `$number` argument whose digit count exceeds the decimal capacity of `MaxIntegerBits` (⌈bits·log10 2⌉ + 1, about 1234 digits at the default) with `CodeBudget`, before parsing. Wording for `MaxIntegerBits`: "bounds the magnitude of every integer the evaluation holds, including one decoded from input text or read from a json.Number, and is enforced on the token's length before it is parsed." Also charge `json.Number` reads per byte as `RawMessage` reads are.

### 4. Carriage makes result size unbounded at serialization; `Marshal` and `Encoder` have no bound. Severity: high.

`MaxOutputNodes` counts a carried value as one node and does not charge its bytes. `EvalJSON` is "under the expression's Limits," which I read as catching this; plain `Marshal(v, env)` and `Encoder.Encode` are unbounded, and `Unmarshal` (which does state a default bound) makes the asymmetry conspicuous.

Attack: with a 1 MB payload:

```
[1..1000000].$$
```

Passes evaluation at exactly the default node bound with 8 MB of pointers; `Marshal` then attempts 1 TB. The same shape hurts anywhere the result goes next (a JSON encoder, a database driver, a log line).

Doc: **silent** for `Marshal`/`Encoder`; the `Limits` comment actively says carried bytes "are not charged," which is the attack.

Fix: `Marshal` and `Encoder` are bounded by `DefaultLimits().MaxBytes` and `MaxDepth` with `CodeBudget`, mirroring `Unmarshal`; add `MarshalLimits(v any, env *Env, limits *Limits)` or a `limits` field on `Encoder` for callers that compiled under other bounds. Add to "Getting values out": "The serialized size of a result is not bounded by the evaluation: a result may reference one carried value up to MaxOutputNodes times."

### 5. The regex dialect is the one pending ruling, and it is the ReDoS decision. Severity: high while pending.

If the answer is JavaScript semantics via a backtracking engine (needed for lookaround and backreferences), this is catastrophic backtracking under an 8M-unit budget that charges one call:

```
$match($$.s, /^(a+)+$/)     with s = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa!"
```

If the answer is Go `regexp` (RE2), matching is linear in subject × program, but the program can be large (256 KiB of source, "regex programs" charged to a 64 MiB byte bound) and the subject is an uncharged payload string, so a 100 KB alternation over a 60 MB string is still 1e13 steps under a one-unit charge. And `$eval("/" & $$.pattern & "/")` hands the pattern to the payload author, so Compile-time refusal is not the whole story.

Doc: **pending**, explicitly. Note the `$replace` paragraph already describes `$N` group semantics without saying which engine defines a group.

Fix: rule Go `regexp` (RE2 syntax) as the value model's regex, refuse unsupported constructs at Compile (and at `$eval`) with an S code, declare the divergence, and charge `len(subject)` plus program size per regex operation to `MaxWork`. Go's `regexp` already refuses repeat counts above 1000 and oversize nested repeats, which is exactly the "refuse rather than approximate" posture.

### 6. `MaxBytes` does not say whether it is cumulative or peak; if peak, repeated encoding is unbounded CPU. Severity: medium-high.

A `[]byte` is encoded "only when an expression observes it," carriage never encodes, and nothing says the encoding is cached per evaluation.

Attack, with `blob` a 30 MB `[]byte` and a base64 `BytesEncoding`:

```
$count([1..1000000].$length($$.blob))
```

If `MaxBytes` is cumulative, this fails on the second observation (good). If it is live/peak, it is 1M × 40 MB of encoding under budget. The same question decides `$reduce` string doubling, `$pad`, and `$join`.

Doc: **ambiguous.** "Bytes allocated by one evaluation … including intermediates" leans cumulative but never says it.

Fix: "MaxBytes is cumulative over the evaluation: every allocation is charged when made and nothing is credited back when it is dropped." Optionally memoize a `[]byte`'s encoding by identity within one evaluation.

### 7. Results alias `Env.Bindings`; the failure mode is a fatal error or cross-request data bleed. Severity: medium.

A service holds one `Env` with a `map[string]any` of context, evaluates concurrently, a handler adds a field to a returned `*Object` or map before encoding it, and the object was carried from the binding. Outcome: `fatal error: concurrent map writes`, or tenant A's field appears in tenant B's result.

Doc: **addressed**, correctly and prominently. But it is discipline, not a mechanism, and there is no helper: `Materialize` returns by identity when nothing is foreign, so it is not a copy.

Fix: `func Clone(v any, limits *Limits) (any, error)`: a bounded deep copy into caller-owned `*Object`, `[]any`, and fresh strings (which also solves the `Unmarshal` retention footgun the doc mentions). Guidance sentence: "In a service, treat every value in Env.Bindings as immutable for the life of the process; Clone a result before mutating it."

### 8. `Close` is self-contradictory about when the borrow ends. Severity: medium.

"The borrow … ends when Close returns and any call then in flight completes" names two different instants. If `Close` does not block, a caller that mutates its input after `Close` while another goroutine's `Select` is still walking it gets a fatal map race.

Doc: **addressed incorrectly.**

Fix: "Close blocks until every Select and Complete in flight has returned; when Close returns the borrow has ended." If it must not block, say so and require the caller to join its goroutines.

### 9. `Error.Value` and `$error` carry payload content into the error channel. Severity: medium.

`$error("k=" & $$.api_key)` or `$number($$.ssn)` (D3030 carries the value) puts payload content in `Error.Value`, which `slog.Any("err", e)` will print. The expression author can use this deliberately to move payload content into logs or error responses.

Doc: **addressed** (Error() does not render it; "a logger that walks the struct will see it"). Two gaps remain: `Token` for `CodeMalformedInput` is not said to exclude input text, and nothing states the redaction rule for services.

Fix: "Token is drawn from the expression source only; for CodeMalformedInput it is empty." And: "Value can hold payload or binding content; a service that logs errors or returns them to clients redacts or drops Value."

### 10. Cancellation interval wording. Severity: medium.

"Checked at intervals bounded by MaxWork units" reads as "once per MaxWork," which would make a deadline useless. Fix: "at least every 1024 units of work, and inside every operation whose cost scales with a value's size." Also state that `Compile` and `Unmarshal` are bounded by size so their lack of ctx is safe only once findings 3 and 5 are fixed.

### 11. Invalid UTF-8 flows through carriage into `Marshal`. Severity: medium-low.

A string that is not valid UTF-8 is refused only when observed; carried, it reaches `Marshal`, which is silent about it (the standard library would coerce to U+FFFD, which violates "refuse rather than approximate"). `Unmarshal` refuses surrogate escapes but does not say it validates raw bytes.

Doc: **silent** on both ends. Fix: `Unmarshal` refuses text that is not valid UTF-8 (E1006); `Marshal` refuses a string that is not valid UTF-8 (E1001).

### 12. Bindings are an exfiltration surface. Severity: low-medium (a consumer concern the API should name).

Everything in `Env.Bindings` is readable by name and can be carried into any result, including an outbound request body. If the binding layer puts context there, a document expression copies `$token` into a body. Doc: **silent.** Fix, one sentence on `Bindings`: "Every binding is readable by the expression and can appear in any result; bind only what the expression is entitled to see."

### 13. `$eval` has no explicit off switch; shadowing is an accidental one. Severity: low.

Binding `"eval": nil` disables `$eval` (T1006), which is stable because the reference defines shadowing, but nobody will find it. `$eval` is what lets the payload author choose the regex and defeat `Reads()`. Fix: either document the shadowing idiom as supported or add an explicit knob; I would not add API for it if finding 5 lands as RE2.

### 14. Small closures. Severity: low.

Binding names: only a leading "$" is refused; `""` and non-identifier names are unspecified (does `""` shadow `$`?). Validate against the identifier grammar with `CodeBinding`. `ArrayView.Len` negative: say it is E1001. `Env.Now` panic: say it propagates like Resolver. `$random`: say it is the top-level `math/rand/v2` generator (unpredictably seeded, not per-evaluation). Concurrent `Evaluation`: say whether a waiter can receive the computing goroutine's `context.Canceled` as a cached field error.

## What I would change

1. **Make `MaxWork` a CPU bound.** Wording in finding 1; charge per byte or per value visited in every size-proportional operation, including regex steps, equality, ordering, `$string`, encoding, decoding, and `Materialize`. This turns the budget from an allocation bound into the thing the doc already implies it is.

2. **Apply `MaxDepth` to every value walk and say what a fatal error is.** `MaxDepth` bounds "the nesting depth of a parsed expression, of a JSON text Unmarshal decodes, and of any traversal of a value by an operation, Marshal, or Materialize; a cyclic value exceeds it (CodeBudget)." Extend the package promise to name stack overflow, concurrent map access, and OOM as fatal and state which the package prevents.

3. **Bound integer tokens before parsing, and define `MaxBytes` as cumulative.** `MaxIntegerBits` enforced on token length at `Unmarshal`, `Object.UnmarshalJSON`, `json.Number` reads, literals, and `$number`; `json.Number` reads charged per byte. "MaxBytes is cumulative over the evaluation."

4. **Rule the regex dialect now: Go `regexp`.** Declare the divergence from JavaScript (no backreferences, no lookaround, RE2 escapes), refuse unsupported syntax at Compile and `$eval` with an S code, charge `len(subject)` and program size per call.

5. **Bound `Marshal`, block in `Close`, add `Clone`.** `Marshal`/`Encoder` under `DefaultLimits` with a `limits`-taking variant; `Close` blocks until in-flight calls return; `func Clone(v any, limits *Limits) (any, error)` for callers who need an owned result.

## What is done right

- The environment is genuinely closed: no function registration, values-only bindings, no host time zone, clock fixed once per evaluation and thus useless as a timer, `$random` labeled non-cryptographic, function values cannot escape an evaluation.
- Limits live on the `Expression`, not on each call, so a bound cannot be forgotten at one call site; every bound has a finite default; `Expression.Limits()` fills defaults so a cache keys on the truth.
- One budget across all selections and `Complete`, sticky after a budget error, and `$eval` inside the same budget: splitting does not buy work.
- `Unmarshal` refuses duplicate keys and unpaired surrogates and keeps integers exact: this removes the parser-differential class (two decoders disagreeing on an ID or a key) outright.
- "Refuse rather than approximate" is a security property: no silent 2^53 truncation of IDs, no saturation, no coercion of ill-formed strings.
- `Error()` never renders input-derived content; unsupported-value errors name the type, not the contents; errors are charged to the byte bound so `$error` cannot be used to allocate.
- ctx errors come back as `ctx.Err()`, engine panics are recovered to `CodeInternal` and close the `Evaluation`, Resolver panics propagate (the right call: it is the caller's code).
- No goroutines, no process-wide state, `Expression` and `Env` shareable: the concurrency story has no hidden lock.
- The Resolver doc says the right thing: allowlist over known types, never reflection; the expression can drive it with any key.
- The planner is itself bounded (`ReasonPlanBudget`), `RawMessage` is charged per byte, and "Select is not a guard" plus the prelude idiom is honest about the only way selective evaluation can bypass a check.

## On the concept

Host-native evaluation narrows and widens the surface at once. It narrows it by removing the second parse: a port that decodes into its own tree has two value models in the request path (the caller's decoder and its own), and every differential between them (duplicate keys, 2^53, surrogates, depth) is an ambiguity an attacker can stand on. This API resolves each of those once, at `Unmarshal`, and refuses rather than choosing. It widens it by reading the caller's structures by reference: cycles, mutation during the borrow, lazy views of unbounded depth, `json.Number` re-parsing, and result aliasing are all consequences of not owning a copy. The stub's answer, admission on first read and carriage without classification, is the correct performance choice and the wrong safety default in exactly the places I ranked highest: what is never classified is never bounded, and what is carried is never charged. The fix is not to copy; it is to charge and depth-bound every walk, so that the zero-copy model keeps its speed and loses its unbounded corners.

"Documentation as authority" leaves every security-relevant behavior undefined, and the stub is honest that it fills those gaps as "engine" refusals: the documentation says nothing about resource limits, error contents, number size, integer exactness, or regex dialect. That is fine as long as the ledger owns those rulings rather than leaving them pending, and as long as the doctrine never forces an unsafe behavior. I looked for a place where "documentation wins where it speaks" could compel something dangerous and found only one candidate: the documentation's regex examples assume JavaScript flavor, and a literal reading could be taken to require lookaround and backreferences. The value model must win there, and the ledger should say so before anything ships.

The regex analogy is apt precisely on the safety axis, and the authors should lean into it harder than they have. RE2's contribution was not speed; it was a guarantee: linear time in the subject, by construction, and refusal of the features (backreferences) that make the guarantee impossible. Go's `regexp` is the canonical "refuse rather than approximate" engine. A JSONata evaluator that claims the same lineage should make the same shape of promise: every operation's cost is bounded by the budget, by construction, and the constructs that cannot be bounded are refused. Today the API promises closure and allocation bounds and delivers them; it implies a CPU bound and does not deliver it. Close that gap and the analogy is earned.
