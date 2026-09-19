# Iteration-5 panel: application integrator

Cold read of `7f8b4fe`. Lens: the developer integrating this into a production Go service (HTTP and gRPC handlers, untrusted configuration expressions).

## Grades

| Criterion | Grade | One-line justification |
|---|---|---|
| Idiomatic fit for Go | B+ | `regexp`-shaped lifecycle, `iter.Seq`, ctx-aware Resolver, `errors.Is`/`As` done right; docked for `Marshal(v, env)` accepting an Env whose Resolver it refuses to use, pointer-for-optional `*Limits` on `MustCompile`, and a result surface of roughly fifteen concrete Go types. |
| Ergonomics of the common path | B- | JSON bytes in, JSON bytes out is one call (`EvalJSON`). Anything else (protobuf in, typed struct out, selected fields out) is three or four calls, a hand-written Resolver, and a second budget I have to remember to pass. |
| Correctness and footgun risk | B- | The numeric model is the most careful I have seen in a Go transform library, but "Select is not a guard" becomes plan-dependent, results alias shared bindings, `CodeInexact` is data-dependent, and duplicate-key refusal will reject bodies my `encoding/json` handlers accept today. |
| Performance headroom the API permits | A- | Carriage by reference, selective evaluation, `Reads()` for upstream projection, no goroutines, no global state. Lost points: no engine-side memoization of Resolver results, a key sort per order-observing operation on maps, `json.Number` re-parsed per read, and limits baked into the Expression rather than the call. |
| Concept soundness | B+ | JSONata-as-regex is the right lifecycle model and the right authority model; it breaks at egress, where a regex returns one type and this returns whatever shape the input happened to have. |
| Overall | B | I could ship this behind an HTTP handler tomorrow. I could not ship it behind a gRPC handler tomorrow without a day of Resolver work and a set of representation decisions the library correctly refuses to make but gives me no help making. |

## The integration I would write

### (a) HTTP handler, JSON body in, JSON out

```go
type Transform struct {
	expr *jsonata.Expression // compiled at startup
	env  *jsonata.Env        // shared; BytesEncoding set, Bindings never mutated
}

func (t *Transform) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
	if err != nil {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return
	}
	out, present, err := t.expr.EvalJSON(r.Context(), body, t.env)
	if err != nil {
		writeTransformError(w, err)
		return
	}
	if !present {
		w.WriteHeader(http.StatusNoContent) // GUESS: nothing says what absent means at a wire boundary; I picked 204 over writing "null"
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
}
```

This path is fine. `EvalJSON` is the one thing in the package that reads like it was designed by someone who has written a handler.

### (b) Protobuf message in, including the Resolver

```go
type protoResolver struct{}

func (protoResolver) Resolve(ctx context.Context, v any) (any, error) {
	switch x := v.(type) {
	case proto.Message:
		return msgView{x.ProtoReflect()}, nil
	case protoreflect.List: // GUESS: I have to allowlist these too, because msgView.Get hands them back as foreign
		return listView{x}, nil
	case protoreflect.Map:
		return mapView{x}, nil
	}
	return nil, fmt.Errorf("unresolvable %T", v)
}

type msgView struct{ m protoreflect.Message }

func (o msgView) Get(ctx context.Context, key string) (any, bool, error) {
	fd := o.m.Descriptor().Fields().ByJSONName(key) // GUESS: JSON names, since the transform authors see the protojson wire shape, not Go field names
	if fd == nil {
		return nil, false, nil
	}
	if fd.HasPresence() && !o.m.Has(fd) {
		return nil, false, nil // GUESS: unset optional == absent, not null; the library is silent on what a view *should* do here
	}
	return fieldValue(fd, o.m.Get(fd)), true, nil
}

func (o msgView) Range(ctx context.Context, fn func(string, any) bool) error {
	// GUESS: field-number order, matching protojson; nothing tells me whether $keys order for a view is expected to match anything
	fields := o.m.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if fd.HasPresence() && !o.m.Has(fd) {
			continue
		}
		if !fn(fd.JSONName(), fieldValue(fd, o.m.Get(fd))) {
			return nil
		}
	}
	return nil
}

func fieldValue(fd protoreflect.FieldDescriptor, v protoreflect.Value) any {
	switch {
	case fd.IsList():
		return v.List() // foreign; Resolve runs again on every read, no memo
	case fd.IsMap():
		return v.Map()
	}
	switch fd.Kind() {
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return v.Int() // exact int64; Marshal will write digits, NOT the protojson quoted-string form
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return v.Uint()
	case protoreflect.EnumKind:
		return string(fd.Enum().Values().ByNumber(v.Enum()).Name()) // GUESS: names as protojson does; a transform comparing to 2 will now be false
	case protoreflect.BytesKind:
		return v.Bytes() // requires Env.BytesEncoding, or the first $string on it is E1001
	case protoreflect.MessageKind:
		m := v.Message().Interface()
		if ts, ok := m.(*timestamppb.Timestamp); ok {
			return ts.AsTime().UTC().Format(time.RFC3339Nano) // GUESS: no guidance on timestamps; RFC3339 so $toMillis works
		}
		return m // foreign again
	default:
		return v.Interface()
	}
}
```

