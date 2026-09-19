# Iteration 1 cold read: security and sandbox

> Given only the class README and go/jsonata + go/text at feb3743; told not to read design/. Verbatim.

## Grades

| Criterion | Grade | One-line justification |
|---|---|---|
| Idiomatic fit for Go | B | Compile/MustCompile/String mirror `regexp` well and options travel with the artifact, but `Compile` has no `ctx` while the doc promises cancellation "at compile", and `Access` methods cannot return errors, which is un-Go and (below) unsafe. |
| Ergonomics of the common path | B | `(value, present, err)` is the right shape and `With` is the right per-binding shape; the cost is that every consumer of "an object" writes a two-arm switch, and `Eval` cannot tighten compile bounds at all. |
| Correctness and footgun risk | C | Four independent ways for a predicate to lie or a result to be corrupted: `Access` absence-on-failure, nil slice as null (`$count(null)` is 1), memoized fields handed to the caller as "owned", and `Select` succeeding where `Complete` fails. |
| Performance headroom the API permits | B+ | By-reference admission, memoized selection and `Reads()` give real headroom; the same by-reference design leaves the memory bound open, so the headroom is currently paid for with an unbounded allocation surface. |
| Concept soundness | B- | Documentation-as-authority is right for semantics; for regex it collides head-on with the host engine, and "host-native values" moves the sandbox boundary into the caller's `Access`, which the README does not acknowledge. |
| Overall | B- | The bounds section makes the strongest claim in the file ("no expression, whatever its content, can terminate the process") and the bounds as specified do not deliver it; everything else is fixable in wording and one added option. |

## Threat model

- **Expression author**: a third party. The expression arrives inside an interface document that the caller fetched or was handed; it is compiled and evaluated inside the caller's process on the request path. Assume the author is hostile and patient: they can iterate offline against this exact library.
- **Input author**: a second, possibly different, third party. The input is a decoded API payload (request or response) and may contain values the expression author never sees directly but can probe (`$lookup`, `**`, `$keys`). Large integer IDs, arbitrary strings, arbitrary nesting, and, through `Access`, arbitrary Go structs.
- **Evaluator operator**: the OpenBindings runtime (ob, an SDK consumer, or a Panjir-style service) acting for many tenants in one process, with one compiled `Expression` per document and possibly one `Expression.With(...)` per binding shared across all requests.
- **What the attacker wants**, in descending order of likelihood:
  1. **Take the process down** (OOM kill, stack overflow, fatal runtime error, engine panic). One expression, one request, whole-process outage for every tenant.
  2. **Burn CPU or memory below the kill threshold** so the request path degrades for everyone else (the regex-DoS shape: legal, bounded-by-count, unbounded-by-bytes).
  3. **Make a predicate lie**: a transform used as a filter or guard (`$$.owner = $auth.user`, `$count($$.roles) = 0`) yields the permissive answer on a path the author engineered (absence, null, refusal).
  4. **Corrupt or read across requests**: a shared binding or memoized value carried by reference into a result that another request or the caller mutates.
  5. **Exfiltrate through a side channel**: the error channel (logs, telemetry, error responses) carrying `Error.Value` or a value-bearing `Message` to a party who does not see the result.
  6. **Drive the caller's `Access`** to do work or reach data it would not otherwise do: lazy loaders, reflection over structs, ORM relations.

## Attacks against the API as written

**1. String doubling and quadratic replace (memory).**
Expression: `( $s := $pad("a", 262144); $replace($s, "a", $s) )`. Two nodes of work, a 64 GiB result. Variants: `$join($map([1..1000000], function(){ $s }))`, `$pad("x", 2000000000)`, `$formatNumber(1, $pad("0", 200000))`, repeated `$x & $x`.
Targets: `WithMaxWork` ("counting nodes evaluated and elements materialized") and `WithMaxOutputNodes` ("counting every value"), and the headline "no expression can terminate the process". A string is one element and one value; its length is unaccounted.
Stopped? No. Nothing in the API bounds bytes.
What would: a byte budget checked *before* allocation (see change 1).

