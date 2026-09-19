# Iteration-4 panel: application integrator

Cold read of `e57816b`. Lens: the developer integrating this into a production Go service (HTTP and gRPC handlers, untrusted configuration expressions).

## Grades

| Criterion | Grade | One-line justification |
|---|---|---|
| Idiomatic fit for Go | A- | Compile/MustCompile, ctx-first, errors.Is/As with sentinels, iter.Seq, Close, borrowed-not-copied: it reads like a stdlib package; the `(value, present, err)` triple and the `any` result are the price of the language, not a design smell. |
| Ergonomics of the common path | B | JSON-in JSON-out via `EvalJSON` is a one-liner; anything involving my own structs, protobuf, or typed output is 150 lines of view boilerplate the package offers no help with. |
| Correctness and footgun risk | B- | Almost every trap is documented, which is more than most libraries do, but there are a lot of them: carried foreign values in results, `[]*pb.T` being foreign as a whole, the nil-slice/nil-map/nil-pointer asymmetry, aliasing of shared bindings, `$eval` on by default, `Unmarshal` with no depth bound. |
| Performance headroom the API permits | A- | Views by reference, no copy on admission, per-expression budgets that are cache-key friendly, `Reads()` for fetch planning, `Prepare/Select` sharing work, no goroutines, no global state; missing only a process-wide budget and any memoization guarantee for `Resolve`. |
| Concept soundness | B+ | The regex framing is right and the exactness promises are exactly what I need at the boundary; the weak spot is that the class explicitly permits members to disagree on big integers, so an OBI transform is only portable inside the "portable core" subset. |
| Overall | B+ | I would ship it behind a handler tomorrow for JSON bodies; for protobuf and typed consumers I would first write the adapter package this repo should have written. |

## The integration I would write

### (a) HTTP handler over a decoded JSON body

```go
type transformHandler struct {
	expr *jsonata.Expression
	env  *jsonata.Env // shared, read-only: Resolver + BytesEncoding set at startup
}

func (h *transformHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return
	}
	out, present, err := h.expr.EvalJSON(r.Context(), body, h.env)
	if err != nil {
		status, msg := toHTTP(err) // see (d)
		http.Error(w, msg, status)
		return
	}
	if !present {
		w.WriteHeader(http.StatusNoContent) // GUESS: absent has no JSON spelling; I picked 204. Nothing in the docs suggests a convention.
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(out) // GUESS: Marshal output has no trailing newline (Encoder says it appends one; Marshal says nothing).
}
```

That is the happy case and it is genuinely good. The realistic case is worse: my service already decodes the body with `encoding/json` into a request struct for validation. Now I have three options and none is free: decode twice (`json.Unmarshal` into the struct, `jsonata.Unmarshal` for the evaluator), decode once with `jsonata.Unmarshal` and validate through `Member` on an `*Object`, or pass the struct and write a Resolver for it. I guessed that decode-twice is what most people will do, which doubles parse cost and silently lets the two decoders disagree (`Unmarshal` refuses duplicate keys; `encoding/json` keeps the last).

### (b) gRPC handler over a protobuf message, with the Resolver