Then the gRPC handler:

```go
func (s *svc) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.UserResponse, error) {
	u := s.store.Load(req.Id)
	env := &jsonata.Env{Resolver: protoResolver{}, BytesEncoding: base64.StdEncoding}

	out, present, err := s.expr.Eval(ctx, u, env)
	if err != nil {
		return nil, grpcStatus(err)
	}
	if !present {
		return &pb.UserResponse{}, nil // GUESS
	}
	lim := s.expr.Limits()
	out, err = jsonata.Materialize(ctx, out, env, &lim) // GUESS: nil would give this pass DefaultLimits, a bigger budget than the eval had; I assume passing expr.Limits() is what's intended
	if err != nil {
		return nil, grpcStatus(err)
	}
	b, err := jsonata.Marshal(out, env)
	if err != nil {
		return nil, grpcStatus(err)
	}
	var resp pb.UserResponse
	if err := protojson.Unmarshal(b, &resp); err != nil { // GUESS: this fails on int64 fields because Marshal wrote digits and protojson accepts both, so actually fine; but a JS client hitting the same bytes rounds them
		return nil, status.Error(codes.Internal, "transform shape")
	}
	return &resp, nil
}
```

Three library calls plus a bytes round-trip to get from a proto to a proto. Every line of `fieldValue` is a representation decision the library explicitly declines to make and gives no worked example for.

### (c) Two fields of a large result

```go
// startup: validate configured selections
keys, ok := expr.Fields()
if !ok { return fmt.Errorf("transform is not selectable") }
for _, k := range wanted { if !slices.Contains(keys, k) { return fmt.Errorf("unknown field %q", k) } }

// request:
ev, err := expr.Prepare(ctx, in, env)
if err != nil { return err }
defer ev.Close()

res := jsonata.NewObject(len(wanted))
for _, k := range wanted {
	v, present, err := ev.Select(ctx, k)
	if err != nil {
		return err // GUESS: doc says a language error in one field leaves the others selectable, but I don't know if I want partial results, and nothing helps me decide
	}
	if present {
		res.Set(k, v)
	}
}
b, err := jsonata.Marshal(res, env) // GUESS: Marshal is a read, so doing it before Close is legal under the "must not mutate" rule
```

There is no multi-select. Every integrator will write this loop, and every one will get the partial-failure policy slightly different.

### (d) Errors and absence

```go
func writeTransformError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		http.Error(w, "timeout", http.StatusGatewayTimeout)
		return
	case errors.Is(err, jsonata.ErrMalformedInput):
		http.Error(w, "malformed JSON", http.StatusBadRequest)
		return
	case errors.Is(err, jsonata.ErrBudget):
		http.Error(w, "transform too expensive", http.StatusUnprocessableEntity) // GUESS: could be my config or their body; the error cannot tell me which
		return
	}
	var e *jsonata.Error
	if !errors.As(err, &e) {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	switch e.Code.Class() {
	case jsonata.ClassRaised:
		msg, _ := e.Value.(string) // GUESS: $error's Value is a string when the author passed one; docs say "the message the expression supplied", type unstated
		http.Error(w, msg, http.StatusUnprocessableEntity)
	case jsonata.ClassType, jsonata.ClassEvaluation:
		slog.Error("transform failed", "code", e.Code, "offset", e.Offset, "err", e) // Error() is input-free, safe
		http.Error(w, "transform failed", http.StatusUnprocessableEntity) // GUESS: "a"+1 is a config bug if the field is always a string, a body bug if it isn't; I blame the body
	case jsonata.ClassEngine:
		if e.Code == jsonata.CodeInexact { /* data-dependent; see footguns */ }
		http.Error(w, "internal", http.StatusInternalServerError)
	default:
		http.Error(w, "internal", http.StatusInternalServerError)
	}
}
```

