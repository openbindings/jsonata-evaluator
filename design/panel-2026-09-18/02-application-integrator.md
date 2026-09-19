# Panel cold read: application integrator

> 2026-09-18. Given only the class README and the vetted API stub; told not to read anything else. Verbatim.

## Grades

| Criterion | Grade | One-line justification |
|---|---|---|
| Idiomatic fit for Go | B | Compile/MustCompile, ctx-first Eval, functional options, `errors.As` are all right; `Close() error` on a must-close object, a variadic `path ...string`, a nine-method error-less `Access`, and an import path ending in `/go` with a package named `jsonata` are all things I would have to explain to a reviewer. |
| Ergonomics of the common path | B- | Two lines to compile and evaluate, then an `any` that fans out to seventeen concrete types the moment I need a typed value; every call site must re-supply policy options; `text` cannot take a compiled expression. |
| Correctness and footgun risk | C | Unstated defaults for depth and recursion (a Go stack overflow is a process death, not a recoverable panic), results that alias inputs and memoized state, foreign values carried by identity into the output, `map[string]any` key order left unspecified, and `[]byte` encoded one way by the engine and another by `encoding/json`. |
| Performance headroom the API permits | B+ | By-reference admission, compile-once, selective evaluation with shared memoization, and `Reads()` for lazy fetch are exactly what a request path wants; what is missing is a way to pin policy once so the per-call option slice stops allocating, and any budget on intermediates. |
| Concept soundness | B+ | The regex framing is the right promise for an integrator (closed, budgeted, host-native, deterministic modulo declared functions); the number story is sound in principle but the API tells me only half of the promotion lattice and none of the boundary rules I need for IDs. |
| Overall | B- | Usable tomorrow with a wrapper around it; not safely usable tomorrow without one, and the doc comments do not tell me which parts need the wrapper. |

## The integration I would write

### (e) Startup: cache, policy bundle, fail-fast validation

```go
// One place that holds the policy I could not attach at Compile.
type Transformer struct {
    expr *jsonata.Expression
    opts []jsonata.EvalOption
}

var protoAcc = protoAccess{} // see (b)

func NewTransformer(src string) (*Transformer, error) {
    expr, err := jsonata.Compile(src,
        jsonata.WithMaxExpressionBytes(64<<10),
        jsonata.WithMaxDepth(64), // GUESS: no default is stated; I assume the default is unbounded and picked a number.
    )
    if err != nil {
        return nil, fmt.Errorf("transform %q: %w", truncate(src), err)
    }
    return &Transformer{
        expr: expr,
        opts: []jsonata.EvalOption{
            jsonata.WithAccess(protoAcc),
            jsonata.WithMaxRecursion(512),      // GUESS: no default stated; assumed unbounded, which for untrusted config means a Go stack overflow and a dead process.
            jsonata.WithMaxOutputNodes(200_000), // GUESS: no default stated.
            // GUESS: there is no step/time/allocation budget, so I rely on ctx deadline alone and assume "function boundaries" is fine-grained enough to catch $pad("x", 1e9). I do not believe it is.
        },
    }, nil
}

// Compile every configured transform at boot so a bad config fails deploy, not a request.
var transforms = map[string]*Transformer{}   // keyed by config path; expressions are immutable and goroutine-safe per the doc, so this is shared freely.
var dynamicCache = lru.New[string, *jsonata.Expression](1024) // for expressions that arrive at runtime; key MUST include compile options, which I concatenate into the key by hand.
```

### (a) HTTP handler over a decoded JSON body

```go
func (t *Transformer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    dec := json.NewDecoder(io.LimitReader(r.Body, 4<<20))
    dec.UseNumber() // GUESS: nothing in the doc says this is the required decode path for exact large integers; I inferred it from json.Number being admitted and from the text package's "exact number tokens".
    var in any
    if err := dec.Decode(&in); err != nil {
        http.Error(w, "bad body", http.StatusBadRequest)
        return
    }

    ctx, cancel := context.WithTimeout(r.Context(), 250*time.Millisecond)
    defer cancel()

    out, present, err := t.expr.Eval(ctx, in, t.opts...)
    if err != nil {
        t.writeEvalError(w, err)
        return
    }
    if !present {
        w.WriteHeader(http.StatusNoContent) // GUESS: nothing says how "absent" should reach a JSON consumer; text.Evaluate says "no bytes", so I mirrored that.
        return
    }
    w.Header().Set("Content-Type", "application/json")
    // GUESS: because input contained only json-decoded types, I assume out contains only
    // nil/bool/string/json.Number/int64/float64/*big.Int/[]any/map[string]any/*Object, all of which
    // encoding/json handles (json.Number verbatim, *big.Int as a number, *Object via MarshalJSON).
    // I have NOT been told what concrete integer type `$.n + 1` produces from a json.Number operand.
    if err := json.NewEncoder(w).Encode(out); err != nil {
        log.Printf("encode: %v", err)
    }
}
```

