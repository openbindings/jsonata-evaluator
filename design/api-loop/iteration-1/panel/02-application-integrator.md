# Iteration 1 cold read: application integrator

> Given only the class README and go/jsonata + go/text at feb3743; told not to read design/. Verbatim.

## Grades

| Criterion | Grade | One-line justification |
|---|---|---|
| Idiomatic fit for Go | B+ | `Compile`/`MustCompile`/`String()` mirroring `regexp`, ctx-first, `errors.As`, `iter.Seq` are all right; the `(value, present, err)` triple is unusual but earned; `CompileOption` as an uncomparable func while the docs tell me to cache by "the compile options" is not. |
| Ergonomics of the common path | B- | JSON-body path is three lines if you already know to `UseNumber()`; the protobuf path makes me write a nine-method `Access` with no reference implementation and then hand-write the result materializer the library forgot. |
| Correctness and footgun risk | C+ | Foreign values can come back in results, memoized selections are "shared" and "owned by the caller" at once, named types (`type ID int64`) are refused at first use, nil slices are null, the default decoder silently destroys the very IDs the project cares about before the evaluator sees them. |
| Performance headroom the API permits | A- | Compile once, evaluate by reference, lazy admission, memoized selective evaluation, `Reads()` for lazy fetch, all budgets finite; nothing in the surface forces a copy or a serialization. |
| Concept soundness | B+ | The regex framing is the right mental model for safety and bounds; "host defines the number" is fine because the package then makes exact, refusal-not-rounding promises; the analogy hides the part that hurts integrators, which is values crossing a system boundary, and the Access/materialization gap is exactly where it leaks. |
| Overall | B | I could ship the HTTP path tomorrow; the gRPC path and the error mapping would cost me a day of guessing and a helper package the library should have provided. |

## The integration I would write

### (e) Startup: compile, bound, cache

```go
type transformBounds struct {
    MaxDepth, MaxRecursion, MaxOutputNodes, MaxWork int
}

var untrusted = transformBounds{MaxDepth: 64, MaxRecursion: 256, MaxOutputNodes: 1 << 16, MaxWork: 1 << 20}

// GUESS: the doc says cache "keyed by String() and the compile options", but CompileOption is a
// func and cannot be a map key, so the key is my own struct of bound values.
type exprKey struct {
    src    string
    bounds transformBounds
}

var exprCache = lru.New[exprKey, *jsonata.Expression](1024) // GUESS: size of a *Expression is unknown, so I bound by count

func compileConfigured(src string, b transformBounds) (*jsonata.Expression, error) {
    k := exprKey{src, b}
    if e, ok := exprCache.Get(k); ok {
        return e, nil
    }
    e, err := jsonata.Compile(src,
        jsonata.WithMaxDepth(b.MaxDepth),
        jsonata.WithMaxRecursion(b.MaxRecursion),
        jsonata.WithMaxOutputNodes(b.MaxOutputNodes),
        jsonata.WithMaxWork(b.MaxWork),
        // WithMaxExpressionBytes left at default; config loader already caps file size
    )
    if err != nil {
        return nil, fmt.Errorf("transform %q: %w", truncate(src), err) // config error, fail at boot
    }
    exprCache.Add(k, e)
    return e, nil
}

// One resolved expression per gRPC service, Access fixed for the process lifetime.
var protoExpr = mustLoad().With(jsonata.WithAccess(protoAccess{}))
// GUESS: With returns a cheap view sharing the compiled tree, not a recompile.
```

### (a) HTTP handler over a decoded JSON body

```go
func (h *transformHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 250*time.Millisecond)
    defer cancel()

    dec := json.NewDecoder(io.LimitReader(r.Body, 4<<20))
    dec.UseNumber() // GUESS: without this, 9007199254740993 is float64 9007199254740992 before the library ever sees it; nothing in the package doc warns me
    var body any
    if err := dec.Decode(&body); err != nil {
        http.Error(w, "malformed body", http.StatusBadRequest)
        return
    }

    out, present, err := h.expr.Eval(ctx, body, jsonata.WithBindings(map[string]any{
        "requestId": r.Header.Get("X-Request-Id"),
    }))
    if err != nil {
        status, msg := classifyEvalError(err)
        http.Error(w, msg, status)
        return
    }
    if !present {
        w.WriteHeader(http.StatusNoContent) // GUESS: absent top-level result; the library is silent and I picked 204 over writing "null"
        return
    }
    w.Header().Set("Content-Type", "application/json")
    enc := json.NewEncoder(w)
    enc.SetEscapeHTML(false)
    if err := enc.Encode(out); err != nil { // GUESS: int64, float64, *big.Int, json.Number, []byte, []any, map[string]any and *Object all encode as I expect; *big.Int as a bare number, []byte as base64.Std regardless of WithBytesEncoding
        log.Error("encode", "err", err)
    }
}
```

