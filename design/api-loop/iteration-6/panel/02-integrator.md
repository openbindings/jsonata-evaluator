# Iteration-6 panel: application integrator

Cold read of `81e3693`. Lens: the developer integrating this into a production Go service (HTTP and gRPC handlers, untrusted configuration expressions).

## Grades

| Criterion | Grade | One-line justification |
|---|---|---|
| Idiomatic fit for Go | B+ | `Limits` as a `net.Dialer`-style receiver, sentinels plus `errors.As`, `iter.Seq`, `io.Writer` encoder, and `MustCompile` all read as Go; the `(value, present, err)` triple everywhere, the variadic `Select` path, and bounds baked into the compiled expression do not. |
| Ergonomics of the common path | B | `EvalJSON` is a one-liner for the HTTP case; everything after that (a result that may be `map[string]any` or `*Object` or `[]any` or a carried `[]string`, an `Object.MarshalJSON` that cannot see my `Env`, three-value returns on every call) costs integration code the docs do not show. |
| Correctness and footgun risk | B- | The number model is careful and the aliasing rules are stated, but `Select(ctx, "id", "name")` means `id.name`, `Object.MarshalJSON` silently uses a nil `Env`, Resolver panics kill a gRPC process, and `CodeInexact` is a runtime failure that only certain payloads trigger. |
| Performance headroom the API permits | A- | Compile once, values by reference, no serialization, lazy views, `Reads()` for fetch pruning, selective evaluation, no goroutines; docked for per-tenant budgets needing a recompile, non-memoized `Resolve`, and `json.Number` reparsed on every read. |
| Concept soundness | B+ | Exact carriage and value-based comparison are exactly what a payload transformer needs; the framing is weakest precisely where the regex analogy has no equivalent (an open set of host value types on the way in and a type zoo on the way out), and the authoring tool everyone uses is the reference engine the class deliberately diverges from. |
| Overall | B | I could ship it behind an HTTP handler tomorrow with about 80 lines of wrapper and a sharp checklist; the gRPC path needs a week and a test suite of my own. |

## The integration I would write

### (a) HTTP handler over a decoded JSON body

```go
type Transform struct {
	expr *jsonata.Expression
	env  *jsonata.Env // shared, immutable for process life
}

func (t *Transform) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
	if err != nil {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return
	}
	// EvalJSON decodes under expr.Limits(); my MaxBytesReader is belt and braces.
	out, present, err := t.expr.EvalJSON(r.Context(), body, t.env)
	if err != nil {
		writeTransformError(w, err)
		return
	}
	if !present {
		w.WriteHeader(http.StatusNoContent) // GUESS: absent has no JSON spelling; I chose 204. Nothing tells me whether transform authors expect absent to be "no body" or "null".
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
}
```

The envelope case, which is the one my real services actually have:

```go
type envelope struct {
	Data json.RawMessage `json:"data"`
	Meta meta            `json:"meta"`
}

// Data MUST be pre-marshaled with jsonata.Marshal(out, t.env). Putting `out`
// straight into an `any` field routes an *Object through Object.MarshalJSON,
// which uses a nil Env, so any []byte inside fails (E1001) and any carried
// foreign value or view is not handled at all.
b, err := jsonata.Marshal(out, t.env) // GUESS: package-level Marshal runs under DefaultLimits, not expr.Limits(); I assume that is harmless for a result the expression already bounded.
resp := envelope{Data: b, Meta: m}
json.NewEncoder(w).Encode(resp) // encoding/json re-escapes < > & inside Data unless SetEscapeHTML(false); jsonata.Marshal deliberately did not.
```

### (b) gRPC handler over a protobuf message, with the Resolver

```go
func (s *server) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.GetUserResponse, error) {
	u, err := s.store.Get(ctx, req.Id)
	if err != nil { return nil, err }

	out, present, err := s.expr.Eval(ctx, u, s.env) // root is foreign: resolved via s.env.Resolver
	if err != nil { return nil, toStatus(err) }
	if !present { return &pb.GetUserResponse{}, nil }

	// Hand to typed code. `out` may still contain a protoreflect.Message that
	// the expression carried unread, so I must Materialize before walking it.
	v, err := jsonata.Materialize(ctx, out, s.env) // GUESS: Materialize turns my views back into *Object; nothing says Marshal alone would read through a view, so I always Materialize first on this path.
	if err != nil { return nil, toStatus(err) }

	idv, _ := jsonata.Member(v, "id")
	id, ok := jsonata.Int64(idv) // GUESS: a carried int64 from protoreflect.Value.Int() comes back as int64; Int64 accepts it.
	if !ok { return nil, status.Error(codes.Internal, "transform produced a non-integer id") }
	...
}
```