### (d) Errors and absence

```go
func (t *Transformer) writeEvalError(w http.ResponseWriter, err error) {
    var je *jsonata.Error
    switch {
    case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
        // GUESS: the doc does not say whether ctx errors come back raw, wrapped, or as a *jsonata.Error with an engine code.
        http.Error(w, "transform timed out", http.StatusGatewayTimeout)
    case errors.As(err, &je):
        switch {
        case je.Code == jsonata.CodeBudget:
            http.Error(w, "transform too expensive", http.StatusUnprocessableEntity)
        case je.Code == jsonata.CodeUnsupportedValue:
            // My Access returned KindUnknown for something; that is my bug.
            http.Error(w, "internal", http.StatusInternalServerError)
        case strings.HasPrefix(je.Code, "S"):
            // GUESS: I assume S-codes are syntax and cannot occur after a successful Compile,
            // unless $eval exists and is supported, which the doc does not say.
            http.Error(w, "internal", http.StatusInternalServerError)
        default:
            // GUESS: T-codes, D-codes, and E1001 mean "the expression did not fit this payload".
            // That is a config-author problem surfaced by upstream data, so I blame the upstream.
            log.Printf("transform %q failed at byte %d (%s): %s", je.Token, je.Position, je.Code, je.Message)
            http.Error(w, "upstream shape mismatch", http.StatusBadGateway)
        }
    default:
        http.Error(w, "internal", http.StatusInternalServerError)
    }
}
```

### (b) gRPC handler over a protobuf message

```go
type protoAccess struct{}

func (protoAccess) Kind(v any) jsonata.Kind {
    switch x := v.(type) {
    case *timestamppb.Timestamp:
        return jsonata.KindString // my choice: present as RFC 3339, matching protojson
    case proto.Message:
        // GUESS: a typed nil (*pb.User)(nil) arrives as a non-nil `any`; the doc does not say
        // whether the engine unwraps that, so I map it to null myself.
        if x == nil || !x.ProtoReflect().IsValid() {
            return jsonata.KindNull
        }
        return jsonata.KindObject
    case protoreflect.List:
        return jsonata.KindArray
    case protoreflect.Map:
        return jsonata.KindObject
    }
    return jsonata.KindUnknown
}

func (protoAccess) Get(v any, key string) (any, bool) {
    m := v.(proto.Message).ProtoReflect()
    // GUESS: nothing says whether expression authors write proto field names or JSON names.
    // I chose JSON names because authors will test against protojson output.
    fd := m.Descriptor().Fields().ByJSONName(key)
    if fd == nil || (fd.HasPresence() && !m.Has(fd)) {
        return nil, false
    }
    return unwrap(m.Get(fd), fd), true // scalars come back as int64/uint64/string/[]byte/bool/float64 (admitted); messages and lists come back foreign and route back through Kind.
}

// Keys, Len, Index, Number, String, Bool, Bytes: about eighty more lines, all mine,
// including enum-as-name, map-key stringification, and oneof presence. The doc names
// "protocol buffer messages" as the motivating case and ships nothing for it.

func (s *svc) Get(ctx context.Context, req *pb.GetReq) (*pb.GetResp, error) {
    out, present, err := s.t.expr.Eval(ctx, req, s.t.opts...)
    if err != nil {
        return nil, toStatus(err)
    }
    if !present {
        return &pb.GetResp{}, nil
    }
    // GUESS: `out` may contain a *pb.Address carried by identity ("same type, same identity").
    // encoding/json would serialize its internal fields; structpb.NewValue rejects it.
    // So I walk the result myself, and while walking I must also decide what to do with
    // json.Number and *big.Int, which structpb cannot hold (it is float64-only).
    sv, err := s.toStructValue(out) // int64 > 2^53 becomes a string here; that is my rule, not the library's.
    return &pb.GetResp{Body: sv}, err
}
```

### (c) Selecting two fields of a large result

