# Iteration-3 panel: application integrator

Cold read of `b501e30`. Lens: the developer integrating this into a production Go service (HTTP and gRPC handlers, untrusted configuration expressions).

## Grades

| Criterion | Grade | One-line justification |
| --- | --- | --- |
| Idiomatic fit for Go | B+ | `MustCompile`/`String`/`errors.Is`/`iter.Seq`/idempotent `Close` are all right; the `(value, present, err)` triple is defensible but un-Go-like, and `Marshal(v, env)` taking an `*Env` that it then half-ignores (uses `BytesEncoding`, refuses to use `Resolver`) is not. |
| Ergonomics of the common path | B- | JSON-in/JSON-out over HTTP is one call (`EvalJSON`) and it is good; proto-in requires a hand-rolled view layer plus `Eval` + `Materialize` + `Marshal` + `protojson.Unmarshal`, and handing any result to typed Go code means a four-way numeric switch and a two-way object switch I write myself. |
| Correctness and footgun risk | B- | The numeric model is the best I have seen in a JSON tool; but `E1001` conflates "client sent bad JSON" with "my Resolver is missing a type", `Unmarshal` refuses payloads `encoding/json` accepts, `Select` silently skips guards, results alias my shared bindings, and the regex dialect is "pending ruling". |
| Performance headroom the API permits | A- | Compile-once, borrow-by-reference, lazy admission, lazy `RawMessage`, `Prepare`/`Select` work sharing, `Reads()` for fetch planning, views over proto without conversion. Missing only: an aggregate (per-process) budget, encoder reuse, and any streaming of large results. |
| Concept soundness | B+ | The framing is right and the value-model split is exactly where it should be; but from an integrator's seat the class disclaims the one property I actually need across a Go service and a JS service running the same config document, and config authors calibrate on the JS playground, not the docs. |
| Overall | B | I would ship it behind an HTTP handler tomorrow with `EvalJSON`; I would not put it behind gRPC or typed code until the error-code split and the aliasing story are fixed, and I would not accept untrusted config expressions until the regex ruling lands. |

## The integration I would write

### (a) HTTP handler over a decoded JSON body

```go
type transformHandler struct {
	expr *jsonata.Expression // compiled at config load, see (e)
	env  *jsonata.Env        // shared, read-only: Resolver nil, BytesEncoding nil (base64.Std)
}

func (h *transformHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		body = []byte("null") // GUESS: "not one complete JSON value" means an empty body is an error, so a bodyless request must be presented as null
	}
	ctx, cancel := context.WithTimeout(r.Context(), 200*time.Millisecond)
	defer cancel()

	out, present, err := h.expr.EvalJSON(ctx, body, h.env)
	if err != nil {
		status, msg := httpStatus(err)
		http.Error(w, msg, status)
		return
	}
	if !present {
		w.WriteHeader(http.StatusNoContent) // GUESS: nothing tells me what absent means on the wire; 204 is my policy
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(out) // GUESS: out has no trailing newline (Marshal semantics, not Encoder)
}

func httpStatus(err error) (int, string) {
	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout, "transform timed out"
	}
	var e *jsonata.Error
	if !errors.As(err, &e) {
		return http.StatusInternalServerError, "internal" // GUESS: everything non-ctx is an *Error; the doc says so for Eval but not for the Unmarshal half of EvalJSON
	}
	switch {
	case errors.Is(err, jsonata.ErrUnsupportedValue):
		// GUESS: this is where a malformed request body lands (Unmarshal: CodeUnsupportedValue).
		// It is ALSO where "foreign value, no Resolver", "Resolver returned junk", and
		// "function escaped" land. I cannot tell a 400 from a 500 here. I picked 400
		// and I will be wrong the first time a Resolver bug ships.
		return http.StatusBadRequest, "malformed body"
	case errors.Is(err, jsonata.ErrBudget):
		return http.StatusUnprocessableEntity, "transform exceeded its budget"
	}
	switch e.Code.Class() {
	case jsonata.ClassRaised:
		// GUESS: $error("text") puts "text" in e.Value, not e.Message, because Message
		// "never includes content derived from the input" and the text may be. So the
		// client-visible string has to come from Value and I have to sanitise it myself.
		return http.StatusUnprocessableEntity, fmt.Sprint(e.Value)
	case jsonata.ClassType, jsonata.ClassEvaluation:
		// GUESS: is this the config author's bug (T2001 because they wrote it wrong) or the
		// payload's shape (T2001 because a string arrived where a number was expected)?
		// No field tells me. I log Code/Offset/Token and return 500.
		return http.StatusInternalServerError, "transform failed"
	default:
		return http.StatusInternalServerError, "transform engine error"
	}
}
```