The Resolver. This is the part the doc comments hand to me with "the Resolver author decides", and every decision below is a guess at what the engine wants:

```go
// safeResolver exists because a panic inside Resolve propagates, and a gRPC
// server without a recovery interceptor dies. protoreflect panics on a
// descriptor mismatch. I will not ship without this wrapper.
type safeResolver struct{ inner jsonata.Resolver }

func (r safeResolver) Resolve(ctx context.Context, v any) (out any, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("resolver panic: %v", p)
		}
	}()
	return r.inner.Resolve(ctx, v)
}

type protoResolver struct{}

func (protoResolver) Resolve(ctx context.Context, v any) (any, error) {
	switch x := v.(type) {
	case *timestamppb.Timestamp: // before proto.Message: well-known types get protojson's presentation
		return x.AsTime().UTC().Format(time.RFC3339Nano), nil
	case *durationpb.Duration:
		return x.AsDuration().String(), nil // GUESS: protojson renders "1.5s"; I approximate
	case proto.Message:
		return messageView{x.ProtoReflect()}, nil
	case protoreflect.Message:
		return messageView{x}, nil
	}
	return nil, fmt.Errorf("protoResolver: %T", v) // surfaces as E1007 with this wrapped
}

type messageView struct{ m protoreflect.Message } // pointer-shaped: one interface field. GUESS: "pointer-shaped (one pointer field)" includes an interface field; if not, every read allocates.

func (mv messageView) Get(ctx context.Context, key string) (any, bool, error) {
	fd := mv.m.Descriptor().Fields().ByJSONName(key) // GUESS: JSON names, so transforms written against protojson output work. The doc says this is my call; it gives no steer.
	if fd == nil {
		return nil, false, nil
	}
	if fd.HasPresence() && !mv.m.Has(fd) {
		return nil, false, nil // GUESS: unset optional/message field is absent, not null, so $exists and ?? behave like the JSON body path where the key is simply missing.
	}
	return fieldValue(mv.m, fd), true, nil // proto3 scalars without presence report their default, matching protojson with EmitUnpopulated
}

func (mv messageView) Range(ctx context.Context, fn func(string, any) bool) error {
	// NOT protoreflect.Message.Range: that visits populated fields only, and
	// Get above answers ok=true for unpopulated non-presence scalars. The doc
	// says "Get and Range must agree on membership; the engine trusts both",
	// so I walk the descriptors in field-number order instead.
	fds := mv.m.Descriptor().Fields()
	for i := 0; i < fds.Len(); i++ {
		fd := fds.Get(i)
		if fd.HasPresence() && !mv.m.Has(fd) {
			continue
		}
		if !fn(fd.JSONName(), fieldValue(mv.m, fd)) { // GUESS: fn returning false means stop. The doc never says.
			return nil
		}
	}
	return nil
}

func fieldValue(m protoreflect.Message, fd protoreflect.FieldDescriptor) any {
	switch {
	case fd.IsList():
		return listView{m.Get(fd).List(), fd}
	case fd.IsMap():
		return mapView{m.Get(fd).Map(), fd} // keys stringified; int64 keys become decimal strings
	}
	return scalar(m.Get(fd), fd)
}

func scalar(v protoreflect.Value, fd protoreflect.FieldDescriptor) any {
	switch fd.Kind() {
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind,
		protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return v.Int() // int64, exact. NOT protojson's string form; see the concept section.
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind, protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return v.Uint()
	case protoreflect.DoubleKind:
		return v.Float()
	case protoreflect.FloatKind:
		// A carried float32(0.1) marshals as 0.10000000149011612 (doc says so).
		// protojson and encoding/json both print 0.1. I have to launder it:
		f, _ := strconv.ParseFloat(strconv.FormatFloat(v.Float(), 'g', -1, 32), 64)
		return f // GUESS: this is what "the binding decides how a value presents" is supposed to cover. It feels like working around the library.
	case protoreflect.BoolKind:
		return v.Bool()
	case protoreflect.StringKind:
		return v.String()
	case protoreflect.BytesKind:
		return v.Bytes() // []byte; env.BytesEncoding = base64.StdEncoding gives protojson's presentation
	case protoreflect.EnumKind:
		if ev := fd.Enum().Values().ByNumber(v.Enum()); ev != nil {
			return string(ev.Name()) // GUESS: names, matching protojson; unknown numbers fall through to the number
		}
		return int64(v.Enum())
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return v.Message() // foreign; the engine calls Resolve again when it reads it. GUESS: returning messageView directly would skip a Resolve call per read, but then a carried value comes back to me as a view instead of a message, and I do not know which the engine prefers.
	}
	return nil
}

type listView struct {
	l  protoreflect.List
	fd protoreflect.FieldDescriptor
}

func (lv listView) Len(ctx context.Context) (int, error)       { return lv.l.Len(), nil }
func (lv listView) At(ctx context.Context, i int) (any, error) { return scalar(lv.l.Get(i), lv.fd), nil }
```