```go
type protoResolver struct{}

func (protoResolver) Resolve(ctx context.Context, v any) (any, error) {
	switch m := v.(type) {
	case proto.Message:
		return messageView{m.ProtoReflect()}, nil
	case protoreflect.Message:
		return messageView{m}, nil
	}
	// GUESS: for "not on my allowlist" I want CodeUnsupportedValue, not CodeResolver.
	// Returning an error gives E1007 with my cause wrapped; returning v unchanged
	// presumably gives E1001. The docs do not say which the package prefers.
	return nil, fmt.Errorf("transform: %T is not readable", v)
}

type messageView struct{ m protoreflect.Message }

func (v messageView) Get(ctx context.Context, key string) (any, bool, error) {
	fd := v.m.Descriptor().Fields().ByJSONName(key) // GUESS: keys are mine to choose; I picked protojson names so transforms match the REST binding.
	if fd == nil {
		return nil, false, nil
	}
	if fd.HasPresence() && !v.m.Has(fd) {
		return nil, false, nil // GUESS: unset message/optional field is absent, not null. Get and Range "must agree", so Range below skips the same fields.
	}
	return convert(fd, v.m.Get(fd)), true, nil
}

func (v messageView) Range(ctx context.Context, fn func(string, any) bool) error {
	fields := v.m.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if fd.HasPresence() && !v.m.Has(fd) {
			continue
		}
		if !fn(fd.JSONName(), convert(fd, v.m.Get(fd))) {
			return nil
		}
	}
	return nil
}

func convert(fd protoreflect.FieldDescriptor, val protoreflect.Value) any {
	switch {
	case fd.IsList():
		return listView{fd, val.List()}
	case fd.IsMap():
		return mapView{fd, val.Map()} // GUESS: map<int64,T> keys must become strings; I stringify them. The package admits only map[string]T.
	}
	switch fd.Kind() {
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return val.Int() // exact int64; this is the whole reason I am using this package
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return val.Uint() // package widens to int64 or *big.Int; fine
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return int32(val.Int())
	case protoreflect.FloatKind:
		return float32(val.Float()) // GUESS: float32 "is float64" on admission; a wire 0.1f becomes 0.10000000149011612 and $string will print all of it
	case protoreflect.DoubleKind:
		return val.Float()
	case protoreflect.BoolKind:
		return val.Bool()
	case protoreflect.StringKind:
		return val.String()
	case protoreflect.BytesKind:
		return val.Bytes() // needs Env.BytesEncoding or any string use is E1001
	case protoreflect.EnumKind:
		return string(fd.Enum().Values().ByNumber(val.Enum()).Name()) // my binding choice: names, as protojson
	case protoreflect.MessageKind:
		msg := val.Message()
		if ts, ok := msg.Interface().(*timestamppb.Timestamp); ok {
			return ts.AsTime().UTC().Format(time.RFC3339Nano) // my binding choice
		}
		return msg.Interface() // foreign; resolved on read
	}
	return nil
}

type listView struct {
	fd protoreflect.FieldDescriptor
	l  protoreflect.List
}

func (v listView) Len(ctx context.Context) (int, error)      { return v.l.Len(), nil }
func (v listView) At(ctx context.Context, i int) (any, error) { return convert(v.fd, v.l.Get(i)), nil }
```

Note the thing I nearly got wrong: returning `[]*pb.Item` from `convert` would not be "an array with foreign elements", it would be a foreign value as a whole, because `[]T` is admitted only when `T` is admitted. So every repeated message field has to go through `listView` or be copied into `[]any`. That is correct per the docs but I only caught it on the second read.

The gRPC handler:

```go
func (s *server) Summarize(ctx context.Context, req *pb.SummarizeRequest) (*pb.SummarizeResponse, error) {
	u, err := s.users.Get(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	out, present, err := s.expr.Eval(ctx, u, s.env) // u is *pb.User: foreign root, Resolver is consulted
	if err != nil {
		return nil, toStatus(err)
	}
	if !present {
		return &pb.SummarizeResponse{}, nil
	}
	// GUESS: if the transform is `$` or `$.profile`, out is the original *pb.User or *pb.Profile,
	// not a view, and Marshal would fail with E1001. So I Materialize unconditionally.
	out, err = jsonata.Materialize(ctx, out, s.env, nil)
	if err != nil {
		return nil, toStatus(err)
	}
	b, err := jsonata.Marshal(out, s.env)
	if err != nil {
		return nil, status.Error(codes.Internal, "encode")
	}
	var st structpb.Struct
	if err := protojson.Unmarshal(b, &st); err != nil { // GUESS: no direct result -> structpb path; I go through text, and structpb is float64, so the exactness I paid for is lost here anyway
		return nil, status.Error(codes.Internal, "encode")
	}
	return &pb.SummarizeResponse{Result: &st}, nil
}
```

### (c) Selecting two fields of a large result