### (b) gRPC handler over a protobuf message, with the Resolver

```go
// protoResolver exposes an allowlist of message types through protoreflect.
// GUESS: "allowlist over known types, never reflection over arbitrary structs" is a
// warning about Go struct reflection; I take protoreflect over an allowlisted set of
// descriptors to be what the doc wants, but it does not say so.
type protoResolver struct {
	allow map[protoreflect.FullName]struct{}
}

func (r protoResolver) Resolve(ctx context.Context, v any) (any, error) {
	switch x := v.(type) {
	case proto.Message:
		m := x.ProtoReflect()
		if _, ok := r.allow[m.Descriptor().FullName()]; !ok {
			return nil, fmt.Errorf("message %s is not exposed to transforms", m.Descriptor().FullName())
			// GUESS: this comes back to my handler wrapped in an *Error; with which Code? Unstated.
		}
		return msgView{m}, nil
	case protoreflect.List:
		return listView{x}, nil
	case protoreflect.Map:
		return mapView{x}, nil // GUESS: keys must be presented as strings; I stringify int keys
	}
	return nil, fmt.Errorf("type %T is not exposed to transforms", v)
}

type msgView struct{ m protoreflect.Message }

func (v msgView) Get(ctx context.Context, key string) (any, bool, error) {
	fields := v.m.Descriptor().Fields()
	fd := fields.ByJSONName(key)
	if fd == nil {
		fd = fields.ByName(protoreflect.Name(key)) // binding decision: accept both spellings
	}
	if fd == nil {
		return nil, false, nil
	}
	if fd.HasPresence() && !v.m.Has(fd) {
		return nil, false, nil // absent, not null: unset optional/message/oneof
	}
	return present(fd, v.m.Get(fd)), true, nil
}

func (v msgView) Range(ctx context.Context, fn func(key string, value any) bool) error {
	var stop bool
	v.m.Range(func(fd protoreflect.FieldDescriptor, val protoreflect.Value) bool {
		// proto Range visits only populated fields, in field-number order.
		// GUESS: that order is what $keys and $string will show; fine, but Range and Get
		// disagree about proto3 scalars at their default (Get says present, Range never
		// visits them). Nothing says the engine requires the two to agree.
		if !fn(fd.JSONName(), present(fd, val)) {
			stop = true
		}
		return !stop
	})
	return nil
}

func present(fd protoreflect.FieldDescriptor, val protoreflect.Value) any {
	switch {
	case fd.IsList():
		return val.List() // foreign; resolved when read
	case fd.IsMap():
		return val.Map() // foreign; resolved when read
	}
	switch fd.Kind() {
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return val.Int() // int64, exact
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return val.Uint() // uint64: becomes *big.Int above MaxInt64 on read
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return int32(val.Int())
	case protoreflect.FloatKind:
		return float32(val.Float()) // GUESS: hand it float32 so carriage keeps the proto width; the engine widens on arithmetic. Unstated what Marshal prints for a carried float32.
	case protoreflect.DoubleKind:
		return val.Float()
	case protoreflect.BytesKind:
		return val.Bytes() // presented via Env.BytesEncoding when used as a string (base64.Std matches proto JSON)
	case protoreflect.EnumKind:
		return string(fd.Enum().Values().ByNumber(val.Enum()).Name()) // binding decision: names
	case protoreflect.MessageKind, protoreflect.GroupKind:
		if fd.Message().FullName() == "google.protobuf.Timestamp" {
			return val.Message().Interface().(*timestamppb.Timestamp).AsTime().UTC().Format(time.RFC3339Nano)
		}
		return val.Message().Interface() // foreign proto.Message; Resolve is called again when read
	default:
		return val.Interface() // bool, string
	}
}

type listView struct{ l protoreflect.List }

func (v listView) Len(ctx context.Context) (int, error) { return v.l.Len(), nil }
func (v listView) At(ctx context.Context, i int) (any, error) {
	// GUESS: I do not have the FieldDescriptor here, so I cannot run present() on
	// elements; I have to close over it. The view interfaces give me no way to carry
	// per-element conversion context except my own struct fields.
	return v.conv(v.l.Get(i)), nil
}
```

