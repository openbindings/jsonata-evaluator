# Panel cold read: performance engineer

> 2026-09-18. Given only the class README and the vetted API stub; told not to read anything else. Verbatim.

## Grades

| Criterion | Grade | One-line justification |
|---|---|---|
| Idiomatic fit for Go | B+ | `regexp`-shaped Compile/MustCompile, immutable `Expression`, `ctx` on eval are right; `Close() error` with nothing to error, and an `Evaluation` that promises concurrency safety nobody asked for, are not. |
| Ergonomics of the common path | B | `Compile` once, `Eval(ctx, input)` is fine; the `(value, present, err)` triple is the correct price of undefined-vs-null and I would not remove it. |
| Correctness and footgun risk | B- | No default `WithMaxDepth`, no work budget, an `Evaluation` you must remember to `Close`, `json.Number` compared by value on every touch, and unspecified integer promotion widths are all latent surprises. |
| Performance headroom the API permits | B- | By-reference carriage and an immutable compiled expression permit a genuinely fast `Eval`; option closures, `[]string` paths, `Keys() []string`, a locked memo table, and no capacity hints tax `Select`, `Access`, and every options-bearing call. |
| Concept soundness | B+ | Host-native is the right value model for this workload (decode dominates; carriage is free); the regex analogy is apt for dispatch and wrong for bounding, because JSONata is not linear-time by construction and the API does not yet make it boundable. |
| Overall | B | Permits fast whole evaluation; the selective path and the option surface are where allocation and locking are baked in, and untrusted expressions cannot be bounded in memory. |

## Where the API forecloses performance

**1. Per-call option closures.** `Eval(ctx, input, opts ...EvalOption)` with `EvalOption func(*evalConfig)`: every call that passes `WithAccess(a)` or `WithBytesEncoding(enc)` allocates one closure per option plus one `evalConfig`, and the engine must re-resolve them. In OB, `Access` and `BytesEncoding` are constant for the lifetime of a binding; paying for them per request is pure waste. Nothing in the signature lets a caller hoist resolution.

**2. `Select(ctx, path ...string)` with memoization.** The variadic slice is stack-allocated only if it does not escape. The memo table must key on it, so it escapes (or the engine joins it into a string, which also allocates). Then a map insert. Then, because the doc says "safe for concurrent use", a mutex (or a `sync.Map`, which is worse for this write-then-read pattern) around every lookup. That is 3 to 4 allocations and a lock acquisition per `Select` before any evaluation, for a call whose whole reason to exist is to be cheaper than `Eval`.

**3. `Evaluation` concurrency promise.** "Safe for concurrent use" forces heap-resident, lock-protected memo state even for the 99% case of one goroutine selecting three fields. `Expression` being immutable and shareable is the concurrency contract that matters; `Evaluation` should be a plain single-goroutine value like `bufio.Reader`.

**4. `Access.Keys(v any) []string`.** Allocates a slice per object visit. Walking a proto message of 40 fields through `Access` for a `$keys` or `**` or `$merge` allocates on every object touched. `Kind(v)` is also called before every operation, so a value reached through `Access` pays two interface dispatches where an admitted value pays one type switch.

**5. `Object`: no capacity, unspecified `Get` complexity, allocating `Keys`.** An object constructor with N literal keys knows N at compile time; `NewObject()` cannot be told. If `Get` is backed by an index map, every constructed object is a struct + entries slice + map = 3 allocations and a hash per key. For the typical 4 to 20 key API object a linear scan over an entries slice beats a map and drops an allocation; the API should say which it is so callers and the engine can reason about it.

**6. `Reads() (paths []string, known bool)`.** Returns a fresh slice each call. Trivial per compile but it is exactly the kind of thing that gets called per request by a lazy-fetching caller. Compute at compile, return the shared slice, document immutability.

**7. `MarshalJSON` on `Object`.** `encoding/json` calls `MarshalJSON`, then re-validates and compacts the returned bytes into its own buffer. Nested `*Object` values pay that at every level, so a deep constructed result is copied and rescanned once per nesting depth. This is an `encoding/json` defect, not yours, but `text.Evaluate` must not encode through it.