```go
ev, err := expr.Prepare(ctx, in, env)
if err != nil {
	return err
}
defer ev.Close()

if !ev.Plan().Selective() {
	metrics.WholeEval.WithLabelValues(ev.Plan().Reason.String()).Inc() // worth knowing in prod
}
id, idOK, err := ev.Select(ctx, "id")
if err != nil {
	return err
}
name, nameOK, err := ev.Select(ctx, "display_name")
if err != nil {
	return err
}
// GUESS: a typo in "display_name" and a field whose subexpression legitimately yields
// undefined both come back as (nil, false, nil). I cannot tell them apart, and I cannot
// check the key list at startup because Plan lives on Evaluation, not Expression.

resp := &pb.Summary{}
if idOK {
	resp.Id, err = asInt64(id) // GUESS: id may be int64, int, int32, *big.Int, float64 (if the transform divided), or json.Number (if the input was encoding/json+UseNumber). I wrote my own coercion.
}
if nameOK {
	s, ok := name.(string) // GUESS: could also be []byte if the field was proto bytes and the transform just carried it
	...
}
```

The `asInt64` I had to write is ten cases long and is exactly the kind of thing the package knows how to do already (it does this on every comparison). It should export it.

### (d) Errors and absence

```go
func toHTTP(err error) (int, string) {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout, "transform timed out"
	case errors.Is(err, context.Canceled):
		return 499, ""
	case errors.Is(err, jsonata.ErrMalformedInput):
		return http.StatusBadRequest, err.Error() // Offset is into the body; safe and useful
	case errors.Is(err, jsonata.ErrResolver):
		if errors.Is(err, store.ErrNotFound) { // Unwrap returns my cause: nice
			return http.StatusNotFound, "not found"
		}
		return http.StatusBadGateway, "upstream"
	case errors.Is(err, jsonata.ErrBudget):
		return http.StatusUnprocessableEntity, "transform exceeded its budget" // GUESS: 422 if the tenant owns the transform, 500 if I do; the class does not tell me who to blame, and it cannot
	}
	var e *jsonata.Error
	if !errors.As(err, &e) {
		return http.StatusInternalServerError, ""
	}
	switch e.Code.Class() {
	case jsonata.ClassRaised:
		// $error("...") or a failed $assert: the transform author's own refusal.
		return http.StatusUnprocessableEntity, fmt.Sprint(e.Value) // GUESS: Value is "the offending value"; for $error I assume it is the message string. It is NOT scrubbed of input content the way Message is, so returning it to a client is a decision, not a default.
	case jsonata.ClassType, jsonata.ClassEvaluation:
		return http.StatusUnprocessableEntity, e.Error() // Error() is Code+Message+Offset; documented as free of input content, so I can return it
	case jsonata.ClassSyntax:
		return http.StatusInternalServerError, "" // only reachable via $eval after a successful Compile
	default: // ClassEngine: E1001/E1005/E1008 are mine to fix, log with e.Code
		return http.StatusInternalServerError, ""
	}
}
```

Absence is handled in (a): the three-way switch is fine, but note that `EvalJSON` returning `(nil, false, nil)` means my handler has a branch that `encoding/json`-shaped code never had.

### (e) Startup, caching, lifecycle

```go
// Per-evaluation defaults are 64 MiB and 8M work units. At 500 in-flight requests that is a
// 32 GB ceiling, so I shrink them. GUESS: nothing shares a budget across evaluations.
var limits = jsonata.Limits{
	MaxWork:        1 << 20,
	MaxBytes:       8 << 20,
	MaxOutputNodes: 1 << 16,
	MaxRecursion:   256,
}

var env = &jsonata.Env{
	Resolver:      protoResolver{},
	BytesEncoding: base64.StdEncoding, // matches protojson
}

type exprKey struct {
	src string
	lim jsonata.Limits // comparable, documented to stay so: good
}

type registry struct {
	sem  chan struct{} // bound concurrency myself; the engine will not
	exprs sync.Map     // exprKey -> *jsonata.Expression
}

func (r *registry) load(cfg Config) error {
	for _, t := range cfg.Transforms {
		expr, err := jsonata.Compile(t.Source, &limits)
		if err != nil {
			var e *jsonata.Error
			errors.As(err, &e)
			return fmt.Errorf("transform %q: %s at %d:%d near %q", t.Name, e.Message, e.Line, e.Column, e.Token)
		}
		if paths, known := expr.Reads(); known {
			r.fieldMasks[t.Name] = toFieldMask(paths) // genuinely useful for gRPC read requests
		}
		// GUESS: I cannot verify here that t.SelectFields are constructor keys, or that the
		// expression is selectable at all. Plan needs a Prepare, which needs an input.
		r.exprs.Store(exprKey{t.Source, limits}, expr)
	}
	return nil
}
```