```go
func (s *server) GetOrder(ctx context.Context, req *pb.GetOrderRequest) (*pb.GetOrderResponse, error) {
	order, err := s.store.Get(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	env := *s.baseEnv // copy the shared Env, then attach per-request bindings
	env.Bindings = map[string]any{
		"principal": principalMap(ctx), // map[string]string: admitted
	}

	out, present, err := s.expr.Eval(ctx, order, &env) // order is *pb.Order: foreign, root goes through Resolve
	if err != nil {
		return nil, grpcStatus(err)
	}
	if !present {
		return &pb.GetOrderResponse{}, nil // GUESS: again my policy
	}
	// `$` or any carried sub-message comes back as the proto itself, and Marshal refuses
	// foreign values even though env.Resolver is sitting right there. So:
	lim := s.expr.Limits()
	out, err = jsonata.Materialize(ctx, out, env.Resolver, &lim) // GUESS: this is a second, separate budget of the same size
	if err != nil {
		return nil, grpcStatus(err)
	}
	b, err := jsonata.Marshal(out, &env)
	if err != nil {
		return nil, status.Error(codes.Internal, "encode")
	}
	var resp pb.GetOrderResponse
	if err := protojson.Unmarshal(b, &resp); err != nil { // the only way to get a proto out is through JSON text
		return nil, status.Error(codes.Internal, "transform produced a response the schema rejects")
	}
	return &resp, nil
}
```

### (c) Selecting two fields of a large result

```go
ev, err := s.expr.Prepare(in, &env)
if err != nil {
	return err // only an invalid Env fails here; foreign-value and budget failures surface in Select
}
defer ev.Close()

if p := ev.Plan(); !p.Selective() {
	selectiveFallback.WithLabelValues(p.Reason.String()).Inc() // otherwise I never learn the optimisation is off
}

// Evaluation is documented safe for concurrent use, so the two fields can run in parallel.
var id, name any
var idOK, nameOK bool
g, gctx := errgroup.WithContext(ctx)
g.Go(func() (err error) { id, idOK, err = ev.Select(gctx, "id"); return })
g.Go(func() (err error) { name, nameOK, err = ev.Select(gctx, "user", "display_name"); return })
// GUESS: "user" is a constructor key and "display_name" a literal key inside it, so this is selective
// two levels down; if "user" were `$.user` (carried) the second element becomes a plain lookup.
if err := g.Wait(); err != nil {
	return err // GUESS: a prelude failure surfaces identically from both Selects, so I only see it once via errgroup
}
// id and name alias the Evaluation until Close; they are scalars here so nothing to copy.
// If a selected field were an *Object I would not mutate it before Close, and after Close I still
// would not, because it may alias `in` (see Footguns).
```

### (d) Errors, absence, and handing values to typed code