**8. No work or intermediate-memory budget.** `WithMaxOutputNodes` bounds the result. `WithMaxRecursion` bounds the stack. Neither bounds `$count([1..100000000])`, which outputs one node, recurses zero deep, and allocates 800 MB of `[]any` plus 100M boxed ints before a deadline can fire. The context deadline is a wall-clock bound, not a memory bound; by the time it trips, the OOM killer may have already made the decision for you.

**Allocation estimate, common path, API as it stands.** For `{ "id": id, "name": profile.name, "active": status = "ACTIVE", "count": $count(items) }` over a decoded `map[string]any` via `expr.Eval(ctx, input)` with no options: a careful implementation is forced to about 4 to 5 allocations (frame, `Object`, entries slice, one boxed `int` for `$count`; the carried `id` and `name` and the static `bool` are free). A straightforward implementation that materializes a `[]any` sequence per path step will land at 15 to 25. With `WithBindings(m)` add 3 (closure, config, environment copy). Via `Prepare` + one `Select` + `Close`: add roughly 5 (Evaluation, memo map, escaped path slice, key, map entry) plus the lock. That last number is the problem: selective evaluation's fixed overhead exceeds the entire cost of evaluating a rename-only expression.

## Where the API enables performance

- **By-reference admission and carriage.** "Never copied on admission" and "same type, same identity" out means a 2 MB decoded payload costs zero to enter and zero to leave, and a field that only navigates costs a few map lookups. This is the single biggest performance decision in the design and it is correct.
- **Immutable, shareable `Expression`.** No locks on the hot path; a plan, constant-folded subtrees, precompiled regex literals, resolved standard-library function pointers, and the static `Reads`/`Plan` skeleton can all be computed once at `Compile`.
- **`any` out permits an internal unboxed scalar.** Nothing in the API requires intermediates to be `any`. An interpreter can carry a 24-byte `{kind, i64/f64, ptr}` register through arithmetic, comparison, and predicates, and box only when a value is stored into an output container or returned. That recovers most of what a canonical representation would give for scalars without copying the input tree.
- **`Range` on `Object` and `Index`/`Len` on `Access`.** Iteration without materializing key slices is possible for the constructed-object side.
- **`json.Number` carriage.** Large integer IDs pass through untouched with no parse. Exactness is free on the copy path.
- **Cooperative cancellation at boundaries, not per node.** "Function boundaries" is the right granularity. A `select` on `ctx.Done()` per AST node would cost 10 to 30% on a tight interpreter; `cancelCtx.Err()` takes a mutex and must never be on the per-node path.
- **Memoized `Select` feeding `Complete`.** When selection is used correctly, nothing is evaluated twice.

## Selective evaluation, honestly

What `Select` saves is exactly one thing: the evaluation of the sibling fields' subexpressions. It does not save decode (already done), does not save admission (free), and does not save navigation inside the selected field. So the saving is proportional to how expensive the *unselected* siblings are.

For the workload described, most fields are renames and conditionals. Their evaluation cost is nanoseconds. `Select` cannot beat `Eval` there; it loses, by the fixed overhead in item 2 above. `Select` wins when a sibling field does real work: `$sort`, `$distinct`, group-by, a mapping over a 10k-element array, string assembly over a large tree. In that case it wins by orders of magnitude, and it is worth having for exactly that case.

When it cannot apply at all, and these are common OB shapes:

- **Array-mapping constructors.** `items.{ "id": id, "name": name }` is the list-response transform. Its result is an array; Plan says `array-input`; every `Select` is a full evaluation. A caller wanting `id` from each element gets nothing. The `path ...string` model has no element wildcard, so this cannot be expressed even if the engine could do it.
- **`$merge`, `~>` pipelines, top-level function calls.** The README lists `$merge` in the portable core. `$merge([{...}, {...}])` has a dynamic shape; Plan says `unsupported-call` or `dynamic-shape`.
- **Block preludes.** `( $x := expensive; { "a": $x.foo, "b": $x.bar } )` is how anyone writes a transform with shared work. The package doc says selection applies "when the result is an object constructor with literal keys". If a block whose final expression is a constructor does not qualify, the most common non-trivial transform falls out. If it does qualify, the doc must promise that `$x` is evaluated once and shared across selections, or selection re-evaluates the prelude per field and is *slower* than `Eval`.
- **`$now()` and `$millis()`.** JSONata fixes the timestamp for the duration of an evaluation. Rule 9 (unobservable selection) therefore requires `Prepare` to snapshot the clock so that a field selected at T1 and `Complete` at T2 agree. The API does not say this.