Or, if I do not need bindings from the request, `text.Evaluate(ctx, h.expr, rawBody)` replaces the decode and encode; I would still have to decide what to write on `present == false`, and I lose `SetEscapeHTML(false)`.

### (b) gRPC handler over a protobuf message

```go
type protoAccess struct{}

func (protoAccess) Kind(v any) jsonata.Kind {
    switch x := v.(type) {
    case proto.Message:
        if x == nil || !x.ProtoReflect().IsValid() {
            return jsonata.KindNull // GUESS: an unset message field is null, not absent; nothing says which the binding wants
        }
        return jsonata.KindObject
    case protoreflect.List:
        return jsonata.KindArray
    case protoreflect.Map:
        return jsonata.KindObject
    case protoreflect.EnumNumber:
        return jsonata.KindString // GUESS: enums as names (protojson style); could equally be numbers
    }
    return jsonata.KindUnknown
}

func (protoAccess) Get(v any, key string) (any, bool) {
    m := v.(proto.Message).ProtoReflect() // GUESS: Get is only ever called after Kind returned KindObject for this exact value
    fds := m.Descriptor().Fields()
    fd := fds.ByJSONName(key) // GUESS: transform authors will write camelCase JSON names; I chose to accept both
    if fd == nil {
        fd = fds.ByName(protoreflect.Name(key))
    }
    if fd == nil {
        return nil, false
    }
    if fd.HasPresence() && !m.Has(fd) {
        return nil, false // GUESS: absence, so $exists is false; proto3 implicit-presence scalars return their zero value
    }
    return unwrap(fd, m.Get(fd)), true
}

// unwrap turns scalars into admitted Go types and leaves messages, lists, and maps
// foreign so the engine routes them back through Access.
func unwrap(fd protoreflect.FieldDescriptor, val protoreflect.Value) any {
    switch fd.Kind() {
    case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
        return val.Int() // int64, carried exactly
    case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
        return val.Uint() // uint64 -> int64 or *big.Int per admission rules
    case protoreflect.BytesKind:
        return val.Bytes() // GUESS: []byte is what KindBytes/Bytes() means; I bypass Access for it by returning an admitted type
    case protoreflect.MessageKind:
        if isWellKnownTimestamp(fd) {
            return val.Message().Interface().(*timestamppb.Timestamp).AsTime().Format(time.RFC3339Nano) // GUESS: authors expect protojson's string, not {seconds, nanos}
        }
        return val.Message().Interface()
    // ...
    }
}

func (protoAccess) Range(v any, fn func(string, any) bool) {
    m := v.(proto.Message).ProtoReflect()
    // GUESS: "in order" for a message means field-number order; for a proto map with int64 keys I stringify the key
    m.Range(func(fd protoreflect.FieldDescriptor, val protoreflect.Value) bool {
        return fn(fd.JSONName(), unwrap(fd, val))
    })
}
// Len, Index, Number, String, Bool, Bytes omitted; Number, String, Bool, Bytes are dead code
// for me because unwrap already returns admitted scalars. GUESS: that is allowed.

func (s *server) Transform(ctx context.Context, req *pb.Request) (*pb.Response, error) {
    out, present, err := protoExpr.Eval(ctx, req.GetPayload())
    if err != nil {
        return nil, toStatus(err)
    }
    if !present {
        return &pb.Response{}, nil
    }
    // GUESS: out may still contain carried proto messages ("same type, same identity"), and
    // encoding/json would marshal those by struct reflection with the wrong names, so I
    // must walk the result and materialize foreign values myself. The library has no helper.
    plain, err := materialize(out, protoAccess{})
    if err != nil {
        return nil, status.Error(codes.Internal, "materialize")
    }
    b, err := json.Marshal(plain) // GUESS: int64 IDs come out as bare digits, which protojson would have quoted; my JS consumers will truncate them
    if err != nil {
        return nil, status.Error(codes.Internal, "encode")
    }
    return &pb.Response{Json: b}, nil
}
```

### (c) Selecting two fields of a large result