```go
// Absence: always the three-way switch. `v, _, err :=` is the bug the doc warns about and
// it is exactly what every reviewer will let through, because it looks like Go.
out, present, err := expr.Eval(ctx, in, env)
switch {
case err != nil:
	return err
case !present:
	return errNoResult
case out == nil:
	return errNullResult
}

// Typed extraction. There is no helper, so this lives in my code, and every service will
// have a subtly different copy.
func member(v any, key string) (any, bool) {
	switch o := v.(type) {
	case *jsonata.Object:
		return o.Get(key)
	case map[string]any:
		x, ok := o[key]
		return x, ok
	}
	return nil, false
}

func asInt64(v any) (int64, bool) {
	switch x := v.(type) {
	case int64:
		return x, true // constructed by arithmetic, or carried from a proto int64
	case json.Number:
		i, err := x.Int64() // carried from Unmarshal input: SAME expression, different Go type
		return i, err == nil
	case *big.Int:
		return 0, false // above int64, refuse
	case int, int8, int16, int32, uint, uint8, uint16, uint32, uint64:
		return reflect.ValueOf(x).Convert(reflect.TypeOf(int64(0))).Int(), true // carried typed Go input
	case float64:
		// GUESS: a merely selected integer never arrives as float64 (carriage), but an
		// integer that touched float arithmetic does; whether to accept 3.0 as 3 is my call.
		if x == math.Trunc(x) && math.Abs(x) < 1<<53 {
			return int64(x), true
		}
	}
	return 0, false
}
```

What I actually expect most teams to write, because it is the only path that handles every representation without a switch:

```go
b, _ := jsonata.Marshal(out, env)
var typed OrderSummary
json.Unmarshal(b, &typed) // text round trip, and now 9007199254740993 into an `any` field is 9007199254740992 again
```

### (e) Startup: limits, cache, shared Env

```go
// Defaults are per-evaluation: 64 MiB bytes, 8M work, 1M output nodes. At 200 in-flight
// requests that is a 12.8 GiB worst case. Size them against fan-out, not against one call.
var limits = jsonata.Limits{
	MaxExpressionBytes: 32 << 10,
	MaxDepth:           64,
	MaxRecursion:       256,
	MaxOutputNodes:     1 << 16,
	MaxWork:            1 << 20,
	MaxBytes:           8 << 20,
	MaxIntegerBits:     256,
}

type exprKey struct {
	src    string
	limits jsonata.Limits // GUESS: Limits stays a struct of plain ints, so it is comparable and map-keyable; nothing promises that
}

type exprCache struct {
	mu  sync.Mutex
	lru *lru.Cache[exprKey, *jsonata.Expression] // bounded: sources come from config documents I do not fully trust
}

func (c *exprCache) get(src string) (*jsonata.Expression, error) {
	k := exprKey{src, limits}
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.lru.Get(k); ok {
		return e, nil
	}
	e, err := jsonata.Compile(src, &limits)
	if err != nil {
		// GUESS: this is an *Error with ClassSyntax and Line/Column filled in, so I can
		// report a position to the config author. Compile's doc comment does not say
		// what it returns.
		return nil, fmt.Errorf("transform %q: %w", truncate(src), err)
	}
	c.lru.Add(k, e)
	return e, nil
}

// One Env for the process; per-request code copies the struct and sets Bindings.
var baseEnv = &jsonata.Env{
	Resolver:      protoResolver{allow: exposedMessages},
	BytesEncoding: base64.StdEncoding,
}

// At config load I also want to validate the expression against the schema of the
// payload it will run over:
paths, known := e.Reads()
if known {
	checkAgainstSchema(paths) // GUESS: "a binding makes known false" reads as "any `$x := ...` disables this", which is nearly every non-trivial transform
}
// I cannot pre-check selectivity: Plan needs Prepare, Prepare needs an input, and
// ReasonArrayInput depends on that input.
```

## What I would change

**1. Split `CodeUnsupportedValue` so a malformed input is its own code, with its offset into the input.**
Today `E1001` covers: malformed JSON in `Unmarshal`/`EvalJSON`/a `json.RawMessage`, a duplicate member name, invalid UTF-8, a foreign value with no Resolver, a Resolver returning something inadmissible, and a function escaping. The first three are the client's fault (400); the rest are my bugs (500). I would add:

```go
CodeMalformedInput Code = "E1006" // input text is not one complete JSON value, has a duplicate member, or is not valid UTF-8
```

