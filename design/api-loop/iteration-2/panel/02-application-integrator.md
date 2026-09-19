# Iteration 2 cold read: application integrator

> Given only the class README and go/jsonata at 4cb2f5d; told not to read design/. Verbatim.

## Grades

| Criterion | Grade | One-line justification |
|---|---|---|
| Idiomatic fit for Go | B+ | `Compile`/`MustCompile`/`String`, `ctx` first everywhere, `iter.Seq`, `errors.Is`/`As` with sentinels all feel native; the `(value, present, err)` triple, a `Close` on something that holds no OS resource, and `*Limits` with zero-means-default are each defensible but each one is a thing I have to explain to the next engineer. |
| Ergonomics of the common path | B- | Bytes in, bytes out via `EvalJSON` is genuinely one line; the native path hands me a result whose static type is "one of about twenty kinds, possibly containing my own protobuf messages", and ships no helper to collapse it. |
| Correctness and footgun risk | C+ | The aliasing contract (results alias `Env.Bindings`), the named-type admission rule (`json.RawMessage` becomes base64), `[]string` not being admitted, `encoding/json` silently disagreeing with `EvalJSON`, and selection ignoring sibling `$assert` are all week-one production bugs, and none is loud in the docs. |
| Performance headroom the API permits | A- | By reference, no copy on admission, lazy `Access`, shared compile, selective evaluation, `Reads()` for prefetching, budgets measured in work not time: this is the right shape. Deductions for limits being frozen at compile time and `Materialize` having no bound at all. |
| Concept soundness | B+ | "Evaluator as regex engine" is the right framing for a library that has to carry a 20-digit ID untouched; it is undersold as a risk for authors who test in the reference exerciser and deploy against the documentation. |
| Overall | B | I would ship it behind a wrapper of roughly 150 lines that the library should have shipped itself. |

## The integration I would write

### (a) HTTP handler, configured expression over a JSON body

```go
type Transform struct {
	expr *jsonata.Expression
	env  *jsonata.Env // shared, read-only, built once at startup
}

func (t *Transform) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20)) // I bound the body; nothing in Limits does
	if err != nil { http.Error(w, "body too large", http.StatusRequestEntityTooLarge); return }

	out, present, err := t.expr.EvalJSON(r.Context(), body, t.env)
	if err != nil { writeErr(w, err); return }
	if !present {
		w.WriteHeader(http.StatusNoContent) // GUESS: absent means "no body"; nothing tells me whether OpenBindings wants 204 or a literal null here
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(out) // GUESS: no trailing newline, no leading whitespace; doc says nothing about the shape of the bytes beyond escaping
}
```

The version I would actually end up with, because my framework already decoded the body into `map[string]any`:

```go
func (t *Transform) handleDecoded(w http.ResponseWriter, r *http.Request) {
	dec := json.NewDecoder(r.Body)
	dec.UseNumber() // without this, IDs are float64 before the evaluator ever sees them; the library cannot detect or repair that
	var in any
	if err := dec.Decode(&in); err != nil { http.Error(w, err.Error(), 400); return }

	v, present, err := t.expr.Eval(r.Context(), in, t.env)
	if err != nil { writeErr(w, err); return }
	if !present { w.WriteHeader(204); return }

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false) // to match what $string would have produced
	if err := enc.Encode(v); err != nil { // GUESS: every output kind has a sane encoding/json form
		// *big.Int and *Object have MarshalJSON, json.Number is a token, []byte is base64.Std ...
		// ... which is only correct if I never set Env.BytesEncoding to something else.
	}
}
```

### (b) The same over a protobuf message, with the `Access`