For small messages I would honestly skip all of this and write a 60-line eager `proto -> *Object` converter, evaluate over admitted values only, and never touch `Materialize`. The view machinery pays off only when messages are large and selection or `Reads()` prunes them.

### (c) Selecting two fields of a large result

```go
// At startup, against configuration, before any input exists:
keys, ok := expr.Fields()
if !ok { return fmt.Errorf("transform %q is not selectable", name) } // GUESS: whether I should reject or just accept whole-evaluation fallback is a product decision the doc leaves to me; I reject because the point of configuring selections is cost.
for _, want := range cfg.Selections { if !slices.Contains(keys, want) { return fmt.Errorf(...) } }

// Per request:
ev, err := expr.Prepare(ctx, in, env)
if err != nil { return err }
defer ev.Close()

if !ev.Plan().Selective() {
	metrics.wholeEval.WithLabelValues(ev.Plan().Reason.String()).Inc() // ReasonArrayInput can only be known here
}

result := jsonata.NewObject(len(cfg.Selections))
for _, k := range cfg.Selections {
	v, present, err := ev.Select(ctx, k) // ONE key per call. ev.Select(ctx, "id", "name") is the path id.name, not two fields.
	if err != nil { return err }          // GUESS: a language error in "id" leaves "name" selectable, so I could keep going; I do not, because a partial object is worse than an error for my callers.
	if present {
		result.Set(k, v) // v is shared with ev until Close; I only read it.
	}
}
b, err := jsonata.Marshal(result, env) // GUESS: marshaling values still shared with the Evaluation is a read and therefore allowed before Close. The doc only forbids modification.
```

### (d) Errors and absence

```go
func toHTTP(err error) (int, string) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout, "timeout"
	}
	var e *jsonata.Error
	if !errors.As(err, &e) {
		return 500, "internal" // GUESS: doc says every failure is *Error or ctx.Err(); I still guard.
	}
	// Log Code, Message, Offset, Line, Column, Token. Never e.Value, never
	// slog.Any("err", e), never %+v: Value can hold payload content.
	switch e.Code {
	case jsonata.CodeMalformedInput:
		return 400, "malformed JSON"
	case jsonata.CodeBudget:
		return 413, "too expensive" // GUESS: I cannot tell whether the budget blew because the input was big (client's fault) or the expression loops (config's fault).
	case jsonata.CodeInexact:
		return 500, "transform cannot represent this value" // this is a config bug that only some payloads reveal
	case jsonata.CodeResolver, jsonata.CodeUnsupportedValue, jsonata.CodeInternal, jsonata.CodeBinding, jsonata.CodeClosed:
		return 500, "internal"
	}
	switch e.Code.Class() {
	case jsonata.ClassRaised:
		return 422, "rejected" // GUESS: e.Value is the $error payload; I have no way to know whether the transform author intended it to be client-visible.
	case jsonata.ClassType, jsonata.ClassEvaluation:
		return 500, "transform failed" // GUESS: "a" + 1 could be an unexpected input shape (400) or an expression bug (500); the error cannot tell me which.
	}
	return 500, "internal"
}
```

### (e) Startup, caching, and lifecycle