and document that for this code `Error.Offset` is the byte offset into the *input*, with `Line`/`Column` likewise. `Error.Offset` is currently defined only as "into the expression source", which is meaningless for `Unmarshal`. Also state the `Code` a wrapped Resolver error carries (I would give it its own: `CodeResolver`), because I need to route it to 500 and page on it, not treat it like a bad body.

**2. Make `Marshal` and `Encoder` resolve foreign values through `env.Resolver`, and give `Materialize` an `*Env`.**
`Marshal(v, env)` already receives the `*Env` that holds the Resolver, then refuses foreign values and tells me to call `Materialize` with the same Resolver pulled back out, plus a `*Limits` I have to fetch from the Expression and take the address of. `EvalJSON` already does the right thing internally. Wording:

> Marshal encodes a result as JSON text ... A foreign value is resolved through env's Resolver as an evaluation would resolve it, bounded by limits; it is an error (CodeUnsupportedValue) when env has no Resolver.

and `func Materialize(ctx context.Context, v any, env *Env, limits *Limits) (any, error)`. That deletes two lines and one guess from every proto integration.

**3. Give typed consumers one supported way off the result domain.**
The doc is honest that a result number is one of `int64`, `float64`, `*big.Int`, or a carried `json.Number`/`int`/`uint64`/`float32`/..., and an object is one of `map[string]any` or `*Object`. Every service will write `asInt64` and `member` from (d), each slightly wrong. I want, at minimum:

```go
// Int64 reports v's integer value when v denotes an integer that fits in int64, across every admitted representation.
func Int64(v any) (int64, bool)
// Member returns the member named key of an object result, whether map or *Object.
func Member(v any, key string) (any, bool)
```

and ideally `func Decode(v any, target any, env *Env) error`, defined as equivalent to `json.Unmarshal(Marshal(v, env), target)` but free to skip the text, because that round trip is what people will otherwise do and it silently re-introduces the 2^53 loss the whole package exists to prevent.

**4. Put the input location of an evaluation failure on `Error`, without content.**
When a `T2001` fires over a payload I did not write, from an expression I did not write, `Offset`/`Token` tell me where in the expression, and nothing tells me where in the input. Add:

```go
// Path is the sequence of member keys and array indices from the root of the input to the value the failing operation read, when known, rendered without content ("items", "3", "price").
Path []string
```

This is what lets me answer "was it the config or the payload" from a log line without logging the payload, which is the one question I cannot answer today.

**5. Fix the `Close` doc so it does not promise what the Ownership section takes away, and lower the byte default.**
`Close` says returned values "may be mutated"; the Ownership section says results may alias the input and `Env.Bindings` by identity, and a constructed container's members may be carried. Both cannot be true for a caller who shares bindings across requests. Wording for `Close`:

> ...values already returned are no longer shared with the Evaluation. They may still alias the input and Env.Bindings, as any result may; mutate them only if you own those.

And `MaxBytes` default 64 MiB is a single-caller number; for a library that expects to sit under concurrent request handlers, 8 MiB is a default people can raise rather than one they discover at OOM.

## Footguns