```go
type protoAccess struct{}

func (protoAccess) Resolve(ctx context.Context, v any) (any, error) {
	switch x := v.(type) {
	case *timestamppb.Timestamp:
		return x.AsTime().UTC().Format(time.RFC3339Nano), nil // my choice; the library has no opinion
	case *durationpb.Duration:
		return x.AsDuration().String(), nil
	case proto.Message:
		return msgView{x.ProtoReflect()}, nil
	case protoreflect.Message:
		return msgView{x}, nil
	case protoreflect.List:
		return listView{x}, nil
	case protoreflect.Map:
		return mapView{x}, nil
	}
	return nil, fmt.Errorf("jsonata: no access for %T", v)
	// GUESS: this error comes back to my handler wrapped so errors.As(*jsonata.Error) sees E1001,
	// or comes back raw so errors.As fails. The doc says neither. My writeErr has to handle both.
}

type msgView struct{ m protoreflect.Message }

func (mv msgView) Get(ctx context.Context, key string) (any, bool, error) {
	fds := mv.m.Descriptor().Fields()
	fd := fds.ByJSONName(key)
	if fd == nil { fd = fds.ByName(protoreflect.Name(key)) }
	if fd == nil { return nil, false, nil }
	if fd.HasPresence() && !mv.m.Has(fd) { return nil, false, nil } // absent, not null
	return fieldValue(fd, mv.m.Get(fd)), true, nil
}

func (mv msgView) Range(ctx context.Context, fn func(string, any) bool) error {
	mv.m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		return fn(fd.JSONName(), fieldValue(fd, v))
	})
	// GUESS: proto3 scalars at their zero value are skipped by m.Range, so $keys($) will omit them
	// while $.count (via Get) returns 0. I have to decide whether that inconsistency is acceptable
	// or enumerate every field myself. The doc gives no guidance on view consistency.
	return nil
}

func fieldValue(fd protoreflect.FieldDescriptor, v protoreflect.Value) any {
	switch {
	case fd.IsList(): return v.List() // foreign, resolved on read
	case fd.IsMap():  return v.Map()
	}
	switch fd.Kind() {
	case protoreflect.EnumKind:
		// MUST convert: protoreflect.EnumNumber is a named int32, which the admission rule
		// accepts silently as a number. Without this line every enum renders as an integer.
		return string(fd.Enum().Values().ByNumber(v.Enum()).Name())
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return v.Message().Interface() // foreign, resolved on read
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return v.Uint() // GUESS: a pure copy carries uint64; arithmetic widens to int64 or *big.Int
	case protoreflect.BytesKind:
		return v.Bytes() // []byte, base64 only when used as a string
	default:
		return v.Interface() // int32, int64, float32, float64, string, bool: all admitted
	}
}
```

The gRPC handler:

```go
func (s *svc) GetThing(ctx context.Context, req *pb.GetThingRequest) (*pb.ThingResponse, error) {
	v, present, err := s.expr.Eval(ctx, req, s.env) // s.env.Access = protoAccess{}
	if err != nil { return nil, grpcStatus(err) }
	if !present { return &pb.ThingResponse{}, nil }

	v, err = jsonata.Materialize(ctx, v, s.env.Access)
	// Mandatory, easy to forget: `{ "thing": $.thing }` returns an *Object whose member is a live
	// *pb.Thing. encoding/json would happily reflect over the generated struct's exported fields
	// and emit something plausible and wrong. There is no error to catch.
	// GUESS: Materialize turns an ObjectView into *Object and an ArrayView into []any.
	// GUESS: Materialize has no budget; a view over a 10^7-element list will allocate all of it.

	raw, err := json.Marshal(v)
	var resp pb.ThingResponse
	if err := protojson.Unmarshal(raw, &resp); err != nil { ... }
	// Two serializations to get a typed message back. There is no other route, and the int64
	// fields will arrive in the JSON as bare numbers, which protojson accepts but which is not
	// what protojson would have emitted.
	return &resp, nil
}
```

### (c) Selecting two fields of a large result

```go
ev, err := t.expr.Prepare(in, t.env)
if err != nil { return err } // only an invalid Env fails here
defer ev.Close()

if p := ev.Plan(); p.Mode == jsonata.ModeComplete {
	slog.Debug("selection unavailable", "reason", p.Reason) // I want this at startup, not per request; see changes
}

id, idOK, err := ev.Select(ctx, "id")
if err != nil { ... }
name, nameOK, err := ev.Select(ctx, "profile", "displayName")
// GUESS: "profile" is a constructor key, "displayName" is a structural lookup inside whatever
// "profile" evaluated to; I cannot select into an array element ("items", "0", "id") at all.

out := jsonata.NewObject(2)
if idOK   { out.Set("id", id) }
if nameOK { out.Set("displayName", name) }
enc.Encode(out) // GUESS: encoding a value that is still "shared with the Evaluation" before Close
                // is fine because encoding does not mutate. The doc only forbids mutation.
```

### (d) Errors and absence