Per-request bindings (tenant id, request headers) mean a fresh `Env` per request: I copy the shared one by value and set `Bindings`. That works because `Env` is a plain struct, and the docs say Env is read-only during evaluation, so copying is safe. Nothing needs closing except `Evaluation`, and even that is GC-safe if forgotten. Lifecycle is the best part of this API.

## What I would change

1. **Export the coercions the engine already has, and a typed decode.** The stated requirement is handing results to typed Go code, and the package offers `Member` and nothing else. Every integrator will write the `asInt64` switch above, and most will get `*big.Int` or `json.Number` wrong. Add:
   ```go
   func Int64(v any) (int64, bool)     // exact only: int kinds, *big.Int that fits, integral float64 <= 2^53, json.Number
   func Float64(v any) (float64, bool) // exact only; false on CodeInexact grounds
   func String(v any, env *Env) (string, bool) // string, or []byte through env
   func Index(v any, i int) (any, bool)  // []any, []T, ArrayView, carried foreign via env? see below
   func Decode(ctx context.Context, v any, env *Env, target any) error // json-tag semantics into a struct; int64 fields exact
   ```
   `Decode` is the mirror of `Resolver`: the package defines the number model, so it, not `encoding/json` via a round-trip through text, should be the thing that lands values in `int64` and `*big.Int` fields.

2. **Ship the two Resolvers everyone needs, as subpackages, and pin the memoization contract.** `jsonata/protoview` (a `Resolver` over `proto.Message` with explicit options: `Int64` as exact int64, enums as names or numbers, `Timestamp`/`Duration` rendering, presence rule) and a `jsonata/structview.Resolver(types ...any)` that is an allowlist by concrete type using `json` tags. "Never reflection over arbitrary structs" is a fine principle and an allowlist of named types satisfies it. The Get/Range agreement rule on presence semantics is a trap best solved once, not per team. And the doc should say whether `Resolve` results are memoized per evaluation by identity; "once per read" as written means I build a `sync.Map` cache in my Resolver or eat repeated `ProtoReflect()` work on every path step.

3. **Give `Expression` a static view of selectability.** `func (e *Expression) Fields() (keys []string, ok bool)` returning the top-level constructor keys when the result shape is a literal object constructor (ignoring the input-dependent `ReasonArrayInput`). Then `Select("dispaly_name")` can be rejected at config-load time instead of coming back absent forever in production. Optionally make `Select` of a key that is statically not among the constructor's keys an `*Error` (a new engine code) rather than absent; absent should mean the field's subexpression produced undefined, not that I misspelled it.

4. **A knob to close `$eval`.** With document-supplied expressions over untrusted input, `$eval` makes input into code. It is inside the budget, so it is not a DoS hole, but it defeats `Reads()`, defeats selective evaluation (`ReasonOpaque`), and makes the expression's behavior unreviewable from its source. I want `Env.DisallowEval bool` (evaluating `$eval` then raises an engine code) or, if the binding-shadowing trick (`Bindings["eval"] = nil`) is the intended mechanism, say so in the doc and make it produce a clear error rather than a T1006 about a null.

5. **Bound `Unmarshal`.** `func Unmarshal(data []byte, limits *Limits) (any, error)` honoring `MaxDepth` and `MaxBytes`. Today a body of a million `[` characters is bounded only by my `MaxBytesReader` and Go's recursive-descent stack; the evaluation is carefully budgeted but the decoder in front of it is not. `Materialize` already takes `limits`, so the pattern exists.

## Footguns