```go
ev, err := h.expr.Prepare(body) // GUESS: no ctx because Prepare does no work worth cancelling
if err != nil {
    return err // only invalid options, per the doc
}
defer ev.Close() // GUESS: safe to call after returned values escaped; doc says yes

if p := ev.Plan(); p.Mode == jsonata.ModeComplete {
    metrics.SelectiveMiss.WithLabelValues(p.Reason.String()).Inc()
}

id, idPresent, err := ev.Select(ctx, "id")
if err != nil { ... }
name, namePresent, err := ev.Select(ctx, "profile", "displayName")
// GUESS: "profile" is itself an object constructor in the expression, so the second element is
// also selective; Plan.Fields only lists top-level keys so I cannot confirm from the plan.
// GUESS: MaxWork is per Select, not cumulative across the Evaluation. If cumulative, a hot
// endpoint doing ten Selects has ten times less budget per field than a whole Eval.
resp := struct {
    ID   any `json:"id,omitempty"`
    Name any `json:"name,omitempty"`
}{}
if idPresent { resp.ID = id }
if namePresent { resp.Name = name }
```

### (d) Errors and absence

```go
func classifyEvalError(err error) (int, string) {
    switch {
    case errors.Is(err, context.DeadlineExceeded):
        return http.StatusGatewayTimeout, "transform timed out"
    case errors.Is(err, context.Canceled):
        return 499, "client closed request"
    }
    var e *jsonata.Error
    if !errors.As(err, &e) {
        return http.StatusInternalServerError, "transform failed"
    }
    switch {
    case e.Code == jsonata.CodeBudget:
        return http.StatusUnprocessableEntity, "transform exceeded its work budget" // GUESS: budget is the input's fault, not ours
    case e.Code == jsonata.CodeUnsupportedValue:
        return http.StatusInternalServerError, "transform reached an unsupported value" // our bug: an Access gap
    case strings.HasPrefix(e.Code, "S"):
        return http.StatusInternalServerError, "transform syntax" // GUESS: S-codes only arise from $eval at runtime since Compile already ran
    case e.Code == "D3137": // GUESS: this is the code $error() raises; the doc does not say
        return http.StatusBadRequest, "transform rejected input" // never echo e.Message or e.Value: Value may be the caller's own payload
    default:
        log.Warn("transform", "code", e.Code, "line", e.Line, "col", e.Column, "msg", e.Message) // GUESS: Message never embeds input data, safe to log
        return http.StatusInternalServerError, "transform failed"
    }
}
```

## What I would change

1. **Give me a way to materialize a result, or promise results never contain foreign values.**
   `func Materialize(v any, a Access) (any, error)` in package `jsonata`, and `func WithMaterializedResult() EvalOption` so the engine does it on the way out. Today "returned as the value that was carried: same type, same identity" means `{ "user": $.user }` over a proto input returns an `*Object` whose one member is a `*pb.User`. Every integrator behind a gRPC handler will write the same recursive walker, and every one of them will get the `Range` ordering or the `KindBytes` handling subtly different from the engine. The engine already knows how to walk a foreign value through `Access`; expose it.

2. **Ship a reflection `Access` for structs and admit named basic kinds natively.**
   `package access; func Struct(tag string) jsonata.Access` (tag `"json"` by default, honoring `omitempty` and `-`), and in the core package, admit any value whose `reflect.Kind` is one of the admitted kinds, so `type ID int64` and `type Status string` in a `map[string]any` do not fail with E1001 at first use. "Decoded JSON bodies" in real services are very often decoded into structs, not `any`, and the current doc's answer, "write an Access", is a nine-method chore for what should be a one-liner.

3. **Classify errors with a type, not a string prefix.**
   `func (e *Error) Kind() ErrorKind` with `ErrorSyntax, ErrorType, ErrorEvaluation, ErrorRaised, ErrorUnsupportedValue, ErrorBudget`, plus `func (e *Error) Unwrap() error` for the D3030 case wrapping the underlying parse failure. Also document the code `$error` produces and state whether `Message` can contain input content. My HTTP status mapping should not depend on `strings.HasPrefix(e.Code, "S")` and a guess at `D3137`.

4. **Make bounds a value so a cache key exists.**
   `type Bounds struct { MaxExpressionBytes, MaxDepth, MaxRecursion, MaxOutputNodes, MaxWork int }`, `func WithBounds(b Bounds) CompileOption`, `func DefaultBounds() Bounds`, and `func (e *Expression) Bounds() Bounds`. Keep the individual `WithMax*` options if you like, but the doc's own caching advice is unactionable with func-typed options, and every integrator will reinvent the struct above.