```go
func httpStatus(err error) (code int, public string) {
	switch {
	case errors.Is(err, context.DeadlineExceeded): return 504, "transform timed out"
	case errors.Is(err, context.Canceled):         return 499, ""
	}
	var je *jsonata.Error
	if !errors.As(err, &je) {
		return 500, "" // GUESS: this branch is reached by errors my Access returned; or never, if the engine wraps them
	}
	switch je.Code.Class() {
	case jsonata.ClassEngine:
		if errors.Is(err, jsonata.ErrBudget) {
			return 422, "transform exceeded its budget" // GUESS at the status: I cannot tell whether the input was too big or the expression too greedy
		}
		return 500, "" // E1001 unsupported value is my bug, E1005 is theirs
	case jsonata.ClassRaised:
		return 400, fmt.Sprint(je.Value) // GUESS: je.Value is the $error argument; it may be input-derived so this could leak
	case jsonata.ClassType, jsonata.ClassEvaluation:
		return 502, "" // a T or D code is either a transform bug or an unexpected input shape; Class does not say which
	case jsonata.ClassSyntax:
		return 500, "" // only reachable via $eval after a successful Compile
	}
	return 500, ""
}
```

Absence is handled by the `present` flag; I will get it wrong once by treating `nil, true` and `nil, false` the same and sending `null` where the transform meant "no field".

### (e) Startup lifecycle and caching

```go
type exprKey struct {
	src string
	lim jsonata.Limits
}

var cache sync.Map // exprKey -> *jsonata.Expression; bounded LRU in reality

func compile(src string, lim *jsonata.Limits) (*jsonata.Expression, error) {
	l := jsonata.DefaultLimits()
	if lim != nil { /* overlay non-zero fields */ } // GUESS: normalizing so nil, &Limits{}, and &DefaultLimits() all hit one key
	k := exprKey{src, l}
	if e, ok := cache.Load(k); ok { return e.(*jsonata.Expression), nil }
	e, err := jsonata.Compile(src, &l)
	if err != nil { return nil, err }
	cache.Store(k, e)
	return e, nil
}

// Once, at startup:
env := &jsonata.Env{
	Bindings: map[string]any{"config": cfgSnapshot}, // cfgSnapshot must be frozen forever: results alias it
	Access:   protoAccess{},
}
// Validate the Env eagerly, because a "$"-prefixed binding name is only reported at evaluation time:
if ev, err := jsonata.MustCompile("$", nil).Prepare(nil, env); err != nil { // GUESS: Prepare validates binding names
	log.Fatal(err)
} else {
	ev.Close()
}
```

## What I would change

1. **Export the encoder that `EvalJSON` uses, and ship numeric collapse helpers.**
   `func Marshal(v any, env *Env) ([]byte, error)` and `func NewEncoder(w io.Writer, env *Env) *Encoder`, plus `func Int64(v any) (int64, bool)`, `func Float64(v any) (float64, bool)`, `func BigInt(v any) (*big.Int, bool)`.
   Today, `Eval` plus `encoding/json` disagrees with `EvalJSON` on HTML escaping and on `[]byte` when `BytesEncoding` is not the default, and there is no error for either. And a typed consumer of `$.id` must switch over `json.Number`, `int64`, `uint64`, `*big.Int`, `float64`, `int`, and any named type of those. The package already knows how to classify a number by value (it does it for `=`); expose that classifier.

2. **Fix admission at the two places every service will hit it.**
   (i) Treat `json.RawMessage` as JSON, classified on first use the way `json.Number` is, instead of as a named `[]byte` that renders as base64. Both types live in the package you already special-case. (ii) Admit homogeneous `[]T` and `map[string]T` where `T` is admitted (`[]string`, `[]int64`, `map[string]string`, `[]map[string]any`). That is a type switch, not reflection over structs, and it removes the most common `E1001` that only appears when an expression happens to touch that field in production. If you refuse (ii), ship `SliceView`/`MapView` adapters so the answer is one line, not a custom view.

3. **Specify how errors from `Access` and views come back, and bound `Materialize`.**
   Wording: "An error returned by Access or by a view is returned to the caller wrapped so that `errors.Is` and `errors.As` see the original; it is not an `*Error` unless it was one. A `ctx` error returned by Access is returned as `ctx.Err()`." And `func Materialize(ctx, v, a Access, limits *Limits) (any, error)`, charged to `MaxOutputNodes`/`MaxWork`/`MaxBytes` like an evaluation. Right now `Materialize` is the one call in the package that a hostile expression plus a large view can turn into an unbounded allocation, and it is the call the doc tells me to use before every encode.

4. **Make the plan and the read set available at compile time, and let me validate an `Env` without an input.**
   `func (e *Expression) Plan() Plan` reporting every input-independent reason (`ReasonDynamicShape`, `ReasonImpure`, `ReasonUnsupportedCall`, `ReasonPlanBudget`), with `ReasonArrayInput` left to `Evaluation.Plan`. And `func (e *Env) Validate() error`. I load transforms from configuration at startup; that is when I want to reject one that will never select, one that uses `$eval`, or a binding name that is illegal. Learning it on the first request is the wrong time.