**2. Big integer growth (memory and CPU).**
Expression: `( $f := function($x, $n){ $n = 0 ? $x : $f($x * $x, $n - 1) }; $f(18446744073709551616, 40) )`, or in one call `$power(18446744073709551616, 100000000)`. The Numbers section is explicit that "integer with integer yields ... *big.Int when either operand is big" and bounds only the int64 case. Each squaring doubles the bit length; thirty squarings is 8 GiB; `big.Int` multiplication at that size is also seconds of CPU inside one node with no ctx check.
Targets: the D1001 promise ("never a wrap and never a promotion") is stated for int64 but the big lane is unbounded.
Stopped? No.
What would: a magnitude cap on `*big.Int` results (bit length), refused as D1001, plus the byte budget.

**3. `$eval` as a compile amplifier.**
Expression: `( $src := $pad("1+", 131072) & "1"; $map([1..100000], function(){ $eval($src) }) )`. "Compiles its argument under the same bounds as the enclosing expression" says each `$eval` gets the *same limits*; it does not say they share the *same budget*. 100,000 compiles of a 256 KiB source is 25 GB of parsing; with fresh per-compile counters, `MaxWork` is trivially multiplied.
Targets: "the bounds travel with the compiled expression so they apply wherever it is evaluated."
Stopped? Ambiguous, which for a sandbox means no.
What would: state that a top-level `Eval` or `Evaluation` has exactly one budget, that `$eval` draws from it, and that parsing is charged to it by source byte.

**4. `Access` as a traversal engine.**
Expression: `$$.**.password`, `$$.**`, `$lookup($$, $$.k)`, `$keys($$.**)`. Every call goes to the caller's `Access.Get`/`Range`/`Index` with attacker-chosen keys and no bound on call count, no `ctx`, and no error return. The `Reads()` doc explicitly invites lazy fetching; an `Access` over an ORM, a proto with lazy sub-messages, or `reflect` over a struct will be walked to exhaustion. A self-referential `Access` (Get returns its receiver) plus `**` only halts if traversal is charged to `MaxWork`, which is unstated.
Targets: "No expression can reach the host." The host is reached through the one door the API opens and the doc says nothing about guarding it.
Stopped? No. The engine cannot stop it; the doc does not even tell the `Access` author they are the authorization boundary.
What would: `ctx` and `error` on `Access` methods, a work charge per `Access` call, and doc text that `Access` is an allowlist, never reflection.

**5. `Access` failure is indistinguishable from absence (predicate flip).**
`Get(v, key) (value any, ok bool)`: "ok == false is absence, not null." A backing store timeout, a permission error, or a nil-pointer dereference guarded by the `Access` author all have exactly one honest return: `ok = false`. Expression: `$$.locked ? "deny" : "allow"`. Absent is falsy; the result is `"allow"`. `$not($$.locked)` is undefined; `$$.owner = $auth.user` is false; `$exists($$.suspended)` is false. Every one of these fails open.
Targets: rule 8, "refuses rather than approximates." The interface forces the `Access` author to approximate.
Stopped? No, structurally.
What would: `Get(ctx, v, key) (any, bool, error)` and the same for `Index`, `Range`, `String`, `Bool`, `Bytes`, `Len`. `Number` already has the error; the asymmetry is the tell.

**6. Nil slice is null and `$count(null)` is 1.**
Input: a struct field `Roles []string` that was never set, reached through `Access`, or a caller-built `map[string]any{"roles": []any(nil)}`. The doc: "A typed nil pointer or nil slice or map of those types is null." Expression: `$count($$.roles) = 0 ? "guest" : "member"`. JSONata counts a non-array value as a singleton, so `$count(null)` is 1 and the user is a member. `$$.roles = []` is false; `$exists($$.roles)` is true; `$$.roles[0]` is null. In Go, nil and empty slices are interchangeable everywhere except `encoding/json`, and this API inherited the one place they are not.
Targets: rule 5 "comparison is by value" and every authorization-shaped predicate over a collection.
Stopped? No; it is the documented behavior.
What would: admit a nil `[]any` as the empty array and a nil `map[string]any` as the empty object; only untyped nil and typed nil pointers are null. If the project insists on `encoding/json` parity, the doc must carry this exact `$count` example as a warning.