5. **Resolve the ownership and cancellation story for `Evaluation`, in the doc and in the design.**
   State that values returned by `Select` and `Complete` are shared among all selections of that `Evaluation` (so "constructed values are owned by the caller" is false until `Close`), that mutating an `*Object` returned by `Select` is a data race against a concurrent `Select` of the same field, and whether a `Select` whose ctx expires mid-shared-computation poisons that memoized field for other goroutines or is retried by them. Also make `text.Evaluate` honor `WithBytesEncoding` on output, or say plainly that it does not.

## Footguns

- **The default JSON decoder destroys the IDs you care about before the library is involved.** `json.Unmarshal` into `any` gives `float64`; `9007199254740993` is already `9007199254740992`. You must use `UseNumber()` or `text.Decode`. The package doc lists `json.Number` as admitted and never says why it matters.
- **Foreign values in results.** Anything carried from a proto input comes back as a proto. `encoding/json` will happily marshal `*pb.User` by struct reflection with Go field names, `XXX_` internals, and wrong enum rendering. You will not notice until a transform that used to compute a field switches to copying one.
- **Named types are not admitted.** `type UserID int64` inside a `map[string]any` is E1001 at first use, which is mid-evaluation, not at `Prepare`. Unit tests with literal maps pass; production values from your own code fail.
- **Nil slice and nil map are null, not empty.** `var items []any` never appended is JSON `null` to the expression, so `$count($.items)` is 0 either way but `$.items = []` is false and `$type` is `"null"`. Build inputs with `[]any{}`.
- **Output is not one Go type per JSON type.** Objects are `map[string]any` or `*Object` depending on whether the expression built them; numbers are `int64`, `float64`, `*big.Int`, or a carried `json.Number`. Typed consumers need a switch with seven arms or a round-trip through JSON. `Object.Map()` does not recurse, so it does not help.
- **`uint64` above `MaxInt64` becomes `*big.Int`.** Typed code expecting an integer type assertion on a proto `fixed64` field will get a pointer.
- **Large-int-times-float refuses at runtime.** `$.id * 1.5` works for small IDs and returns D1001 for an ID that is not exactly representable as float64. Transforms tested against small fixtures fail in production on real IDs. This is the correct behavior per the README; it is still a week-one page.
- **`WithBytesEncoding` only affects the expression's view of bytes.** A carried `[]byte` in the result is base64.Std under `encoding/json` no matter what you set. Set `URLEncoding` and `$string($.blob)` disagrees with the same field copied through.
- **`Error.Value` and possibly `Error.Message` alias caller data.** `$error($)` puts the whole request payload in `Value`. Never write `err.Error()` to the response for an untrusted expression.
- **Memoized selections are shared.** Two goroutines selecting the same field get the same `*Object`. `Object` "is not safe for concurrent mutation." The doc says constructed values are "owned by the caller." Both cannot be true; assume shared and read-only.
- **The input is frozen for the life of the `Evaluation`, not the call.** Lazy admission plus memoization means "must not mutate the input while an evaluation is running" extends until `Close` or GC. Do not reuse a pooled request body under a live `Evaluation`.
- **No wall-clock bound travels with the expression.** Bounds are work counts; time is only `ctx`. A transform with a small budget can still take long on a slow `Access` (a lazy-fetching one, which `Reads()` invites). Always set a per-request deadline.
- **Per-evaluation budgets are not per-process budgets.** Default `MaxWork` of 8M and `MaxOutputNodes` of 1M per evaluation times your concurrency is your memory ceiling. Cap concurrency yourself.
- **Package-level `Eval` recompiles every call.** It is documented, and someone will still put it in a handler.
- **`[]byte` in a `[]any` is a string, not an array.** If some other decoder handed you `[]uint8` for a byte array, the expression sees a base64 string.
- **`text.Decode` rejects duplicate keys; `encoding/json` does not.** Bodies that your other handlers accept will 400 here. Probably right for OpenBindings, surprising for a shared service.
- **Singleton unwrapping.** `$.items.id` over a one-element array is a scalar, not a one-element array. Language behavior, not the library's, but the Go caller's type switch will hit the wrong arm. Authors need `$.items.id[]`.

## Things the doc comments leave unclear