- **`E1001` routes a bad request body and a missing Resolver case to the same branch.** The first week in prod, a Resolver gap will return 400 to a client and never page anyone.
- **`Unmarshal` (and so `EvalJSON`) rejects duplicate member names.** `encoding/json`, `protojson`, and every upstream API gateway accept them last-wins. A payload the rest of the stack accepted is refused only at the transform, with a code that looks like a client error.
- **Two decoders, two `$keys` orders.** `Unmarshal` gives document order via `*Object`; `json.Decoder.UseNumber` gives sorted order via `map`. `$string($)` of the same document differs by which decoder the binding used. If anything downstream hashes, signs, or diffs that text, it is decoder-dependent.
- **Same expression, different Go types out.** `{ "id": user_id }` yields `json.Number` over an `Unmarshal` input and `int64` over a proto input; `{ "id": user_id + 0 }` yields `int64` for both; a `uint64` proto field above `MaxInt64` yields `*big.Int`. Typed code that only tested one path will type-assert and panic on the other.
- **`Select` is not a guard, and silently is not selective.** `{ "check": $assert(cond, "bad"), "data": ... }` with `Select("data")` skips the assert. Config authors will write that. And when the plan is not selective every `Select` is a full evaluation; nothing but `Plan()` tells you, and `Plan()` cannot be asked at config-load time because `ReasonArrayInput` depends on the input.
- **Results alias shared state.** Bind a process-wide `map[string]any` in `Env.Bindings`, let `$config` reach the result, hand the result to code that calls `Object.Set` on a nested member: you just mutated every future request's config. The `Close` doc actively suggests this is safe.
- **Per-evaluation budgets, no aggregate, and `Materialize` is a second budget.** Defaults × concurrency is the memory bound. A `$` over a large input costs one output node, then `Materialize` and `Marshal` pay for all of it under a fresh budget.
- **Resolver panics propagate.** `net/http` recovers per connection; gRPC does not without a recovery interceptor. "No expression can terminate the process" is true only for the engine's own code; an expression drives my Resolver with any key in any order, and my view code is where the nil dereference lives.
- **`Resolve` is called once per read.** `items.price` over a repeated field of N messages is N+1 Resolver calls, each charged as work. Legitimate large lists will hit `MaxWork` before anything malicious does, and I have to size the bound against my biggest response, not my threat model.
- **Regex dialect is "pending ruling".** Lookbehind and backreferences compile in the JS playground and fail (or, worse, differ in `\b`/`\d`/`$` semantics) in Go. Until this is ruled I cannot accept a config document that was validated in the playground.
- **`Error.Value` carries input-derived content; `Error()` does not.** `slog.Any("err", e)`, `%+v`, or any structured logger that walks the struct puts payload in the logs. Only `Error()` is scrubbed.
- **Package-level `Eval` uses `DefaultLimits`, not mine.** Someone will call it in a handler with a request-supplied expression, and it will run under the 256 KiB / 64 MiB defaults, not the cache's limits.
- **A nil typed map or slice is `{}`/`[]`, not null.** Go code that returns `var tags map[string]string` for "no tags" now satisfies `$exists(tags)`. Documented, and it will still surprise everyone.
- **`Prepare` fixes `$now`.** A long-lived `Evaluation` (say, cached per input for repeated selections) serves a stale clock forever.
- **`Encoder.Encode` writes directly to `w`.** On a foreign-value error you have already written a 200 and half a body. Buffer with `Marshal` in handlers; `NewEncoder` is for files and logs.

## Things the doc comments leave unclear

- What `Compile` returns on failure: an `*Error`? With `ClassSyntax`, `Line`, `Column`? The Errors section talks only about evaluation.
- What `Unmarshal` puts in `Error.Offset`, given `Offset` is defined as an offset into the *expression*.
- Whether `EvalJSON`/`Unmarshal` accept leading/trailing whitespace, a trailing newline, or an empty input; whether a top-level scalar is "one complete JSON value".
- Whether `Unmarshal` rejects a lone surrogate escape (`"\ud800"`) as invalid UTF-8 at decode time or on first read.
- `[]uint8` is `[]byte`. The doc admits `[]byte` as a string-like value and "any `[]T` whose T is admitted" (uint8 is admitted). Is `[]uint8{1,2,3}` the string `"AQID"` or the array `[1,2,3]`? (It must be bytes, but the text says both.) Is a nil `[]byte` the empty string or null?
- What `Marshal` prints for a carried `float32`: shortest round-trip of the float32, or of `float64(f)`?
- What `Code` a wrapped Resolver error carries, and whether `errors.Is(err, ErrUnsupportedValue)` is true for it.
- Whether a value that itself implements `ObjectView`/`ArrayView` can be passed as input or in `Bindings` without a Resolver, or whether views are only legal as Resolver output.
- Whether a foreign value in `Env.Bindings` is resolved through the Resolver on read, or is a `CodeBinding` error.
- Whether `Resolve` is called for a foreign value nested inside an admitted container (`[]any{msg}`, `map[string]any{"m": msg}`). (Presumably yes; not stated.)
- Whether `ObjectView.Get` and `ObjectView.Range` must agree on membership, and what the engine does if they do not (proto3 scalars at their default make this a real question).
- Whether bytes returned by a Resolver (strings, `[]byte`) are charged to `MaxBytes`, or only the call to `MaxWork`.
- Where `$error("text")` puts the text: `Message` or `Value`. The privacy rule on `Message` implies `Value`; the `ClassRaised` example prints `e.Value`, which supports that, but nothing says it.
- What `Reads()` means by "a binding makes known false": any `$x := ...` in the expression, or only a reference to an `Env.Bindings` name?
- Whether `Complete` returns the same value identities that earlier `Select` calls returned for those fields.
- Whether a `Resolver` that returns a `context` error from a context that is not `ctx` is rewritten to `ctx.Err()` (which may be nil).
- Whether `Limits` is guaranteed to remain comparable (plain ints) so it can key a map.
- Whether an `Encoder` is reusable across many `Encode` calls and whether it writes partial output on error.
- How `json.Number` is "classified once on first read" when a `json.Number` is a string with no identity; is it per read site, per value position in the input, or per evaluation?
- Rough memory footprint of a compiled `Expression` (regex programs are compiled in), which decides my cache size.