```go
func (t *Transformer) selectFields(ctx context.Context, in any, fields []string) (*jsonata.Object, error) {
    ev, err := t.expr.Prepare(in, t.opts...)
    if err != nil {
        return nil, err
    }
    defer ev.Close() // GUESS: I do not know what error Close can return or what leaks if I skip it; errcheck will flag this line forever.

    if p := ev.Plan(); p.Mode == jsonata.Complete {
        metrics.SelectiveFallback.WithLabelValues(p.Reason).Inc() // useful, once I learned the Reason strings are stable identifiers (GUESS)
    }

    resp := jsonata.NewObject()
    for _, f := range fields { // f comes from ?fields=, so it is user input; keys are literal, so I assume no injection surface (GUESS).
        v, present, err := ev.Select(ctx, f)
        if err != nil {
            return nil, err
        }
        if present {
            resp.Set(f, v) // GUESS: v may alias memoized state inside ev; I do not mutate it, but nothing tells me I may not.
        }
    }
    // GUESS: ev.Select(ctx, "profile", "displayName") does a structural lookup under "profile" if only "profile" is a constructor key; I could not confirm what happens if "profile" is constructed but is an array.
    return resp, nil
}
```

## What I would change

**1. Give every budget a stated finite default, accept budgets at Compile, and add a work budget.**

```go
type Option interface{ CompileOption; EvalOption } // or one Option type applied by both
func WithMaxDepth(n int) Option        // default 64
func WithMaxRecursion(n int) Option    // default 1024
func WithMaxOutputNodes(n int) Option  // default 1<<20
func WithMaxWork(n int) Option         // counts evaluation steps and allocated nodes, including intermediates; default 10<<20
```

The expression is the untrusted artifact and it is the thing I cache; policy belongs on it, not re-supplied on every `Eval`. Separately, the package doc must contain the sentence "no expression, whatever its content, can terminate the process": recursion and parse depth in Go end in a fatal stack overflow, not a panic, and today the defaults are not even stated. `WithMaxOutputNodes` bounding only the result means `[1..100000000][0]` is a 1-node result with an 800 MB intermediate; ctx "function boundaries" will not interrupt `$pad`.

**2. Define the output contract and provide a materializer.**

```go
// WithAdmittedOutput materializes every foreign value in the result through the Access
// into admitted values. Without it, foreign values are carried by identity.
func WithAdmittedOutput() EvalOption

// Decode assigns a result to dst with encoding/json semantics, without producing JSON text.
func Decode(v any, dst any) error
```

"Same type, same identity" is right for carriage fidelity and wrong for a result I am about to hand to `encoding/json` or `structpb`. Every integrator with an `Access` will write the same walker. `Decode` closes the typed-Go path; today the honest route is Eval, Marshal, Unmarshal.

**3. `Close()` returns nothing, or Evaluation is not closable.**

```go
func (v *Evaluation) Close()
```

Nothing described (captured input, memoized fields) needs deterministic release; if there is a pool underneath, say so and keep `Close`, but an error from releasing memory has no caller who can act on it. As written, every handler grows a `defer ev.Close()` that lint flags and a leak vector when someone forgets it.

**4. Classify errors so a handler can choose a status without string-prefix matching.**

```go
type Class uint8
const ( Syntax Class = iota + 1; Runtime; Budget; Unsupported; Overflow )
type Error struct { Class Class; Code string; Message string; Position int; Line, Column int; Token string }
var ErrBudget, ErrUnsupportedValue error // errors.Is targets
```

And one sentence: "A cancelled or expired ctx is returned so that `errors.Is(err, ctx.Err())` holds." I need to distinguish "config author wrote it wrong" (log for humans), "upstream data did not match" (502-ish), and "resource" (429/504) on every request. Byte offsets are fine for me, but the person reading the log wants line and column.

**5. Make `text` useful in production and state the decode contract in the main package.**

```go
package text
func Decode(in []byte) (any, error)                                                     // the exact-number decoder, exposed
func Eval(ctx context.Context, expr *jsonata.Expression, in []byte, opts ...jsonata.EvalOption) ([]byte, error)
```

And in package jsonata's Values section: "Decode input with `json.Decoder.UseNumber`; a float64 produced by the default decoder has already lost integer precision above 2^53, and the engine carries it as the float64 it became." Right now `text.Evaluate` recompiles per call, so the only package that knows the exact-decode recipe is the one I cannot use on a hot path.