Is `Plan` enough? For *mode*, yes: `Mode` and `Reason` tell a caller whether `Select` is served individually. For *deciding whether to bother*, no. A caller holding a document-supplied expression cannot know whether the siblings are cheap or expensive; `Plan.Fields` tells it which keys exist, not what they cost. Two options: expose a coarse static cost class per field (constant, navigation-only, input-linear, unknown), or say plainly in the doc that `Select` is for expressions the caller knows to have expensive fields and that `Eval` is the default. The second is honest and cheaper. For OB's transform layer I would write the rule as: use `Prepare`/`Select` only when `Plan.Mode == Selective` and the requested fields are fewer than half of `Plan.Fields`; otherwise `Eval`.

Cost model I would hold the implementation to: `Prepare` is O(1) beyond option resolution (the plan is computed at `Compile`; `Prepare` only checks the input's kind for `array-input`), with at most 2 allocations and zero evaluation. `Select` is one memo lookup plus the subexpression. `Complete` evaluates only the remainder and assembles the `Object` from memoized values by reference. `Close` returns scratch to a pool and is O(1).

## What I would change

1. **Add a work budget.** `func WithMaxWork(n int) EvalOption`, counting evaluated nodes plus sequence elements produced plus bytes of strings built, failing with `CodeBudget`. Removes: unbounded intermediate memory on untrusted expressions, the only bound today being wall clock. This is the one change I would block a release on.

2. **Resolve options once.** `func (e *Expression) With(opts ...EvalOption) *Expression`, returning a derived immutable expression with the options pre-resolved, and `Eval`/`Prepare` callable with no options. Removes: one closure per option and one config per call on every request in OB, where `Access` and `BytesEncoding` never change. (If the idiom reviewer prefers a plain `Options` struct pointer, that removes the same cost; either is fine, the current shape is not.)

3. **Resolve field handles at compile; drop the concurrency promise.** `func (e *Expression) Field(path ...string) (Field, bool)` resolved once against the constructor skeleton, and `func (v *Evaluation) Select(ctx context.Context, f Field) (any, bool, error)`. `Evaluation` documented as not safe for concurrent use; `Close()` with no error. Removes: the escaped path slice, the joined key string, the map insert, and the lock on every `Select`. Also lets a caller learn at compile time, before any input exists, whether the field it wants is selectable.

4. **Iteration instead of key slices on `Access`.** Replace `Keys(v any) []string` with `Range(v any, fn func(key string, value any) bool)`. Removes: one slice allocation per object visited through `Access`, which for proto-backed inputs is every object.

5. **Sized objects and stated complexity.** `func NewObject(capacity int) *Object`, `Get` documented as O(1) or O(n) (my recommendation: linear scan below 16 members, index map above), `Keys` documented as allocating. `Reads()` computed at `Compile` and returned as a shared immutable slice. Removes: one allocation per constructed object and the growth copies for the common 4 to 20 member case, plus per-call `Reads` garbage.

Not in the top five but needed: a default for `WithMaxDepth` (an unbounded recursive-descent parser over 256 KiB of untrusted `(((((` is a stack exhaustion with no budget), and a statement that `$eval` (documented in 2.1) is subject to the same compile limits and work budget as the outer expression.

## Benchmarks I would demand

All with `ReportAllocs`, `benchstat` over at least 10 runs, and an alloc profile attached. Inputs: a 4 KB single-object response and a 2 MB list response, both pre-decoded with `UseNumber`.

- **Identity.** `$` over the 2 MB input: 0 allocs, under 100 ns. Proves by-reference carriage is real.
- **Rename.** Five-field constructor of path renames over the 4 KB object: at most 6 allocs, under 1 µs. Proves singleton sequences do not materialize slices.
- **List map.** `items.{ "id": id, "name": name, "ok": status = "ACTIVE" }` over 10k elements: at most 3 allocs per element, under 300 ns per element, and total evaluation time under 25% of `encoding/json` decode time for the same bytes. If evaluation is not small relative to decode, host-native has not delivered.
- **Selective win.** Constructor with one cheap field and one `$sort` over 10k elements; `Prepare` + `Select(cheap)` must beat `Eval` by at least 50x, and `Prepare` + `Select(cheap)` + `Complete` must be within 5% of `Eval`. Proves memoization and proves `Select` is not re-evaluating the prelude.
- **Selective loss.** Same rename expression via `Prepare` + one `Select` + `Close` versus `Eval`: report the ratio honestly in the README. I expect `Select` to lose; the number tells callers when to use which.
- **`json.Number` sort.** `$sort` of 100k `json.Number` keys: number of `strconv` parses must be O(n), not O(n log n). Proves decorate-sort-undecorate.
- **Descendant walk.** `**.id` over the 2 MB input: linear in nodes, allocations bounded by result size, not by nodes visited.
- **Bounding.** `$count([1..1e9])` fails via `WithMaxWork` before allocating 64 MB; a runaway `$reduce` observes `ctx` cancellation within 1 ms; `Compile` of a 256 KiB maximally nested expression fails via depth in under 10 ms with bounded stack.
- **Concurrency.** Eight goroutines sharing one `Expression`: at least 7x scaling. Proves no hidden shared mutable state and no false sharing on the compiled tree.
- **`Object.Get`** at 8, 64, and 1024 members, and construction of the 8-member case: allocations per object and per key.
- **`text.Evaluate`** versus manual decode + `Eval` + encode: overhead at most 5%, and nested `*Object` encoding must not go through `MarshalJSON` recursion (allocations flat in nesting depth).
- **Against the incumbent Go port** on the reference fixture set: at least 2x faster and fewer allocations on the rename and list-map workloads, or the "native, not ported" claim is marketing.

## On the concept

Host-native values are the right call for this workload, and the reason is arithmetic, not philosophy. A 2 MB API response decodes to roughly 50k Go values; copying that into a canonical tree is 50k allocations and a few milliseconds, which is more than the entire evaluation of a typical rename transform by two orders of magnitude, and you pay it again on the way out. By-reference admission makes selection and reshaping cost only the lookups you actually perform. The price is dispatch: every operation type-switches over seventeen admitted types instead of seven kinds, and there is no slot on a host value to cache derived facts, so a `json.Number` is re-parsed every time it is compared, a string's rune count is recomputed every `$length`, and a large `map[string]any` cannot grow an index. For compute-heavy expressions (large sorts, group-bys over big arrays) a canonical representation would win by 2 to 5x. For the stated workload it would lose. The API leaves the implementation free to use an unboxed internal register for scalars and to decorate-sort-undecorate, which recovers most of the gap where it matters; it should say so in its benchmarks.

The regex analogy holds at the dispatch layer and breaks at the bounding layer. Go's `regexp` is fast *and* safe because RE2 is linear by construction; a hostile pattern cannot make it slow, only fail to compile. JSONata is not linear by construction. It has recursive lambdas, `$map`/`$reduce` over sequences it can also generate, a range operator that manufactures arrays from two integers, a descendant operator that walks the whole input, cartesian products from chained path steps, and `$eval` that compiles at runtime. That is a small functional language, not a notation, and a small functional language over untrusted input needs budgets, not clever automata. Host-native does buy one real safety win here: JSONata regex literals go through Go's linear-time engine, so the classic catastrophic-backtracking pathology disappears (at the cost of a declared divergence for lookaround and backreferences).

The pathological case, then, is not time but memory: an expression that produces a huge intermediate and a tiny result. `$count([1..1e8])`, `$join($map([1..1e6], function($i){ $pad("", 1000) }))`, or `$sum(a.b.c.d)` over a many-to-many chain. `WithMaxOutputNodes` sees one node, `WithMaxRecursion` sees depth one, and the context deadline fires after the heap has already grown by gigabytes. As drafted, the API does not let a caller bound this. A work budget that counts nodes evaluated and elements materialized closes it, and once that exists the analogy becomes fair: the engine is fast for the common case, refuses the pathological one, and the caller can state the bound in one option, which is exactly how one would describe a good regex engine.

On the compile cache: it belongs in the caller. Expressions in OB are document-supplied, so a global cache in the library is an unbounded-memory attack surface keyed by attacker-chosen strings. The library's job is to make `Expression` compact and immutable (done), keep `Compile` linear and cheap, and expose `Source()` so a bounded LRU keyed by source plus compile options is trivial to write at the transform seam. It should say this in the package doc so no one adds a `sync.Map` of expressions to the library later.