- **Carried foreign values come back as the original.** `$` or `$.profile` over a proto root returns `*pb.User` / `*pb.Profile`, and `Marshal` then fails with E1001. Every test with JSON input passes; the proto path fails on the first identity-ish transform. `EvalJSON` hides this only for JSON input. Always `Materialize` before `Marshal` when a Resolver is in play, and the docs should say that in the `Eval` comment, not only in the package overview.
- **`[]*pb.Item` is foreign as a whole.** `[]T` is admitted only for admitted `T`. A Resolver that returns a Go slice of messages, which is the natural thing to write, gets resolved again as a foreign value, and if the Resolver's switch doesn't handle slices, E1001.
- **Struct input with no Resolver is E1001, not a helpful message.** Week-one error for anyone who decodes bodies into request structs. The fix (double decode, or a hand-written view) is more code than the integration itself.
- **`v, _, err :=`.** Documented, and still the number one bug I expect to see in code review, because every other Go decoder returns two values.
- **nil handling is three different things.** nil slice is `[]`, nil map is `{}`, nil pointer and nil interface are `null`, nil `[]byte` is empty bytes. Sensible individually; as a result surface it means `$exists(items)` is true for a Go struct's unset slice field and false for its unset pointer field, which will surprise transform authors who only see JSON.
- **Aliasing of shared bindings.** If I bind a config object as `$config` in a long-lived `Env`, any result can contain it by identity, and a downstream `append` or map write corrupts it for every future request, or panics with a concurrent map write. There is no `Clone`, and `Materialize` returns identity when nothing is foreign, so there is no package-provided way to detach a result.
- **`$eval` is on.** Covered above. Also `$random` on `math/rand/v2` is fine but the docs should say "not for tokens" more loudly than "not cryptographic".
- **Per-evaluation budgets only.** 64 MiB default `MaxBytes` times in-flight requests. Nothing in the package bounds aggregate cost; I must set smaller limits and a semaphore.
- **Panics in a Resolver propagate.** `net/http` recovers per request; a plain gRPC server does not. Add a recovery interceptor or `recover()` in the Resolver.
- **`Encoder.Encode` can write a partial value.** Fine for a log stream, wrong for an HTTP response, and the doc says so, but it will still be used in handlers because that is how `json.NewEncoder(w).Encode` is used.
- **Stricter decoder than the one my clients are used to.** Duplicate keys and unpaired surrogates are 400s now. Existing clients sending sloppy JSON accepted by `encoding/json` will break at the transform layer, with an error whose Offset is into a body they never see.
- **Regex dialect is "pending ruling."** If the ruling is JavaScript semantics (backreferences, lookaround) that is not Go's `regexp`, and the linear-time guarantee that makes untrusted regex literals safe goes away. From the integrator's seat this ruling matters more than most of the numeric ones.

## Things the doc comments leave unclear