```go
type Registry struct {
	limits jsonata.Limits
	env    *jsonata.Env
	exprs  map[string]*jsonata.Expression // keyed by config name; source+limits are fixed per process
}

func NewRegistry(cfg Config) (*Registry, error) {
	r := &Registry{
		limits: jsonata.Limits{MaxWork: 1 << 20, MaxBytes: 8 << 20, MaxOutputNodes: 1 << 16}, // GUESS: I have no idea what 8M work units cost in wall time, so these are numbers I will tune after a load test.
		env: &jsonata.Env{
			Bindings: map[string]any{
				"eval": nil, // disables $eval for configuration I do not fully trust; the doc says this idiom is supported
				"service": "users", // immutable for process life, and readable by every expression
			},
			Resolver:      safeResolver{protoResolver{}},
			BytesEncoding: base64.StdEncoding,
		},
		exprs: map[string]*jsonata.Expression{},
	}
	for name, src := range cfg.Transforms {
		expr, err := r.limits.Compile(src) // never MustCompile on config
		if err != nil {
			var e *jsonata.Error
			errors.As(err, &e)
			return nil, fmt.Errorf("transform %q: %s at %d:%d near %q", name, e.Message, e.Line, e.Column, e.Token)
		}
		r.exprs[name] = expr
	}
	return r, nil
}

// Per-request bindings (a $caller variable, say) need a NEW Env per request:
// Env has no parent, so I copy the base map every time.
func (r *Registry) envFor(caller string) *jsonata.Env {
	b := maps.Clone(r.env.Bindings)
	b["caller"] = caller
	e := *r.env
	e.Bindings = b
	return &e
}
// Per-tenant budgets are not possible without recompiling: Limits live in the
// Expression. GUESS: I assume Compile is cheap enough to keep a (source, Limits) cache per tenant tier; nothing tells me the cost.
```

Nothing to close on the `Eval` path. On the `Prepare` path, `defer ev.Close()`; the doc says GC reclaims an unclosed one, so a forgotten Close is a leak of the sharing window, not of memory.

## What I would change

1. **Fix `Select`'s signature so the obvious call means the obvious thing, and add the multi-field call that is the actual use case.**
   `func (v *Evaluation) Select(ctx context.Context, key string) (any, bool, error)` for one top-level field; `func (v *Evaluation) SelectPath(ctx context.Context, path []string) (any, bool, error)` for descent; and `func (v *Evaluation) SelectFields(ctx context.Context, keys ...string) (*Object, error)` returning an insertion-ordered object holding the present requested fields. Today `ev.Select(ctx, "id", "name")` compiles, returns absent, and looks correct in review. Sparse fieldsets are the reason selective evaluation exists, and the library makes me build the object myself in a loop.