### (e) Startup, caching, lifecycle

```go
type key struct {
	src string
	lim jsonata.Limits // comparable, per the doc
}

type Compiler struct {
	mu    sync.Mutex
	cache *lru.Cache[key, *jsonata.Expression]
}

func (c *Compiler) Get(src string, lim *jsonata.Limits) (*jsonata.Expression, error) {
	full := jsonata.DefaultLimits()
	if lim != nil { full = *lim } // GUESS: I have to reimplement "zero means default" to build the key before compiling, or compile first and key on expr.Limits() after; the doc says the latter but then every miss compiles under the lock
	k := key{src, full}
	if e, ok := c.cache.Get(k); ok { return e, nil }
	e, err := jsonata.Compile(src, lim) // GUESS: no ctx, so I trust MaxExpressionBytes to bound wall time; no singleflight
	if err != nil { return nil, err }
	c.cache.Add(key{src, e.Limits()}, e)
	return e, nil
}

// Shared Env, built once. Bindings are borrowed and their values can come back by identity,
// so nothing downstream may mutate a result. I deep-copy nothing; I just pray.
var sharedEnv = &jsonata.Env{
	BytesEncoding: base64.StdEncoding,
	Bindings:      map[string]any{"tenant": tenantDefaults}, // GUESS: safe only if every consumer treats results as read-only
}
```

Per-tenant budgets mean per-tenant cache keys, because Limits live on the Expression. Acceptable, but the doc should say "this is how you tier" rather than leave me to derive it.

## What I would change

1. **Give `Eval` the same egress that `EvalJSON` has.** `EvalJSON` already contains "one pass that resolves foreign values through env and encodes"; that pass is exactly what the protobuf path needs and it is not exposed. Add:
   ```go
   func (e *Expression) EvalMarshal(ctx context.Context, input any, env *Env) (out []byte, present bool, err error)
   ```
   and define `EvalJSON` as `Unmarshal` followed by `EvalMarshal`. Today the non-JSON path is `Eval`, then `Materialize` with a second budget I must thread by hand, then `Marshal`. Also either make `Marshal` use `env.Resolver` or stop taking an `*Env` that suggests it does; the current signature is a trap.

2. **Make `Materialize` canonical, or add `Canonical`.** Typed code does not want "the same Go value it went in as"; it wants a fixed set of types. Today a result can be `map[string]any`, `*Object`, `map[string]int64`, `[]any`, `[]string`, `int32`, `json.Number`, `float32`, or a carried proto message. Promise: after `Canonical(ctx, v, env, limits)`, every object is `*Object`, every array `[]any`, every number `int64 | float64 | *big.Int`, every string `string`, every bytes `[]byte`, no `json.Number`, no `json.RawMessage`, no foreign value. Eight cases. If `Materialize` must keep its identity-return optimization, ship both.

3. **Ship the protobuf Resolver as a sibling package.** `github.com/openbindings/jsonata-evaluator/go/jsonataproto` with `func NewResolver(opts ...Option) jsonata.Resolver` implementing the protojson mapping (JSON names, enum names, `Timestamp` as RFC3339, `Duration` as string, `Struct`/`Value` unwrapped, `Any` refused unless opted in, int64 as int64). The main package is right that presenting an int64 is a binding decision; it is wrong to make every integrator reinvent it. Nobody will make the same choices, and the OpenBindings project of all places should not have five private protobuf resolvers.

4. **Give `Unmarshal` limits.** `Compile`, `MustCompile`, `Materialize` all take `*Limits`; `Unmarshal` silently uses `DefaultLimits()`. So an expression compiled with `MaxBytes: 4 << 20` accepts a 64 MiB input when I decode it myself but 4 MiB when `EvalJSON` decodes it. Change to `Unmarshal(data []byte, limits *Limits) (any, error)` or add `UnmarshalLimits`. Same for `Object.UnmarshalJSON`, which cannot take an argument, so document that it is default-bounded and not for untrusted text.