**7. `Select` succeeds where `Complete` fails (guard bypass).**
Expression: `{ "guard": $assert($$.authorized, "forbidden"), "data": $$.payload }`. The doc states, in its own words, "when Complete would fail, Select may still succeed for a field that does not depend on the failing subexpression." So `Select(ctx, "data")` returns the payload. Worse: `( $assert($$.authorized); { "data": $$.payload } )`. The doc says prelude *bindings* are evaluated once and shared; it does not say prelude *statements* are evaluated at all. If the planner treats a non-binding prelude expression as dead code, the transform author's guard is silently dropped by the runtime's choice to select.
Targets: rule 9, "selective evaluation, if offered, is unobservable." This is an observable difference and it is the security-relevant one.
Stopped? No; it is specified.
What would: every prelude statement is evaluated before any `Select` and its failure fails every `Select`; and the doc must state that `$error`/`$assert` in a sibling field is not a guard under selection, so the OpenBindings caller knows it must `Complete` when the transform's failure is meaningful.

**8. Memoized fields handed out as "owned" (result corruption, cross-caller sharing).**
Sequence: `v.Select(ctx, "a")` returns a constructed `*Object`; the doc says constructed values "are owned by the caller"; the caller mutates it (adds a field, redacts one); `v.Complete(ctx)` "reuses every field already selected" and returns the mutated object inside the whole result. Two concurrent `Select`s of the same field return the same pointer to two goroutines, and `*Object` "is not safe for concurrent mutation."
Targets: the ownership sentence in Values, and "an Evaluation is safe for concurrent use."
Stopped? No; the two sentences contradict each other.
What would: "Values returned by Select or Complete are shared with the Evaluation until Close; the caller must not mutate them while it is open." Or copy on hand-out, which costs the memo its point.

**9. Shared bindings carried into results (cross-request corruption).**
`expr := compiled.With(WithBindings(map[string]any{"config": cfg}))`, shared across all requests as the doc recommends. Expression: `{ "config": $config, "user": $$ }`. The result carries `cfg` by reference ("same type, same identity"). The HTTP layer mutates the response object (adds a request ID). Every subsequent request, every tenant, sees it.
Targets: "Constructed values ... are owned by the caller" reads as if the `*Object` is safe to mutate; its members are not.
Stopped? No.
What would: the ownership sentence must say members of a constructed value may be carried; and `WithBindings` should say "values you bind may appear in results by identity."

**10. Error channel as exfiltration and amplification.**
Expression: `$error($$)` puts the entire input in `Error.Value`; `$error($pad("x", 1000000000))` puts a gigabyte there, outside `MaxOutputNodes`. The jsonata-js messages this API inherits by code (T1003, D3010, D3137, T2001 family) embed the offending value in `Message`. `Error.Token` for D3030 is a `json.Number` token from the *input*, which can be megabytes. `CodeUnsupportedValue`'s message, if it uses `%v`, dumps a struct.
Targets: "closed environment" in the one direction the doc never considers: outbound, to logs and error responses that reach a different party than the result does.
Stopped? No.
What would: `Error()` renders code, message, position; `Message` never embeds input-derived content; `Value` is charged to the byte and node budgets; `Token` is truncated; unsupported-value errors carry `%T`, never `%v`.

**11. Regex.**
Go's `regexp` is RE2, so `(a+)+$` over `"aaaa...!"` is linear and this is the one classic attack the API stops by construction. What remains: (a) the JSONata documentation describes JavaScript regex; lookaround and backreferences will not compile, which is correct for safety but must be a declared divergence at Compile with a stable code; (b) `$match($s, /(?:)/)` and `$split($s, "")` over a 256 KiB string yield one match per character, fine if charged as elements; (c) runtime regex through `$eval("/" & $$.pat & "/")` is bounded by RE2's program-size limits per compile but multiplies under attack 3; (d) `(?i)` with large Unicode classes is a compile-cost spike that only the byte budget catches.
Stopped? Mostly, and for the right reason. Charge compile to the budget and it is closed.