- Do `EvalOption`s passed to `Eval` on an expression built by `With` merge with or replace the resolved ones? Do two `WithBindings` calls merge, or does the last win?
- Is `Prepare` cheap and non-blocking? It plans; is planning bounded by `ReasonPlanBudget` work, and can it take long on a large expression?
- Is `MaxWork` per `Select`, per `Evaluation`, or per `Complete`? Same for `MaxOutputNodes` across memoized fields.
- What happens to a memoized field when one `Select`'s ctx is cancelled halfway: is the partial work discarded, retried by the next caller, or does the next caller receive `context.Canceled` from a ctx that was not theirs?
- Can `Select` or `Complete` be called after `Close`? Panic, error, or undefined?
- For nested object constructors, is `Select("a", "b")` selective at both levels? `Plan.Fields` is top-level only, so there is no way to see.
- What order does `Range` on an `Access` need to produce, and does the engine assume `Range` and `Get` agree?
- Does the engine call `Kind` before every `Get`/`Index`/`Len`, or may it cache a classification per value identity? Matters for an `Access` that returns different Go values each time.
- May `Access.Get` return an admitted type (bypassing `Number`/`String`/`Bool`/`Bytes`), or must it return the foreign value and let `Kind` classify? My implementation assumed the former.
- What code does `$error()` raise, and what is in `Code` when `$error` is given an object rather than a string?
- Can `Error.Message` contain input content? Can `Error.Value` alias the input?
- Which codes can only appear at `Compile`? Can `$eval` surface S-codes at runtime?
- Does a `json.Number` in a `map[string]any` output get re-rendered or passed through when the map is carried? "Only a pure copy preserves the representation": is carrying the containing map a pure copy of its members?
- Is `Position` in code points or bytes for a multi-byte expression source?
- What does `String()` return for a `With`-resolved expression, and is it stable enough to be my cache key?
- How large is an `*Expression` in memory, roughly, for cache sizing?
- Is an `*Object` safe for concurrent reads while an evaluation reads it too?
- `text.Evaluate` on a `present == false` result returns "no bytes": `nil` or empty slice?
- Is `Reads()` `known == false` for any expression using `$lookup` with a computed key, `$$`, or a binding, and does it include paths reached through bindings?
- Does `WithMaxExpressionBytes` count bytes or characters, and does it also bound the argument to `$eval`?

## On the concept

From the integrator's seat, "JSONata evaluator as regex engine" is a good framing because it tells me what kind of contract I am getting: a notation implemented natively, a closed environment, bounded resource use, and the host's values in and out with no serialization layer in between. Those are the four properties I actually need to run someone else's expression inside a request handler, and `regexp` is the library everyone already trusts to have them. The framing changes my expectations in a useful direction: I stop asking "does it match jsonata-js" and start asking "does it match the language spec and never hurt my process," which is the question `regexp` answers about RE2 syntax. The one place the analogy breaks for me is that a regex engine has no value model problem. Strings are strings. JSON values in a Go service come from three different decoders (encoding/json, protobuf, my own structs) with three different ideas of a number and an object, and the `Access` seam is where that gets papered over. That is also where the current design leaks: results can contain foreign values, and there is no materializer. The regex analogy does not have a counterpart for "your match result contains a pointer into a system you did not decode from."

"The host defines what a number is" is the right call, but only because the package then makes specific, checkable promises on top of it: lossless widening on admission, exact carriage, refusal over rounding, comparison by value across representations. An integrator does not want a philosophy; they want to know which byte sequence comes out the other end. This design gives me that as long as I control admission, and that is the qualification that matters. "Documentation as authority" is a harder sell to my transform authors, who will develop expressions in the JS playground and assume that is the truth. The portable core paragraph is the mitigation, and I would want it to be executable: an `Expression.Portable() bool` or a list of the constructs outside the core, so I can lint configuration documents at load time rather than discover a declared divergence in production.

For the int64 ID, here is the promise I want. If it came from a JSON body decoded with `UseNumber`, it is a `json.Number` and a selection returns that exact token; equality with a numeric literal in the expression is exact; arithmetic on it stays `int64` until it overflows, then errors. The package doc says all of that, and it should say in bold that the default `encoding/json` decoder breaks the first link. If it came from a protobuf `int64` field, it is a Go `int64` and a selection returns that same `int64`. Also promised. What neither the package nor the README says, and what will bite the OpenBindings project specifically, is that the encoding on the way out is not the evaluator's problem but it is somebody's: `encoding/json` will write that `int64` as bare digits, protojson would have quoted it, and the JavaScript client consuming the transformed response will lose the low bits that everything upstream carefully preserved. The evaluator has done its job, exactly as the README defines the job. The binding that decodes the wire in and encodes the wire out has to own the other half, and the Go package's doc should point at that boundary explicitly so no integrator assumes `json.Marshal(out)` is the finished story.