5. **Add multi-select that returns the object.**
   ```go
   func (v *Evaluation) Project(ctx context.Context, keys ...string) (*Object, error)
   ```
   Returns an `*Object` in constructor order with only present fields, failing on the first field error (matching `Complete`'s policy), or with a documented partial policy. This is the "compute only requested fields of the result" case verbatim; today it is a loop every integrator writes with a different failure policy.

## Footguns

- **The `encoding/json` trap.** Every existing Go service already has `map[string]any` from `json.Unmarshal`. Passing it works, the integral-float64-is-an-integer rule makes the rounded ID look exact, and nobody notices until the 9007199254740993 case. The library cannot detect this and the doc says so, but the frictionless path is the wrong path.
- **"Select is not a guard" is plan-dependent.** Under `ReasonNone` a sibling `$assert` never runs; under `ReasonArrayInput` (same expression, array root) the whole expression evaluates once and the assert fires on every `Select`. So whether a transform's guard is honored depends on the shape of the request body. README rule 9 is true for success only.
- **`CodeInexact` is data-dependent.** `nanos * 1e-9` works for every timestamp below 2^53 and fails on the first one above. The expression passes tests, passes staging, fails in production on one row. The doc gives the workaround (`/ 1e9`) but the config author never reads the Go doc.
- **Results alias `Env.Bindings`.** `{ "defaults": $defaults }` returns the binding map by identity. The first downstream `m["x"] = ...` corrupts every future request on the shared Env, and it is a data race if two requests are in flight. There is no defensive-copy option.
- **Success at `Eval`, failure at `Marshal`.** `$` over a proto root returns the proto; `$.avatar` over a bytes field returns `[]byte`. Both evaluate cleanly, then `Marshal` returns E1001 unless I remembered `Materialize` and `BytesEncoding`. The failure is on the second call, in the encoder, and reads like an encoding bug.
- **Duplicate member names now reject the body.** `encoding/json` keeps the last; `Unmarshal` and `EvalJSON` refuse with E1006. Migrating an existing endpoint to `EvalJSON` will start returning 400 to clients that worked yesterday. This needs to be a headline, not a table row in DIVERGENCES.md.
- **Budgets are per evaluation and large.** 64 MiB `MaxBytes` and 8M `MaxWork` per evaluation, times concurrency, is the real ceiling. Because Limits live on the Expression, I cannot scale a budget by request size or tenant without a cache entry per tier.
- **`Prepare` succeeding proves nothing.** The prelude runs on the first `Select`, and its failure is "reported by every selection", so a loop over five selections logs the same error five times.
- **`Select` paths are not JSONata paths.** `Select(ctx, "items", "name")` yields absent when `items` is an array, where `items.name` in the language would map. Anyone who thinks of the path argument as a JSONata path will be surprised.
- **Resolver cost multiplies.** `Resolve` runs once per read, no engine memo. `items[price > 10].name` over a repeated proto field resolves each element at least twice; `$sort` by a foreign field resolves per comparison. A per-request memoizing Resolver is required, not optional, and the doc mentions it in passing.
- **`nil` binding is null, not absent.** `$x ?? "default"` with `Bindings["x"] = nil` returns null. Optional bindings must be deleted from the map, not set to nil.
- **Map key sort per observation.** `$keys`, `$each`, `$merge`, object construction, and `$string` each sort a `map[string]any`'s keys again. A 10k-key map touched in a loop is O(n log n) per touch. The doc says use `*Object`; my inputs are whatever the decoder gave me.
- **`Error.Value` leaks input to structured loggers.** `Error()` is input-free, but `slog.Any("err", e)` on some handlers, `%+v`, and any JSON logger that walks the struct will print `Value`, which for `$error` is whatever the expression put there, possibly built from the body.

## Things the doc comments leave unclear

- `Compile` "fails with an *Error of ClassSyntax", but exceeding `MaxExpressionBytes` or `MaxDepth` is presumably `CodeBudget` (`ClassEngine`). Which is it, and does `errors.Is(err, ErrBudget)` hold on a compile failure?
- "cancellation and deadlines ... are checked at intervals bounded by MaxWork units". Bounded above by 8M units? That reads as "checked at least once per budget", which is useless. What is the worst-case cancellation latency in work units?
- Does `Unmarshal` refuse invalid UTF-8 in the raw text, replace it with U+FFFD as `encoding/json` does, or accept it and refuse later on observation? Same question for ` ` and for a JSON text with a BOM.
- Does `EvalJSON` check `len(input)` against `MaxBytes` before decoding, or decode until the budget trips? Matters for whether I need `MaxBytesReader` in front of it.
- Is `[N]T` (fixed arrays, so `uuid.UUID`, `[16]byte`) admitted? Not in the list, so presumably foreign; say so, and say `time.Time` is foreign too. Both are in every struct I own.
- For `items[price > 10].name` over a foreign array, exactly how many `Resolve` calls per element? The cost model is "once per read" without defining a read.
- What is the concrete type of `Error.Value` for `$error("msg")` and `$error({ "code": 1 })`? A `string` and an `*Object`? Carried as given?
- Is `Error.Offset` populated for runtime errors at the token that failed (the example implies yes for T2001), and is it `-1` for `ClassRaised`?
- `Materialize` with `nil` limits uses `DefaultLimits`; is the intended idiom to pass `&expr.Limits()`? Say so or make it use the expression's.
- Is `Marshal` of `map[string]int64`, `[]string`, and other generic admitted containers supported? "a map in sorted key order" suggests yes; confirm.
- Under `ReasonDynamicShape`, `Select(ctx, "id")` on a result that turns out to be a string or array: absent, or error?
- Are negative `Limits` fields an error, treated as zero, or undefined?
- Does `Object` (returned by `Unmarshal`) support concurrent reads from many goroutines? "not safe for concurrent mutation" implies yes; state it, because I will share decoded inputs across an `Evaluation`'s concurrent selections.
- What does `Range` order mean for an `ObjectView` in terms of anything the caller can expect? The doc says the view's order is observed, but gives no recommendation for a protobuf view (declaration order? field number?).
- `Encoder.Encode` takes `any` with no `present` flag. What do I pass for an absent result, and does `Encode(nil)` write `null`?
- Does `Env.Now` need to be goroutine-safe when the Env is shared? "called at most once per Eval" across concurrent Evals implies yes.
- `Expression.Fields` says ok is false unless "fields are all pure"; is `$now` pure here (the timestamp section says it is fixed per Prepare, so presumably yes)? Which built-ins are currently qualified for `ReasonUnsupportedCall` is unlisted, so I cannot predict at config-validation time which transforms will be selective.

## On the concept

The regex framing is the right lifecycle model and I would defend it to my team on that basis alone: compile once, share freely, no goroutines, no global registry, no host functions, a bounded engine that cannot panic me, and a divergence ledger instead of a "we match the JS exactly" promise nobody can keep. Documentation-as-authority is fine from my seat because I do not run the JavaScript; I run this and the TypeScript SDK, and what I actually care about is that the same transform in the same OpenBindings document gives the same bytes from both. The README makes that a "quality commitment of the project", not a property of the class. That is honest, but from where I sit it is the only property that matters, and I would want the portable-core paragraph promoted from "an observation" to a tested contract with a fixture set I can point CI at. Where the analogy breaks is egress. A regex returns strings and offsets, which are already the host's one string type. This returns a tree whose concrete Go shape depends on whether each node was carried or constructed, which is a property of the expression's text, not of my code. `regexp` never hands me back "the same []byte you passed in, or a string, depending on the pattern". The carriage promise is essential for exactness and I want it kept; it just means "host-native values" on the output side needs a canonicalizing step the package half-provides.

"The host defines what a number is" is the right call for integers and this package does it better than anything I have used: exact int64, `*big.Int` above that, comparison by value across representations, and a refusal rather than a rounding when an integer meets a decimal. `CodeInexact` will annoy config authors and I still want it, because the alternative is the JavaScript behavior where `id * 1.0` silently loses the low bits. What I would want promised, explicitly and in one place, for an int64 ID: (1) if it arrives as `int64`, `*big.Int`, or `json.Number` digits and the expression only moves it, it leaves as the identical Go value and `Marshal` writes the identical digits, never exponent notation, never quoted; (2) comparing it to the same digits in any other admitted representation is equal; (3) `Int64()` recovers it exactly from any of those; (4) an integral `float64` is trusted as exact and the package cannot know it was rounded upstream, so the caller owns the decoder. All four are in the text, spread across the package doc, `Marshal`, and `Int64`. Put them together under a heading called "Integer identifiers" and I will link my team to it.

The gap the library cannot close, and should say out loud, is that an int64 from a JSON body and an int64 from a protobuf field have different wire expectations on the way out. protojson writes 64-bit integers as quoted strings precisely because JSON consumers round them; this package's `Marshal` writes digits. So a transform that carries a proto int64 to a JSON response will produce a number a browser client rounds, and one that carries it to a proto response works only because protojson accepts both spellings. The package is right that the Resolver author chooses how a proto int64 presents, and right that the binding, not the evaluator, owns that choice. But a Resolver that presents it as `"123"` breaks `id = 123` and `id + 1` in the expression, and one that presents it as `int64` breaks the JSON wire. There is no representation that is correct on both sides, which is exactly why I want the project to ship the protobuf Resolver with its decision written down, rather than have me make it at 4pm the day before launch.