**12. Input-controlled numbers.**
Input: `{"id": 1000...0}` with a million digits. `json.Number` classification on first use is `big.Int.SetString` on a megabyte, then `$$.id * $$.id` is attack 2 with the payload author, not the expression author, as the attacker. Also `"x": 1e999999999` must become D1001 or D3030, never `+Inf`; the doc says so, good.
Stopped? Only by the missing byte budget.

**13. Time is not bounded by work.**
`MaxWork` 8M with a `big.Int` multiply, a regex over a 256 KiB string, or a `BytesEncoding.EncodeToString` of a large blob as the unit is minutes, not milliseconds. ctx is checked "at function boundaries," so one long builtin is not interruptible. A naive caller passing `context.Background()` has no time bound at all.
What would: ctx checks inside long builtins by chunk, or unit costs that scale with bytes, and a doc sentence that `MaxWork` bounds work, not time.

**14. Timing oracle through `$millis`.**
`Prepare` fixes the timestamp. `Eval` does not say it does. If two `$millis()` in one `Eval` differ, an expression can time an `Access` call (cache hit versus miss, row exists versus not) and encode the result in its output. Fix the timestamp at `Eval` start too, as jsonata-js does.

**15. Fatal, not panic.**
"Callers must not mutate the input while an evaluation is running." A concurrent map read and write in Go is a fatal runtime error, not a recoverable panic, and `Evaluation` lengthens the window to "until Close." Not expression-triggered, but the API design makes the caller's bug process-fatal. The doc must say the input is borrowed until `Close`.

## The "closed environment" claim, audited

Every path in or out, as the API is written:

- **Input, by reference.** Read-only by contract, not by mechanism. The result aliases it ("same identity"), and subslices from range predicates (`$$.arr[[1..3]]`) are unstated: if they are Go subslices, a caller's `append` on the result overwrites the input's next element. Outbound aliasing hazard.
- **Bindings, by reference, and shared across requests through `With`.** Values bound once are carried into any number of results by identity. Outbound aliasing hazard with cross-request blast radius.
- **`Access`.** The one deliberate door. Nine methods called with attacker-chosen keys and indices, an attacker-chosen number of times, concurrently, with no `ctx`, and with no way to say "I could not read this" except "it is absent." Returned values are routed back through it, so a misbehaving `Access` (Number returning a struct) must be cut off after one hop or it loops. `Bytes` returns by reference, so the caller's internal buffers can land in results. Everything the caller's `Access` can reach, the expression can reach. The README's rule 7 ("no expression can reach host state") is true only of the built-in value set; the doc should say that `Access` relocates the boundary into the caller's code.
- **`BytesEncoding.EncodeToString`.** Caller code invoked with input bytes, as many times as the expression uses the value as a string, with no memoization stated. A capability the doc treats as a pure formatting choice is a callback.
- **`Error.Value`, `Error.Message`, `Error.Token`.** The only channel by which an expression emits data that is not the result. It is unbounded by `MaxOutputNodes`, uncharged by `MaxWork`, and typically lands in logs. Inherited jsonata-js message templates embed values.
- **`$eval`.** Adds no capability, but defeats `Reads()` (must report `known == false`), multiplies compile cost, and turns input text into expression text, which is the same trust class here but must be said.
- **Time and randomness.** `$now`/`$millis` expose wall clock; inherent. `$fromMillis` with a timezone picture must never consult `time.Local`; the default must be UTC or the host's configuration leaks. `$random`'s source is unstated.
- **Locale.** Sorting by code point and Unicode simple case mapping are host-independent. Good; this is the one place the doc closes a host-state leak explicitly.
- **Panics and fatal errors.** An engine panic on an adversarial expression is process death and breaks the strongest claim in the file; nothing in the API says whether `Eval` recovers. An `Access` panic propagates and is the caller's fault, which the doc should scope explicitly. Concurrent map mutation is fatal and unrecoverable.
- **Goroutines and globals.** Unstated. A sandbox doc should say: the engine spawns no goroutines and keeps no process-global caches keyed by expression content.
- **`Reads()` shared slice.** "Callers must not modify it": one careless caller corrupts every other caller's view of the same compiled expression. Minor; return a copy or an iterator.