Honorable mention: ship the two `Access` implementations the doc itself names (a reflection/`json`-tag struct access and a protoreflect access) as subpackages. The interface has nine methods and two dozen semantic decisions (typed nil, presence, enums, well-known types, key naming); leaving all of them to each integrator guarantees that no two services agree on what `$.createdAt` is.

## Footguns

- **Process death from untrusted recursion or nesting.** `$f := function($n){$f($n+1)}; $f(0)` and `((((((` at depth 100k are both config-supplied. With unstated defaults I must assume unbounded, and Go's response is `fatal error: stack overflow`, uncatchable, taking every in-flight request with it. This is the single thing I would block a rollout on.
- **Cooperative cancellation is not a budget.** ctx is checked at "function boundaries"; `$pad`, `$join`, range construction, and `$string` on a huge array are each one function. A 250 ms deadline does not stop a 2 GB allocation that happens inside one call.
- **Results alias inputs and memoized state.** Admission is by reference, carriage is by identity, `Evaluation` memoizes and is concurrently usable. So: reuse a decoded-body buffer and the result corrupts; call `Object.Set` on something a `Select` returned and another goroutine's `Select` may see it. The doc says "values already returned remain valid" and nothing about ownership.
- **Foreign values in the output.** With an `Access`, `$.address` returns my `*pb.Address` unchanged. `encoding/json` will serialize its unexported-state fields and `XXX_` internals; `structpb.NewValue` returns an error; `protojson` cannot be told about the `*Object` wrapper around it. I need a walker before any encoder.
- **`map[string]any` key order.** `$keys($)[0]`, `$each`, `$spread`, and object-to-array constructions over a Go map are nondeterministic unless the engine sorts, and the doc does not say it does. `Access.Keys` says "in order" for foreign objects and says nothing for the admitted map.
- **Typed nil pointers.** `(*pb.User)(nil)` as `any` is non-nil; it reaches `Access.Kind` rather than being treated as null. Forget that case and `$exists(user)` is true for an unset message.
- **Numeric result type flips with the data.** `$.total / $.count` is an integer when exact and float64 otherwise; `json.Number("9007199254740993") + 1` becomes some integer type the doc does not name. JSON encoding hides this; `v.(int64)` in typed code panics on Tuesday.
- **`[]byte` is encoded two ways.** `WithBytesEncoding(base64.URLEncoding)` governs `$string(bytes)` and string coercion; a purely copied `[]byte` in the result reaches `encoding/json`, which uses `StdEncoding`. Same payload, two alphabets, depending on whether the author wrote `$.sig` or `$string($.sig)`.
- **`$now`, `$millis`, `$random` versus "the environment is closed".** README rule 7 says no host state; the package says every standard-library function is owed; the selective-evaluation doc mentions "pure" subexpressions, implying impure ones exist. I cannot cache results or trust selective-versus-complete identity without knowing.
- **`$eval`.** If supported, it is a compile at runtime that bypasses my `Compile` options unless the doc says budgets are inherited. Not mentioned.
- **Regex dialect.** Host-native means RE2: no lookaround, no backreferences. Config authors who tested on the JavaScript reference will ship `(?=...)`. I need to know whether that fails at `Compile` (good, boot-time) or at `Eval` (bad, per-request).
- **`Compile` has no ctx but the doc says cancellation is honored "at compile".** For a 256 KiB untrusted expression that is a statement I cannot rely on.
- **`text.Evaluate` recompiles per call and returns "no bytes" for absence.** `[]byte(nil)` and `[]byte{}` are both "no bytes"; check `len`, never `== nil`.
- **`Prepare` validates bindings but not input.** An unsupported value surfaces from the first `Select`, so my "prepare succeeded" log line is not a health signal.
- **Cache keys.** `*Expression` is immutable and shareable, so an LRU keyed by source is correct only if the key includes every `CompileOption` value. Nothing on `Expression` reports its options.

## Things the doc comments leave unclear

