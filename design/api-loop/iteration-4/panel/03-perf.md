# Iteration-4 panel: performance engineer

Cold read of `e57816b`. Rotating lens: the engineer asked to make this the fastest JSONata evaluator under this contract.

## Grades

| Criterion | Grade | One-line justification |
|---|---|---|
| Idiomatic fit for Go | B+ | regexp-shaped Compile/MustCompile, `any` values, `iter.Seq`, ctx and error on view calls are all cheap under the register ABI; the only unidiomatic-for-speed item is a concurrency-safe memo object nobody asked for. |
| Ergonomics of the common path | A- | Compile once, Unmarshal, Eval with the triple return, Marshal; nothing on that path forces the caller to wrap, copy, or convert. |
| Correctness and footgun risk | B | "Read" versus "carried" is never defined, `json.RawMessage` "decoded on first read" has nowhere to cache, and a shared budget under concurrent Select has order-dependent failure semantics the doc does not admit. |
| Performance headroom the API permits | B+ | The rename hot path can be three allocations and zero locks; four doc promises push it above that, and one (concurrent Evaluation) puts atomics in the inner loop unless carefully dodged. |
| Concept soundness | A- | By-reference carriage over host values is the right speed thesis; the value model's cost is in switch width and dual object types, not on the hot path. |
| Overall | B+ | A contract I can win under, after five rewordings and one dropped promise. |

## The hot path, costed

Assume the compiled node for `{ "id": user_id, "name": display_name }` holds its two keys pre-interned, capacity 2, and a static plan. Per-evaluation frame: budget counters (three ints), the root, the `Done()` channel captured once, bindings slots. Whether the frame is heap or stack is the first fork: with closure-compiled nodes or interface-dispatched `eval` methods, escape analysis heaps the frame (1 alloc, or a `sync.Pool` Get/Put pair, ~20 ns, lock-free in the common case); with a single switch-dispatched `func (fr *frame) eval(n *node, in any)` walker the frame stays on the stack (0 allocs). I would take the switch walker for exactly this reason.

**Over an `Unmarshal`ed body (root `*Object`, 20 members)**