Net: the claim holds for the built-in value set and the language proper. It does not hold for the API as a whole, because `Access`, `BytesEncoding`, and `Error` are all crossings, and the doc describes none of them as such.

## What I would change

1. **Add a byte budget and a big-integer cap.**
   `func WithMaxBytes(n int) CompileOption` "bounds the bytes allocated by one evaluation for strings, []byte, encoded byte forms, *big.Int, `$eval` source, and regex programs, including intermediates and Error.Value. Checked before allocation. Default 64 MiB."
   And in Numbers: "A *big.Int result whose bit length exceeds 4096 is an error (D1001)."
   Closes attacks 1, 2, 10 (amplification), 12, and the byte half of 3 and 11.

2. **One budget per evaluation, and say what is charged.**
   Bounds section: "An Eval, or an Evaluation across all of its Selects and Complete, has one budget. `$eval` compiles and evaluates within it. Parsing is charged per source byte; every Access call, every BytesEncoding call, every comparator invocation in `$sort`, and every value visited by `**` is charged as work."
   Closes attack 3 and the loop half of 4.

3. **Give `Access` a context and an error.**
   ```go
   Get(ctx context.Context, v any, key string) (value any, ok bool, err error)
   Index(ctx context.Context, v any, i int) (value any, ok bool, err error)
   Range(ctx context.Context, v any, fn func(key string, value any) bool) error
   Len(ctx context.Context, v any) (int, error)
   String(ctx context.Context, v any) (string, error)
   Bool(ctx context.Context, v any) (bool, error)
   Bytes(ctx context.Context, v any) ([]byte, error)
   ```
   Doc on the interface: "Access is the boundary of the closed environment. An expression can call it with any key, in any order, any number of times within the work bound. Implement it as an allowlist over known types; never with reflection over arbitrary structs. A value it returns that it then classifies KindUnknown is CodeUnsupportedValue, not re-consulted." Closes attacks 4 and 5.

4. **Make selection honest about guards and ownership.**
   `Prepare`/`Select` doc: "Every statement of a block prelude is evaluated before the first Select, and a failure there fails every Select. A failure in one field's subexpression does not fail another field; `$error` or `$assert` in a sibling field is therefore not a guard under selection." Values section: "Values returned by an Evaluation's Select or Complete are shared with the Evaluation until Close and must not be mutated before then. A constructed value may carry input or binding values as members; only the container is new." Closes attacks 7, 8, 9.

5. **Error hygiene.**
   `Error` doc: "Error() renders Code, Message, and Position. Message never includes input-derived content; the offending value, when the language defines one, is in Value only. Value is charged to the output and byte budgets. Token is at most 64 characters. Unsupported-value errors name the Go type, never its contents." Closes attack 10.

Not in the top five but I would also: admit nil `[]any` and nil `map[string]any` as empty aggregates (attack 6); fix the timestamp at `Eval` start (14); make `text.Evaluate` encode output bytes with the evaluation's `BytesEncoding` (today `{ "a": $b, "b": $string($b) }` renders two different strings for one value when bytes arrive via bindings or `Access`); and give `Compile` a `ctx` or drop the "at compile" cancellation claim. Also, comparing `[]byte` to `[]byte` "by bytes" but `[]byte` to `string` by encoded form is not a consistent order under base64 (the alphabet is not byte-order preserving), so a mixed `$sort` has an inconsistent comparator; compare bytes by encoded form throughout or declare bytes unorderable.

## What I would require before production