## On the concept

The regex framing is the right one, and it is right for exactly the reason integrators will resent it. Go's `regexp` is RE2, not PCRE; nobody who has ported a Perl regex to Go thinks the notation is shared in any way that matters to them at 2am. The value of the framing is that it puts the divergences where they can be named: in the value model and the dialect, not smeared through a transliterated interpreter. The cost is that "the notation is shared" is a statement about the documentation, and config authors do not read the documentation; they paste into try.jsonata.org until the output looks right, and the playground is the reference implementation with all the departures the README disowns. So the ledger in `DIVERGENCES.md` is necessary but not sufficient. What I need as an integrator is that ledger made executable: `Compile` telling me an expression touches `$round`, `$string` on a float, member order, or a regex feature outside the ruled dialect, so I can reject or flag a config document before it ships to a Go and a JS service and produces two answers. The README's "portable core" is the right observation; it should be a checkable property of an `Expression`, not a paragraph.

"The host defines what a number is" is the right call, and this package's version of it is unusually good: exact integers of any size, refuse rather than round on mixed arithmetic, comparison by value across representations. That is better than what I get from `encoding/json` into `any`, and better than the JavaScript reference. Where it goes wrong for a library that moves values between systems is not the arithmetic model; it is that the *representation* of a value at the Go boundary depends on where the value came from and whether it was merely carried. That is the correct semantics for the engine and the wrong contract for a consumer. I want two promises, stated as promises. First: an integer that enters as the JSON token `9007199254740993`, or as a protobuf `int64` field, and is only selected, copied, or rearranged, leaves `Marshal` as the token `9007199254740993`, and leaves `Eval` as a value from which `int64` can be recovered exactly, whatever its dynamic type. The package delivers the first half (rule 4, carriage) and leaves the second half to my type switch. Second: an integer that is *computed* from either source lands in one of exactly three types (`int64`, `*big.Int`, `float64`), which the package does promise, and it is the better half of the contract.

The remaining gap is cross-member, and the README puts it out of scope on purpose: "no parity between members" and "the project's own members aligning is a quality commitment of the project, not a property of the class." From the class's seat that is coherent. From mine it is the entire question. An OpenBindings transform in a config document does not know whether it will run under the Go evaluator or the JavaScript one; if the JS member's value model is IEEE doubles, then the ID that this package keeps exact is rounded over there by design, and the class has told me that is correct. Rule 4 saves carriage only if the JS binding kept the raw token before the evaluator ever saw it, which is a promise the binding makes, not the evaluator. So the concept is sound as a definition of an evaluator, and the thing I would want written down next to it, somewhere I can point a config author at, is the minimum every project member guarantees for the values that cross systems: carried integers exact through every member, computed integers exact to at least 64 bits, and a named list of the functions where the members are allowed to disagree.