- Defaults for `WithMaxDepth`, `WithMaxRecursion`, `WithMaxOutputNodes`, and whether "unset" means unbounded.
- What "plan-budget" is, and which option sets it (none listed).
- Whether any input can crash the process rather than return an error; whether internal panics are recovered.
- The concrete Go type of an integer result: literal `1`, `int32 + int64`, `uint64 + int64`, `json.Number + int`, `float32 + int`. The lattice is stated only for the int/float64/*big.Int corners.
- How `json.Number` is classified for arithmetic: integer if the token is integral? Which width? Is a token above `MaxUint64` an overflow error or promoted to `*big.Int`? Is `1e400` an error at admission or at first use?
- Whether `$string(json.Number("12345678901234567890"))` yields the token or a float format.
- Iteration order for `map[string]any` input.
- Whether `Object.Map()` is deep or shallow, and whether nested `*Object` values stay `*Object`.
- Ownership of returned values: may I mutate a returned `*Object`? May the engine mutate an input `*Object`? Does the transform operator (`~> |...|`) clone through an `Access`, and if the `Access` cannot construct, what happens?
- The form of ctx errors (raw, wrapped, or `*Error`) and which `Code` they carry, if any.
- The stability of `Plan.Reason` strings as identifiers (metrics labels) versus prose.
- `Select(ctx)` with zero path elements: `Complete`, error, or the whole object?
- `Select` when the selected constructor key holds an array: does the next path element index or map over elements?
- The format of `Reads()` paths (top-level keys only, or dotted), whether they are `Access` key names, and whether `$$` or `$keys($)` makes `known` false.
- Whether `$now`, `$millis`, `$random`, and `$eval` are supported, and if `$eval` is, whether compile budgets apply to its argument.
- Whether regex literals are compiled at `Compile` or at first evaluation, and which dialect.
- What `Evaluation.Close` can fail with, and what is leaked if it is never called.
- Whether `Access` may be called concurrently (an `Evaluation` is concurrently usable, so presumably yes, but my `Access` might not be).
- What `Access.Number` should do when it cannot return the value "exactly" (a decimal type): error is impossible, so refuse how?
- Whether `WithBindings` values may be foreign values reached through the `Access`, and whether binding values are captured by reference too.
- HTML escaping in `text.Evaluate` output.
- The module path: the package imports `github.com/openbindings/jsonata/go` while the README describes a repository named `jsonata-evaluator` with a `go/` directory; the package doc cites `contract/HOST-NATIVE-EVALUATORS.md`, which the README's layout does not contain.
- Which JSONata version the compiled `Expression` targets, as an exported constant I can surface in a `describe` endpoint.

## On the concept

From the integrator's seat the regex framing is not decoration; it is a set of guarantees I already know how to operate. `regexp` gives me: compile once, share freely, linear time, no callbacks into my code, no I/O, a closed set of things it can do to me. If a JSONata evaluator makes the same promises, I can put untrusted expressions on a request path the way I already put untrusted patterns there, and that is the whole value proposition for OpenBindings, where the transform arrives with the document. The framing raises the bar accordingly: `regexp` refuses pathological patterns at `Compile` and cannot blow the stack; this API, as drafted, may do either at `Eval`, with unstated defaults. The concept is sound; the API has not yet earned the analogy on the resource-safety axis, and that is the axis the analogy is chosen for.

Documentation-as-authority is a good deal for me for a reason the README undersells: it makes the JavaScript engine's incidental behavior my problem only where the docs are silent, and it makes the declared-divergence ledger the thing I read before accepting a config. But the README's "portable core" claim is weaker than it looks from where I sit. `$string`, `$type`, equality, and `$lookup` agree between documentation and reference "without exception" only for values inside the IEEE double range. The very reason this library exists is values outside that range. So the honest statement is that the portable core is portable for authors whose data fits in a JavaScript number, and everyone else gets documented behavior; I would rather the README say that than have a config author discover it via an ID that came back as `1.2345678901234567e+19` from a different member.

"The host defines what a number is" is the right call for the engine and the wrong place to stop for a library that moves values between systems. What I need is a boundary promise, not a value-model philosophy. For an int64 ID from a JSON body, decoded with `UseNumber`: the promise should be that a `json.Number` selected, copied, or rearranged reaches the encoder as the same token; that `$string` of it yields the token; that equality with an `int64` holding the same value is true; and that arithmetic on it either stays exact in a named integer width or refuses, never rounds. For the same ID from a protobuf `int64` field: carried as `int64`, encoded as a number, compared by value against the `json.Number` form, and refused rather than rounded if the sink is float64-only. The draft promises the middle two of those (value comparison, refuse-not-round) and is silent on the token, on `$string`, and on the width of arithmetic results. Those three sentences would turn "the host defines the number" from a stance into a contract I can put in a runbook. Without them, the fidelity the library so carefully preserves is lost one line later, in the first `float64` my encoder or `structpb` demands, and the library's doc gives me no warning that this is where the fight actually happens.