- **The bounds test that matters**: a fuzz harness that runs arbitrary expressions (grammar-guided, seeded with every builtin) over arbitrary inputs under the default bounds with a 256 MiB `GOMEMLIMIT` and a 2 s ctx, and asserts the process is alive, RSS never exceeded the limit, and every failure is an `*Error` or `ctx.Err()`. Run it in CI with a corpus that includes attacks 1, 2, 3, 12 verbatim.
- **No `recover` in the engine that masks its own panics**, and a fuzz corpus large enough that the claim "no expression can terminate the process" is evidence-backed. A recover that converts panics to `CodeBudget` is a bug hider, not a sandbox.
- **Budget accounting is incremental**: `[1..1000000000]` must trip `MaxWork` at 8M elements, not allocate then check. Same for `$pad`, `$replace`, `$join`, `$split`, `$string`.
- **ctx checked inside every builtin whose cost scales with input bytes** (`$replace`, `$match`, `$split`, `$string`, `$join`, `$pad`, `$sort`, big arithmetic, `$eval` parse).
- **Defaults tuned for a request path, not a script**: I would ship `MaxExpressionBytes` at 64 KiB, `MaxWork` at 1M, `MaxBytes` at 16 MiB, and let a caller raise them. A document-supplied transform that needs 8M work units is a bug.
- **A conformance case for every declared divergence from the reference suite**, and a separate declared-divergence list for regex flavor (no lookaround, no backreferences, Unicode class differences) with a stable compile-time code.
- **Aliasing tests**: no result slice shares a backing array with an input slice unless it is the input slice itself; `$append`, `$sort`, `$reverse`, `$shuffle`, `$distinct`, range predicates all return fresh slices; results from two concurrent `Select`s of the same field are documented as shared, or are copies.
- **Predicate tests**: `Access` error path, nil slice, nil map, typed-nil pointer through `Access`, absent binding versus nil binding, `[]byte(nil)` versus `[]byte{}`, each asserted against an authorization-shaped expression with the fail-closed answer expected.
- **Error content tests**: no `Error()` string contains any input string; `Token` length bounded; `Value` counted against budgets.
- **Race detector on the concurrent `Evaluation` and shared `With` expression paths**, with an `Access` that asserts it is never called after `Close`.
- **A written statement from the OpenBindings caller** of when it uses `Select` versus `Complete`, and that a transform's error is only trusted from `Complete` or from a prelude failure.

## On the concept

The regex analogy is a promise this library can keep only on the CPU axis, and only because Go happened to ship RE2. The regex world learned its lesson the hard way: backtracking engines are the canonical pathological-input DoS, and the fix was an engine design (linear-time automata), not a bound. JSONata has no such design available; it is a functional language with recursion, higher-order functions, string building, and arbitrary-precision integers in this member, so the sandbox must come from budgets. The doc understands this and reaches for five of them, but every one counts things, and a language that can build a 64 GiB string in two nodes is not bounded by counting nodes. The analogy is honest about compile-once and about "each host declares what it does not support"; it is silent about the fact that `regexp` never calls back into the caller and never returns anything that aliases the input, which are the two properties that make regex trivially sandboxed and that this API deliberately gives up.

"Host-native values" narrows the surface in one direction and widens it in another. It narrows it by removing a serializer and a parser from the request path, which removes an entire class of bugs (encoding confusions, duplicate keys, depth limits, number precision loss) and, more importantly, keeps the large integer IDs exact instead of laundering them through float64, which is exactly the property OpenBindings needs. It widens it by putting the caller's memory inside the evaluator's reach: results alias inputs, bindings are shared by identity across requests, memoized fields are handed out and reused, and `Access` is an open-ended callback interface that the expression drives with attacker-chosen arguments. A canonical internal representation would have made the environment closed by construction, at the cost of a copy. This design chose no copy, which is defensible, but then the closed-environment claim is a claim about the caller's discipline, not the engine's, and the doc should say so plainly instead of stating it as a property.

Documentation-as-authority is the right stance for the language and the wrong stance for regex, and the doc should split the two explicitly: the JSONata documentation governs what `$match` returns; the host's `regexp` governs what a pattern means and what it costs, and unsupported constructs fail at `Compile` with a declared code. Stated that way, this member's regex story is actually *stronger* than the reference implementation's, because RE2 is the safe engine and JavaScript's is not. As written, it reads as if the documentation owes JavaScript regex semantics, which this member cannot deliver and should not want to.