5. **Let me turn `$eval` off, and give me a usage readout.**
   `Limits.DisallowEval bool` (or a `Compile` option) making `$eval` a `Compile`-time error. The environment is closed and budgeted, so `$eval` is not an escape, but for a document-supplied expression it defeats `Reads()` (`known` goes false), defeats selection (`ReasonImpure`), and turns a static expression I can review into one I cannot. Separately, `func (v *Evaluation) Usage() Usage` with work, bytes, and output nodes consumed, and the same as a fourth return on a variant of `Eval`, so I can set `Limits` from measurements and alarm at 80% instead of discovering `E1002` in an incident.

## Footguns

- **Results alias `Env.Bindings`.** One shared `Env` per service is what the doc recommends, and any result can hand a caller the live binding map by identity. The first downstream code that does `m["trace_id"] = ...` on a result is a data race, and if two requests hit it, a fatal concurrent map write. There is no `Clone`. My wrapper freezes bindings by convention and prays.
- **Numbers are lost upstream and the library cannot tell.** If the body was decoded without `UseNumber`, every ID is already a `float64`. The evaluator will carry it "exactly": exactly wrong. The doc explains its own numeric model in detail and never says "decode with `UseNumber` or `jsonata.Unmarshal`".
- **Named types are admitted silently.** `json.RawMessage` becomes base64. `time.Duration` becomes a nanosecond count. `protoreflect.EnumNumber` becomes an integer. Each is "correct" by the stated rule and wrong for the application.
- **`[]string`, `map[string]string`, `[]map[string]any` are not admitted**, and the refusal fires "when first used", so a unit test whose expression does not touch that field passes.
- **`encoding/json` over `Eval` output is a different serializer from `EvalJSON`.** HTML escaping on, `[]byte` always `base64.Std`, and a `json.Number` with an invalid token that was only ever copied (never "classified") survives to the encoder and fails there with `json: invalid number literal`.
- **Carried foreign values encode as garbage without error.** A protobuf message inside a result goes through `encoding/json` reflection over the generated struct and produces plausible JSON. `Materialize` is mandatory on the proto path and nothing enforces it.
- **Selection changes the meaning of the expression.** A sibling field's `$assert` is not a guard under `Select`. The same configured transform validates under `Eval` and does not under `Select`, and the only signal is a paragraph in the package doc.
- **Limits are frozen into the compiled expression.** `MaxWork` tuned for 1 KiB bodies trips on 1 MiB bodies with `E1002`, and I cannot distinguish "input too large" from "expression too greedy" to choose 413 vs 500. Different budgets mean different cache entries.
- **`MaxOutputNodes` may count a pure passthrough.** `$` over a 2M-node body is zero work and possibly a budget error. The doc says "counting every value" and does not say whether carried values count.
- **Budgets are work, not time; `ctx` is the only clock.** An `Access` that does I/O must honor `ctx` itself. `Prepare` and `Compile` take no `ctx`; compiling a 256 KiB untrusted expression per request is on me to prevent with a cache and a lower `MaxExpressionBytes`.
- **`Close` semantics for returned values.** A `Select` result is "shared with the Evaluation until Close", and a later `Complete` reuses it. If I pass a `Select` result to another goroutine that mutates it, then call `Complete`, the whole result is corrupt. Whether `Close` is idempotent, and whether a second `Close` panics, is unstated.
- **`Error.Value` carries input-derived content.** `Error()` is careful; `slog.Any("err", err)` or `%+v` on the struct is not.
- **`$eval` is on and cannot be turned off.** A document-supplied expression can build and run expressions from strings inside the budget; static analysis of the config is meaningless the moment it appears.
- **Regex literals are RE2.** Any JS lookaround or backreference an author tested in the exerciser fails at `Compile`. Good that it is early; make sure the message says "Go regexp does not support ..." and not just `S0302`.
- **`int` versus `int64` on output.** `$count(x)` yields `int64`; `$.n` over an input holding `int` yields `int`. Every type switch needs both arms, or the helpers from change 1.

## Things the doc comments leave unclear