2. **Split parse-time bounds from run-time budget so one compiled expression can be evaluated under different budgets.**
   Keep `MaxExpressionBytes` and `MaxDepth` on `Compile`. Move `MaxRecursion`, `MaxOutputNodes`, `MaxWork`, `MaxBytes`, `MaxIntegerBits` to a `Budget` struct on `Env` (zero means the expression's compiled defaults), or add `func (e *Expression) WithLimits(l Limits) *Expression` sharing the parse. The doc's safety argument (bounds travel with the expression) is real, but `Env` already carries per-call authority (bindings, resolver, clock); a budget belongs there. A multi-tenant service today has to compile every transform once per tier.

3. **Make `Marshal` render `float32` with 32-bit shortest digits, as `encoding/json` and `protojson` do.**
   `float32(0.1)` printing as `0.10000000149011612` is exact and defensible in float64 space, and it is a rendering nobody downstream expects; every proto `float` field would need laundering in the Resolver (see (b)). Wording: "a float32 as the shortest digits that round-trip at 32 bits, as encoding/json renders it." If `$string` must stay consistent with the widened value, say so and accept the asymmetry; the encoder is the boundary that matters to me.

4. **Give budget exhaustion and input rejection their own classes, or add a fault dimension to `Error`.**
   `ClassEngine` today holds the one code I map to 400 (`CodeMalformedInput`), the one I map to 413 (`CodeBudget`), and five I map to 500. I switch on `Code` and ignore `Class`, which means `Class` is not earning its keep for the integrator. Either `ClassInput` and `ClassBudget` as siblings, or `func (e *Error) Fault() Fault` with `FaultExpression`, `FaultInput`, `FaultEnvironment`, `FaultEngine`, `FaultUnknown` (T and D codes would be `FaultUnknown`, which is honest).

5. **Recover Resolver and view panics into `CodeResolver` instead of propagating.**
   The engine already has the recovery frame (it turns its own panics into `CodeInternal`). A gRPC server with no recovery interceptor dies on the first descriptor mismatch. Wording: "A panic in a Resolver, view, BytesEncoding, or Now is recovered into an *Error of CodeResolver whose Unwrap is a *PanicError carrying the value." The bug is still surfaced; the process survives. If the authors hold the line on propagation (the `sort.Interface` convention), the doc should say in the Resolver comment, in so many words, "wrap your Resolver in recover if your server has no panic handler."

## Footguns

- `ev.Select(ctx, "id", "name")` is the path `id.name`. Returns absent, no error, passes review.
- `Object.MarshalJSON` uses a nil `Env`. Embedding a result in a response struct encoded by `encoding/json` fails the moment the result contains a `[]byte`, and there is no way to thread the `Env` through. Always `jsonata.Marshal` first and embed as `json.RawMessage`.
- A carried `float32` marshals as `0.10000000149011612`. Every protobuf `float` field, every `[]float32` you hand in.
- `CodeInexact` is a runtime error that only large values trigger. `ts_ns / 1e9` works, `ts_ns * 1e-9` fails, and only for timestamps past 2^53 nanoseconds, which is every nanosecond timestamp since 1970 plus 104 days. Your staging fixtures with small numbers will not catch it. The doc's own worked example is a nanosecond timestamp, so the authors know this is the realistic case.
- Resolver, view, `BytesEncoding`, and `Now` panics propagate. `net/http` recovers per request; `grpc-go` does not by default.
- `$` returns the input by identity, `{ "a": $.a }` returns a new `*Object` whose `a` is the input's `a` by identity. Mutating a result before you have `Clone`d it mutates the decoded request body, or worse, a value in the shared `Env.Bindings`. There is no runtime check.
- `Unmarshal` rejects duplicate member names and invalid UTF-8 that `encoding/json` accepts. If the rest of the service decodes the same body with `encoding/json`, the same request succeeds in one handler and 400s in the transform handler.
- `Encoder.Encode` can write a partial value on error. Into an `http.ResponseWriter` that is a 200 with a truncated body.
- `MaxBytes` is cumulative with nothing credited back. A transform that builds and discards strings in a loop trips 64 MiB while the process uses a few megabytes. The error says budget; the metric says the box is idle.
- `jsonata.Eval` and `jsonata.Limits.Eval` compile every call. Someone will put one in a handler. Name it `EvalOnce` or `CompileAndEval` and this goes away.
- `expr.Limits()` bounds `EvalMarshal`; the package-level `Marshal` bounds under `DefaultLimits`. Compile tight, marshal loose, nobody writes `expr.Limits().Marshal(out, env)`.
- `Env.Bindings` with a nil value is null, not absent, so `$caller ?? "anonymous"` yields null when you set `Bindings["caller"] = nil` for an unauthenticated request. Delete the key instead.
- A `json.RawMessage` that is nil or empty is `CodeMalformedInput` on read. An unset `json.RawMessage` field in a struct you converted to a map is a latent 500.
- `Unmarshal` results share the decoded buffer: cache one small string from a 4 MiB body and you have pinned 4 MiB.
- Sequence rules apply to results: `items.name` over one item is a `string`, over two is `[]any`. Typed Go code expecting a slice must be given `items.name[]`. Documented, and still the first bug every integrator files.

## Things the doc comments leave unclear

- What `fn`'s `bool` return means in `ObjectView.Range`. I assumed false stops.
- Whether `Marshal` reads through an `ObjectView`/`ArrayView` or whether only `Materialize` does. Views are listed as admitted, and `Marshal` says foreign values are errors, and never mentions views at all.
- What "Resolve must return an equal view for the same value" means. `==` on the interface? `reflect.DeepEqual`? Something the engine checks, or an obligation it merely relies on?
- Whether "pointer-shaped (one pointer field)" covers a struct with one interface field, which is what every protoreflect-backed view will be.
- Whether returning a view directly from `ObjectView.Get` (rather than the foreign value) is preferred, permitted, or changes what a carried value looks like on the way out.
- Whether a view returned from `Get` on one read and the foreign value returned on another are considered "equal views."
- What one unit of `MaxWork` costs in wall time on ordinary hardware, even roughly. I cannot set a `MaxWork` or a request deadline without it.
- How expensive `Compile` is, and the memory footprint of an `*Expression`, for sizing a per-tenant cache.
- Whether `Prepare` followed by `Complete` costs materially more than `Eval`.
- Whether `Encoder` is safe for concurrent use. Assume no.
- Whether `Marshal` renders `json.RawMessage` (decode and re-encode? verbatim?) and `map[string]json.RawMessage`. Both are admitted; `Marshal`'s list omits them.
- Whether `Marshal` renders `int`, `uint64`, `int8`, etc. as digits. Implied, never stated.
- Whether `Reads()` includes paths read through `$$` at top level (not inside a function). The comment only excludes `$$` inside a function.
- What `Error.Value` holds for `CodeBudget` and `CodeInexact` (the offending operand? nil?). "Errors that carry one" is not enumerated.
- Whether `$error`'s `Value` can be shown to a client, i.e., whether the doc's expectation is that transform authors write client-safe messages. Not the library's call, but a sentence would help.
- Whether `Eval` with an already-cancelled ctx returns immediately with `ctx.Err()` or does some work first.
- Whether `Select` on a non-selective plan, where the whole result is not an object, is an error or absent. The doc says absent; whether that also holds for `Complete` returning a non-object under a plan that claimed `Fields` is not stated.
- Whether `Expression.Fields` order is guaranteed stable across `Compile` calls of the same source (for caching validated selections).
- What `Error()` looks like, even as an example, so I can write a log-parsing test.
- Whether a Resolver is called for the root with the `Prepare` ctx or with each `Select`'s ctx when the root is foreign and a `Select` descends into it.
- How `$type` reports a view ("object"/"array" presumably) and a foreign value with no Resolver (error, presumably, but "on read" and `$type` is a read).

## On the concept

The regex framing is right about the half it names and silent about the half that generates the API surface. `regexp` succeeds because the host has one string type; the notation never has to ask what a string is. A JSON evaluator over host values has to ask what a value is at every read, and Go's answer is a list of eighteen admitted types, a foreign category, a `Resolver` door, two view interfaces, a materialization step, and an insertion-ordered `*Object` that competes with `map[string]any` for the title of "an object." None of that has a regex analogue, and all of it is where my integration time goes. The exactness promise (carriage exact, comparison by value, refuse rather than approximate) is the part I actually need for OpenBindings, and I would take it over any port. But the honest description is "a JSONata engine over a defined Go value model with a resolution boundary," not "regexp for JSON," and the docs should stop leaning on the analogy the moment they start explaining `Resolver`.

"The host defines what a number is" is the correct call for a library that moves values between systems, provided the host's definition is exact and the library tells me which representation I get back. It does: an int64 from a JSON body decoded with `Unmarshal` and an int64 from a protobuf field both arrive as `int64`, compare equal, satisfy `jsonata.Int64`, and encode as the same digits. That is the promise I want and it is stated. What is not stated, and what I want in the package comment, is the promise's edges: that `id` and `id + 0` encode identically; that an int64 the transform never reads is emitted byte-for-byte as it was decoded; and that the one path the library cannot protect is a body decoded by `encoding/json` into `any` before it arrived, which is the path every existing Go service is already on. The protobuf side has an asymmetry the doc leaves to me: `protojson` renders int64 as a JSON string precisely because JavaScript consumers cannot hold it, and this library will render the same field as digits. That is a binding decision, and the README says so, but it is the one an integrator will get wrong first, and a sentence in the `Resolver` comment naming it would cost nothing.

Documentation-as-authority is sound as a spec stance and hazardous as an operational one, for a reason the README does not confront: the JSONata Exerciser, the tool every transform author will use to write and test an expression, is the reference implementation. An author who sees `"0.3"` there ships an expression and gets `"0.30000000000000004"` here; an author who sorts an object with numeric keys gets a different order. The "portable core" paragraph is the real mitigation and it is presented as an observation rather than a tool. From the integrator's seat the thing I would want next is not more divergence entries; it is a way to lint a configured expression against that core so the divergences are a compile-time warning in my config validator rather than a production diff. If the class is serious about the documentation being the authority, then a conformance lint is how it makes the authority usable by people who will never read the ledger.