| Step | What the API forces | Allocs | Locks |
|---|---|---|---|
| Enter Eval | `defer recover()` (open-coded, ~1 ns); `time.Now()` because "Eval fixes the timestamp at its start" (~30 ns vDSO, no alloc); `ctx.Done()` once (nil for Background) | 0 | 0 (a first `Done()` on a cancelCtx lazily allocates its channel under the ctx's mutex, once per context, not per Eval) |
| Classify root | type switch, arm `*Object`, nil check | 0 | 0 |
| Constructor: root is not an array, no grouping | `NewObject(2)`: one allocation if `Object` is laid out as `struct{ entries []entry; index map[string]int32 }` and the constructor allocates `struct{ Object; buf [2]entry }` and slices `buf` into `entries` (interior pointers are fine for the GC); lazy `index` for n ≤ 8. Charge MaxOutputNodes 1, MaxWork 1. | 1 | 0 |
| Field "id": path step `user_id` | `Object.Get`: the 20-member object has an index map, so one Go map lookup (~20 ns). Then the sequence rule: a path step result must be inspected for "is it an array of one" (singleton unwrap), which is one type switch on the member value. Carried by identity, no classification of its type. Charge MaxOutputNodes 1. Append to entries. | 0 | 0 |
| Field "name" | same | 0 | 0 |
| Return | `any(*Object)` is pointer-shaped | 0 | 0 |

Forced total: 1 alloc (2 if entries are a separate slice, 3 with a heap frame), 0 locks, ~5 budget decrements, one `time.Now`. Floor: the same 1 alloc; the result object is the only thing that must survive the call. The only avoidable costs the API forces here are the clock read and, if the doc means what it might mean, UTF-8 validation of each carried string (see Promises).

**Over `map[string]any`**

Identical shape. The path step is a Go map lookup (~20 to 30 ns for a 7-byte key). The sorted-order rule does not fire because nothing observes member order. Result is a fresh `*Object` regardless of the input's kind, which is the right asymmetry: the input type never leaks into result construction. Total: 1 alloc, 0 locks. If the input were `map[string]string` instead, the reflect fallback runs: `reflect.Value.MapIndex` takes the `mapaccess_faststr` path (no alloc) but `.Interface()` on a string element allocates a 16-byte header, so 1 alloc per member read. That is acceptable for an exotic input, and the ordering "exact type first" keeps the common concrete types out of reflect.

**Over a protobuf message through a Resolver**

| Step | What the API forces | Allocs | Locks |
|---|---|---|---|
| Classify root | type switch misses, `Resolve(ctx, root)`: interface call, charge MaxWork 1. The Resolver returns an `ObjectView`. If the view is `struct{ m *pb.User }` (single pointer field) it is pointer-shaped and boxes into the interface without allocating; if it wraps `protoreflect.Message` (an interface, two words) it allocates. The doc should tell Resolver authors this. | 0 to 1 | 0 |
| Field "id" | `view.Get(ctx, "user_id")`: interface call, the Resolver maps the key to a field (its own switch or map), reads the int64, boxes it to `any`: allocates unless the value is under 256. | 1 | 0 |
| Field "name" | `Get` returns a string boxed to `any`: `convTstring` allocates the header. | 1 | 0 |
| Result object | as above | 1 | 0 |

Forced total: 3 to 4 allocs, 0 locks. The boxing of scalars is Go's `any` tax, inherent to a host-native value model with `any` at the boundary; a port through an intermediate tree would pay it too, plus the tree. ctx and error on `Get` cost nothing measurable: two words in, two extra words out, all in registers. Charging "the bytes it returns to MaxBytes" is the one line here that cannot be implemented as written for a view (see Promises).

Note what does not happen in this case and must not: `$.user` carried into a result never resolves `user`; resolution occurs only at the point of descent (`$.user.name` resolves `user` to read `name`). That keeps "carried foreign values return as the original, never the view" free: the engine never needs to hold an (original, view) pair, because the pair only exists on the stack at the descent site.

## Promises that cost

1. **"Admission is per value, on first read" without defining read.** If carriage is a read, every carried string pays `utf8.ValidString` (O(len)) and every carried `json.Number` pays a parse, turning the rename hot path from O(fields) into O(bytes carried). Cheapest keeping: carriage inspects nothing. Wording: "Carriage neither classifies nor validates: a value the expression only selects, copies, or rearranges is not inspected, so an invalid string or a foreign value can pass through unread and unresolved. Only an operation that observes a value's content classifies it."

2. **"json.Number is classified each time it is read (holds no cache)."** Honest and fine for int64 tokens (ParseInt, ~15 ns) once carriage is exempt. But an integral token beyond int64 allocates a `*big.Int` (two allocations) per observation, and `$string(n)` parses then re-renders a token that is already the answer. Keep the wording; specify that `$string` and `Marshal` of an integral `json.Number` emit the token verbatim, which they already do for Marshal.

3. **"json.RawMessage is decoded as JSON on first read."** "First" promises a cache, and a `[]byte` has nowhere to carry one. Keeping it means a per-evaluation side table keyed by `(&raw[0], len)`: a map on the frame, lazily allocated, one lookup per read. That is affordable but it is state the doc does not mention. Cheaper and honest: "decoded on each read, charged to MaxWork per byte parsed; a caller who reads it more than once decodes it with Unmarshal first."

4. **"Cancellation is checked between operations."** Implemented literally as `ctx.Err()` per node, this takes `cancelCtx.mu` per node: a mutex in the inner loop. Cheapest keeping: capture `ctx.Done()` once, poll a non-blocking select every N work units (tie N to the MaxWork counter crossing a stride of 1024). Wording: "checked at intervals bounded by MaxWork units, and within operations whose cost scales with input size."

5. **"Eval fixes the timestamp at its start" and "Env.Now is called once per Eval."** A vDSO call per Eval for expressions that cannot observe the clock, and a `time.Time` (24 bytes with a `*Location` the GC must scan) in every frame. The compiler knows whether `$now`, `$millis`, or `$eval` appears. Wording: "fixed before the first observation and at most once per Eval or Prepare; Env.Now is not called when the expression cannot observe the clock."

6. **"An Evaluation is safe for concurrent use: a Select of a field another goroutine is computing waits for it," with "one budget across all selections."** The memo can be cheap: a slot per top-level field with an atomic state word; the uncontended done path is one atomic load; a waiter lazily allocates a channel only on actual contention, and it must be a channel (not `sync.Once`) so the waiter can also select on its own ctx, which the doc requires. The expensive part is the shared budget: three counters decremented on every node become atomic adds, and contended atomics across cores are the slowest thing in this design. Cheapest keeping: per-Select reservation batches (reserve 4096 work units with one atomic, refund on return). That changes when a budget failure fires under concurrency (an in-flight reservation can starve a sibling that would have passed sequentially), which the doc must then admit. Or drop the promise (see Changes).

7. **"Charges each call as work and the bytes it returns to MaxBytes" (Resolver).** The bytes of a returned view are unmeasurable without walking it, and walking it defeats the view. Wording: "charges each call as work, and the bytes of strings and []byte the engine subsequently observes through it to MaxBytes."

8. **"Raised before the allocation that would exceed it."** Free where the size is known before allocating (`&`, `$string` of a scalar, `$join`, `$substring`). For incremental builders (`$replace` with a regex, `$string` of a large object) the honest granularity is buffer growth: charge capacity before each doubling. Keep the wording; note that "before the allocation" is per growth step.

9. **"Materialize: a value containing no foreign value is returned by identity."** Requires a full walk of the result to prove the absence, and `EvalJSON` then walks it again in Marshal. Since carriage does not classify, the engine cannot cheaply know whether a result contains a foreign value. Cheapest keeping: let Marshal resolve through `env.Resolver` when one is present, so EvalJSON is one walk.

10. **"The engine calls Resolve once per read of a foreign value."** Forbids a per-evaluation memo of resolved views, so `{ "a": $.user.a, "b": $.user.b }` resolves `user` twice. The cheap fix costs nothing in the API: a Resolver may memoize by pointer identity itself. Say so in the Resolver doc rather than promising exactly-once.

11. **"Prepare ... plans selective evaluation."** Planning is a static property of the expression plus the root's kind. Do it at Compile; Prepare only checks whether the root is an array. Wording: "The plan is fixed at Compile except ReasonArrayInput, which Prepare determines from the root."

12. **"A map's keys in sorted byte order" for every order-observing operation.** `$keys`, `$each`, `$spread`, `$merge`, `$sift`, `$string` over a map each collect and sort: O(n log n) and one allocation per map per observation. A port with its own ordered tree pays nothing here. This is the price of determinism over Go's randomized iteration and it does not touch the rename path; accept it, but state that `*Object` input avoids it so a caller with a hot `$merge` knows the fix.

13. **"Get is O(1)" on Object.** Forces an index map, which for a two-member constructed object is two more allocations (hmap plus bucket) and dominates the result cost. A linear scan over n ≤ 8 entries is bounded constant time, which satisfies O(1) literally; build the index lazily above 8. If the reviewer of the doc balks, "Get does not scan for large objects" says the same thing.

14. **"Close: a call in flight on another goroutine completes."** If Close must wait, that is a WaitGroup-style atomic inc/dec per Select. Cheaper: Close marks closed (one atomic store) and the borrow ends when the in-flight call returns; reword "the borrow of input and Env ends when Close returns and any call then in flight completes."

15. **"Unmarshal ... duplicate member name is an error."** Free when the index map exists (insert reports it); for the lazy small-object case it is an O(n²) scan with n ≤ 8, at most 28 comparisons. Fine.

## The internal representation

**Values.** `any`, with no wrapper, because carriage by identity and the `any` boundary make any internal tagged union cost a box and unbox at every edge. Classification is a function `kindOf(any) kind`: a type switch ordered by frequency (`*Object`, `map[string]any`, `string`, `[]any`, `int64`, `float64`, `bool`, `nil`, then the rest of the numeric tower, `[]byte`, `json.Number`, `*big.Int`, views), then a `reflect.TypeOf(v).Kind()` fallback for `[]T`, `map[string]T`, and named types. Numbers observed by an operation are normalized to a stack struct `num{kind uint8; i int64; f float64; b *big.Int}`; exact int64-versus-float64 comparison is done by splitting the float with `math.Modf` and integer compares, no allocation. Sequences (multi-element path results) are a private `*sequence{items []any; keepSingleton bool}` that never escapes the evaluator and is unwrapped to a singleton or a `[]any` at the boundary; singletons are never wrapped. Object results are `*Object` with the trailing-buffer trick for small capacities and a lazy index above 8.

**Compiled expression.** Nodes resolved at Compile: variable references to frame slot indices; built-in calls to direct function pointers, except names the expression references that Env.Bindings could shadow, which get a slot filled once at Eval start from the map (one lookup per referenced name, not per call); constructor keys interned as substrings of the retained source; literals pre-boxed once; regexes compiled. Dispatch is a switch on a kind byte in one recursive `eval` so the frame stays on the stack. The Expression also holds the static plan (field ordinals, purity, prelude boundary), the `Reads` result (computed at Compile, copied fresh on call), and the Limits by value.

**Frame and budgets.** `frame{work, bytes, nodes int; env *Env; root any; done <-chan struct{}; slots []any; now time.Time; inUser bool}`. Charging is `fr.work--; if fr.work < 0 { return budget }`. Slots are the one allocation I cannot remove when the expression binds variables; for the rename path there are none. `inUser` is toggled around Resolver and BytesEncoding calls so the single `recover` can re-panic user panics and convert engine panics.

**Evaluation memo.** `Evaluation{ frame; expr *Expression; slots []fieldSlot; preludeOnce state }` with `fieldSlot{ state atomic.Uint32; val any; present bool; err error; wait chan struct{} }`. Select: field name to ordinal via a small map on the Expression (built at Compile, no alloc to query), one atomic load, evaluate on miss. The variadic `path ...string` is stack-allocated at the call site as long as Select never retains it (do not put it in an Error).

**Where the API fights this.** The shared budget under concurrent Select (atomics or reservations in the inner loop). The clock in every frame. Materialize as a separate walk. "Once per read" forbidding a resolver memo. `json.RawMessage` needing a side table. "Prepare plans" implying per-call work. And `Unmarshal(data []byte) (any, error)` returning fully decoded values: the fast decoder converts `data` to one string and substrings keys and unescaped values from it (one copy for the whole text instead of forty small ones; a 20-member string-valued body is about 25 allocations instead of encoding/json's 60-plus), which retains the whole text while any substring lives. That is a memory-retention trade the doc should state or forbid; I would state it.

## What I would change

1. **Drop concurrent safety on Evaluation.** Wording: "An Evaluation is not safe for concurrent use. A caller that selects from more than one goroutine serializes the calls." Why: the only promise in the API that puts atomics into the per-node loop or forces reservation-window budget semantics, and the stated workload (one consumer selecting fields of one transform) never needs it. If it must stay, add: "Under concurrent selections, budget exhaustion may occur at a different point than under sequential selection."

2. **Let the expression drive decoding.** Signature: `func (e *Expression) UnmarshalInput(data []byte) (any, error)`, documented as "Unmarshal, except that when Reads is known the members outside the reported paths are skipped rather than decoded; the result is indistinguishable to this expression, and Marshal of a carried root is then an error (CodeUnsupportedValue) unless the root path is among the reads." Why: for a 20-member body and a two-field transform this drops decoding from ~25 allocations to ~5, and skip-value scanning in JSON is byte-speed. It is the single largest lever the API already half-exposes through `Reads` and does not consume.

3. **Fold Materialize into Marshal.** Wording on Marshal: "A foreign value in v is resolved through env's Resolver when one is present and is an error (CodeUnsupportedValue) otherwise." EvalJSON becomes "Eval, then Marshal through env." Why: one walk of the result instead of two, and the walk is already the Marshal walk. Materialize stays for callers handing results to typed code.

4. **Three rewordings that together remove a fixed ~50 ns and an O(bytes) risk from every Eval.** Carriage: "Carriage neither classifies nor validates" (as in Promises 1). Clock: "fixed before the first observation and at most once; Env.Now is not called when the expression cannot observe the clock." Cancellation: "checked at intervals bounded by MaxWork units." Why: each is unobservable to a caller and each is the difference between a promise I can keep for free and one I keep with a syscall, a mutex, or a validation pass.

5. **Accounting wording that is implementable as written.** Resolver: "charges each call as work, and the bytes of strings and []byte the engine observes through the view to MaxBytes." RawMessage: "decoded on each read, charged per byte to MaxWork." Prepare: "The plan is fixed at Compile except ReasonArrayInput." Object: keep "Get is O(1)" and implement the lazy index, or say "Get does not scan for large objects." Why: each current sentence either cannot be implemented (view bytes), implies a cache with no home (RawMessage), or implies per-call work that should be per-Compile.

## What I would keep

- **Limits baked into the compiled Expression, immutable, concurrent.** This is what lets Compile do all the resolution: slot indices, direct built-in pointers, interned keys, pre-boxed literals, compiled regexes, the static plan, and `Reads`. Every one of those is a per-call cost moved to per-compile.
- **Carriage by identity with "one node, bytes not charged."** The reason the rename path is O(fields) and not O(input). Defend this against anyone who wants defensive copies of results.
- **Admission per value on read, root only at Prepare.** No walk on entry; a megabyte body and a two-field transform cost the same as a two-field body.
- **Unmarshal to int64, `*big.Int`, float64, and `*Object`.** Classification paid once at decode instead of on every observation; `*Object` gives document order without a sort.
- **The zero-value `Object`, `NewObject(capacity)`, and no concurrency safety on Object.** Exactly the shape the constructor needs for a single allocation.
- **Regex at Compile.** Obvious and right.
- **No goroutines, no process state, no function registration, values-only bindings.** Built-ins can be direct function pointers, the environment closure is a compile-time fact, and there is nothing to lock.
- **Budgets by work and bytes, not time.** Counters, not timers; no goroutine per evaluation.
- **`ObjectView.Get` reporting absence by `ok`, and `[]byte` classified before `[]uint8`.** No sentinel values, and a deterministic first-arm type switch.
- **Encoder over `io.Writer`, Marshal into one growable buffer, `Error()` rendered lazily with no input content in Message.** No fmt on the hot path and no partial-copy of results for output.
- **Bytes encoded only when observed.** Strictly cheaper than any boundary that base64-encodes eagerly.

## On the concept

Host-native evaluation by reference does buy the speed the README implies, but for a specific reason the README does not name: the win is the absence of a second tree. Every existing port decodes into its own node type, so a transform over a decoded payload pays decode, convert, evaluate, convert back. This design pays decode and evaluate, and a carried value is the same pointer in and out. On the rename workload that is the whole ballgame; the evaluator's own cost is a handful of map lookups and one allocation, and the payload's size stops mattering. Where the by-reference model does not buy anything is scalar boxing: `any` holding an int64 or a string allocates in Go, and every value a Resolver returns pays that. A port would pay it too, so it is not a cost of the concept, but it caps how fast the protobuf lane can ever be without a typed accessor API, which this package rightly does not offer.

The value model costs more than a port in three places, none on the stated hot path. The admitted set is wide (two dozen types plus a reflect fallback), so `kindOf` is a bigger switch than a six-kind tree's; the numeric tower's exactness rules mean comparison is branchier than a float64 compare; and two object types plus views mean every object operation has three arms, which is code size, branch-predictor pressure, and a standing risk that one arm quietly gets the slow implementation. The sorted-map rule is the one place the model is genuinely slower than a port with its own ordered container, and it only fires on order-observing functions over Go maps, which `*Object` input avoids. Encoding bytes on demand and exact integers cost nothing until observed, and refusing rather than approximating is cheaper than approximating (an overflow check is one instruction; a rounding path is a conversion).

The regex analogy is apt for the shape and wrong for the source of speed. Regex engines are fast because they compile to a program and run it once per input with no allocation per match, and this API honors the shape exactly: Compile once, immutable, bounded, run many. But a regex runs over a flat byte array, and there is no flat array here; the input is a heap graph of interfaces, and the wins come from not copying it, not boxing it twice, and not walking parts the expression never reads. The closest thing to a DFA in this design is `Reads`: a static description of which parts of the input matter, which is why my second change is to let it drive the decoder. The other true regex lesson the API already absorbed is RE2's, not PCRE's: bounded work as a contract, not a hope. Limits are that contract, and they are cheap to enforce precisely because they count work rather than time.