- What happens to an error returned by `Access.Resolve`, `ObjectView.Get`/`Range`, or `ArrayView.Len`/`At`: wrapped into `*Error` (which code?), passed through, or both?
- If `Access` returns `ctx.Err()`, is it surfaced as `ctx.Err()` so `errors.Is(err, context.DeadlineExceeded)` holds?
- What `Materialize` produces for an `ObjectView` (`*Object`? `map[string]any`?) and an `ArrayView`, and whether it is bounded by anything.
- "The engine consults it once per foreign value": per identity, per encounter, or per Evaluation? For value-typed structs there is no stable identity.
- Whether carried (pure copy) values count against `MaxOutputNodes` and `MaxBytes`.
- Whether `Unmarshal`/`EvalJSON` input bytes are charged to any bound.
- Whether `Close` is idempotent, and whether `Close` while a concurrent `Select` is in flight is defined.
- Whether `Expression.Limits()` returns the resolved defaults or the zeros I passed (this decides my cache key).
- Whether `Prepare` rejects a bad binding name (the doc says "fails only for an invalid Env", which I am reading as yes).
- Whether a `Select` path can pass through an array (a key "applies to every element" for `Reads`, but `Select` says "member key only").
- The exact `Code` for a duplicate member name in `Unmarshal`/`EvalJSON`, and its `Class` (I need it to pick 400).
- Whether `EvalJSON` output ends with a newline and whether it can be streamed.
- How a proto3 zero-value scalar should present through a view: absent (`ok=false`) or present with its zero? The view contract talks about `ok`, not about what a good view does.
- Whether `Object.Set` on a value read from a result is a "mutation" of a shared value (it is, but nothing says the returned `*Object` may be the Evaluation's own instance versus a copy).
- Whether `$now` can be fixed by the caller for tests, and whether `$random` can be seeded. The environment section lists what is open; nothing lists what is injectable.
- What `ReasonUnsupportedCall` covers: which standard-library functions are "qualified as selectable"? Without the list I cannot write a transform that I know will select.
- Whether `Eval` on an `Expression` compiled with one `Limits` and evaluated with an `Env` shared by expressions compiled under another `Limits` has any interaction (I assume none).
- What `Value` holds for a `T` or `D` error (the offending operand? nothing?) and whether it can be a foreign value I need to `Materialize` before logging.

## On the concept

The regex-engine framing is sound from where I sit, and the reason is not the elegance of the analogy but a specific production property it buys: values I hand in come back with the same bytes and the same digits, and the engine never routes them through a JavaScript number or a JSON text to do it. Every JSONata port I have used in a service has, somewhere, `float64` as the only number, and the 20-digit ID problem surfaces as a support ticket six weeks after launch. An evaluator that declares "carriage is exact, comparison is by value, refusal over approximation" as membership rules is the first one I could put in front of a payments API without a pre-pass that stringifies every ID. The place the analogy undersells the cost is the authority question. Nobody tests regexes against a "reference implementation"; everybody tests JSONata expressions in the official exerciser, which is the JavaScript engine. "Documentation as authority" means "works in the exerciser, differs in prod" is a defined outcome. The portable-core observation and the pinned divergence ledger are the right mitigation; I would want the ledger machine-readable and a `Compile` mode that flags an expression touching a declared divergence, so the difference is visible at config-load time rather than in a diff of two responses.

"The host defines what a number is" is the right call, with one qualification an integrator will insist on. The right call: a library moving values between systems should not invent a number model that neither the source nor the sink uses. It should carry what it was given and compute by value. The qualification: at my typed boundary I do not see "a number", I see a Go type, and the same logical `int64` ID reaches me as `json.Number` from a JSON body, as `int64` from a protobuf field, as `uint64` from a `fixed64` field, and as `*big.Int` the moment an expression added zero to it. Inside the expression the doc promises all of those are one value, and I believe it. Outside the expression the package offers no single form to normalize to. That is why change 1 is ranked first: the value model can stay host-native and still hand me one function that says "this is an integer that fits int64, here it is".

What I would want the package to promise about an ID, stated in the doc in exactly these terms: an integral JSON token of any length arrives as `json.Number`, is compared and sorted by its mathematical value against every other representation, and leaves as the identical `json.Number` under pure carriage, as `int64` if any arithmetic touched it and the result fits, and as `*big.Int` otherwise; `$string` of any of those renders the same decimal digits. The current text says all of that, scattered across four sections, and I had to assemble it. For a protobuf `int64` field the promise is the same, and the difference that bites is not inside the evaluator at all: protojson would have emitted that field as a JSON string, and my `Access` handed the engine an `int64`, so the engine will faithfully produce a bare JSON number that a browser will round. The README is right that this is the binding's decision, made before the value reaches the evaluator. The package doc should say so in one sentence in the Access section, because the person writing the `Access` is the one making the decision and does not know it.