- Does `Marshal` emit a trailing newline? `Encoder.Encode` says it appends one; `Marshal` is silent.
- Is `Resolve` memoized per evaluation by identity, or truly called on every read of the same foreign value? "Once per read" suggests the latter; `Prepare` "classifies the root" suggests at least the root is done once.
- When my Resolver does not recognize a type, should it return an error (E1007, cause wrapped) or return `v` unchanged (E1001)? Which does the package consider "not on the allowlist"?
- Is a `Resolver` consulted for a value that already implements `ObjectView` or `ArrayView`? The overview says a view "is read through its methods wherever it appears," so presumably not, but `Resolve` is described as being for values "outside the admitted set" and views are listed as admitted; the two sentences should be joined.
- What does `Get` returning `(nil, true, nil)` mean from a view: null. And `(nil, false, err)`: does the engine ignore `ok` when `err` is set? Presumably.
- `json.RawMessage` "is decoded as JSON on first read": cached where? Values are never copied on admission and the package holds no state, so is a RawMessage inside the input re-parsed every read, or parsed once per evaluation? A nil or empty `RawMessage`: E1006, or the empty bytes because it is a named `[]byte`? "Classification is by exact type first" implies E1006, but say it.
- `Error.Value` for `ClassRaised`: is it the `$error` message string, the `$assert` message, or something else? Is it safe to show to a client? `Message` is explicitly scrubbed; `Value` is explicitly not.
- `Select` waiting on another goroutine's computation of the same field: what happens when the waiter's ctx expires but the worker's has not? Does the waiter return `ctx.Err()` and the field still land for the worker?
- Does `Prepare` charge the budget for the prelude at `Prepare` time or on first `Select`? "Without evaluating anything else" says the latter; then the first `Select` can fail with a prelude error the second `Select` will repeat, presumably identically.
- What is the precise output of `Reads()` for a path under a predicate, e.g. `items[price > 10].name`: is `price` in the list? "a key applies to every element when the value at that point is an array" covers `items.name`, not the predicate.
- Does `Reads()` for an expression that reads nothing return `(nil, true)` or `([][]string{}, true)`? Both are "sound"; only one is nice to test against.
- `Limits` is "part of the compiled expression so they apply wherever it is evaluated," but `Materialize` takes its own `limits` and `Unmarshal` takes none. Which bounds does `EvalJSON`'s inner `Unmarshal` and `Materialize` run under: the expression's?
- `Env.Bindings` values may be foreign and are resolved on read; may a binding be a `NoInput`? The doc says nested `NoInput` is foreign, so presumably a Resolver sees it, which would be a strange thing to be handed.
- `Object.Set` on a key that is not valid UTF-8 is stored and refused on read. Does `Unmarshal` ever produce such an object? It refuses unpaired surrogates, so presumably not, but a ` ` key is valid UTF-8 and I would like to know it round-trips through `Marshal`.
- `Marshal` renders `float32` "as its float64 widening": so a carried proto float `0.1` prints as `0.10000000149011612`. True to the model and surprising to anyone comparing against protojson, which prints `0.1`. Worth a sentence.
- `MustCompile` "for an expression fixed at build time" takes `limits`: fine, but say whether `nil` is allowed (it must be) so I can write `MustCompile(src, nil)`.
- The `ExampleError` example evaluates `$sum([1..100000000])` and switches on `ErrBudget` first, then `ClassRaised`; it never shows the `ClassType`/`ClassEvaluation` path, which is the one a handler hits most. One more example, `ExampleExpression_Eval_error` with a `T2001`, would carry its weight.

## On the concept

The regex framing holds up from where I sit, and the place it helps most is the one the README does not emphasize: lifecycle. Because the package behaves like `regexp` (compile once, immutable, safe to share, no goroutines, no global state, budgets attached to the compiled object), I know how to cache it, how to key the cache, and what is safe across goroutines without reading the implementation. That is worth more to a service owner than any single language feature. The "documentation as authority" half of the thesis is invisible to me at integration time, and that is the correct outcome: I need the semantics to be stable and declared, and the divergence ledger pinned to fixtures is exactly the artifact I would cite in an incident postmortem.

"The host defines what a number is" is the right call, with one caveat. It is right because the alternative, porting JavaScript's number model, would make the library useless for the stated job: I am using it precisely because `encoding/json` into `any` already destroyed my IDs and I need something that does not. The promises this package makes are the ones I want at the boundary: an `int64` that came in from a JSON body via `Unmarshal` and an `int64` that came in from a protobuf field via my Resolver are the same value, compare equal exactly, survive `$.id`, `$string`, `$lookup`, object construction, and `$merge` unchanged, and come out as `int64` (or as exact digits from `Marshal`). Arithmetic that would lose precision refuses instead of rounding. That is the contract I would write on the whiteboard, and the doc comment states it. What I would add is a promise about the exit: the package should own the coercion into typed Go, so that "an int64 field in my struct gets the exact value or an error" is a library guarantee rather than a property of my hand-written switch or of a round-trip through JSON text.

The caveat is portability, and it is the class's own admission: "no parity between members." The same OBI transform evaluated by a TypeScript member over `number` will disagree with this Go member on `9007199254740993`, and the README's answer is the "portable core" subset. From the integrator's seat that is honest but uncomfortable: a transform that is correct in the Go binding and silently wrong in the TS binding is the kind of bug that surfaces as a mismatched ID six months later. If the project's own members are going to sit behind the same `TransformEvaluator` seam, I want the TS member to be held to the same exactness commitment (BigInt where the token exceeds 2^53), or I want the class to say, in Core-facing terms, that big-integer carriage is a member property a binding must declare. The value model belonging to the host is the right rule for arithmetic; for carriage of identifiers it should be a class-wide floor, because the rest of the OpenBindings story depends on values crossing systems intact.
