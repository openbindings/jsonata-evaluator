# Panel cold read: go idiom purist

> 2026-09-18. Given only the class README and the vetted API stub; told not to read anything else. Verbatim.

## Grades

| Criterion | Grade | One-line justification |
|---|---|---|
| Idiomatic fit for Go | B- | Compile/MustCompile/ctx/errors.As are right out of the stdlib playbook, but `Range(fn)` instead of `iter.Seq2`, `Close() error` on a GC-managed object, magic-string `Plan.Reason`, bare `Complete`/`Selective` constants colliding with a method name, and an import path whose last element is the keyword `go` are all things a core reviewer would send back. |
| Ergonomics of the common path | B | `MustCompile` + `e.Eval(ctx, in)` is two lines and reads well; the tax is that every consumer of an object result must write a two-arm switch (`map[string]any` if carried, `*Object` if constructed), and every call site must destructure a triple. |
| Correctness and footgun risk | C+ | The numeric tower (the whole reason this library exists) is under-specified at exactly the points a caller will type-switch on: literal types, mixed-width promotion, `json.Number` arithmetic; map iteration order is unspecified, so `$keys` over a `map[string]any` input is nondeterministic unless the doc says otherwise. |
| Performance headroom the API permits | B | The bones are good: compile once, by-reference carriage, no text on the path, `Reads()` for lazy fetch, selective evaluation; the leaks are per-call variadic option slices, mandatory synchronization inside a concurrent-safe memoizing `Evaluation`, `any`-boxing through `Access.Number`, and allocating `Keys()`. |
| Concept soundness | B | The regex framing is sound for "notation shared, engine native", and documentation-as-authority is workable once you admit the fixtures are the real referee; but `*Object` as the constructed-object type contradicts the README's own "docs silent, value model wins" rule, and "host defines number" is a deliberate portability trade the README should own more loudly. |
| Overall | B- | A strong shape with the right stdlib instincts, undermined by heterogeneity on the output side and silence on the exact semantics this library is being built to get right. |

## What I would change

### 1. Do not return a library type for constructed objects by default

The package doc says a constructed object is an `*Object` "which preserves insertion order as the language requires." The README says, in the same breath, that the value model governs "what values are" and that where the documentation is silent, the value model wins. The JSONata documentation does not state that object key order is significant; `$keys` is documented as returning the keys, with no ordering clause. So by the project's own split, key order is the host's business, and Go's ordinary object value is `map[string]any`, unordered. `*Object` is a JavaScript value-model fossil (JS objects happen to preserve insertion order) imported through the back door, which is exactly what the README says this class refuses to do.

The practical damage is worse than the doctrinal one. Because carriage is exact, `$.user` returns the input `map[string]any`, while `{ "user": $.user }` returns an `*Object`. A caller receiving "an object" gets one of two unrelated types depending on how the transform author spelled it. Every consumer must write:

```go
switch o := v.(type) {
case map[string]any: ...
case *jsonata.Object: ...
}
```

`encoding/json` never does this to you; `regexp` never does this to you. Pick one:

```go
// Default: constructed objects are map[string]any, the host's value.
// Opt in to order when the caller is going to serialize:
func WithOrderedObjects() EvalOption
```

If you can cite the documentation passage that makes order normative, keep `*Object` but then apply it uniformly (admit only `*Object` as input, convert maps on admission), because a heterogeneous "object" type is the one outcome that is wrong under either reading.

### 2. Write down the numeric tower, in a table, in the package doc

For a library whose reason to exist is "large integer IDs survive", the doc leaves the following unanswered, and each one is a type switch a caller will get wrong:

- What Go type is the literal `1`? `int`? `int64`? What is `1.5`? (encoding/json precedent: it says `float64` and `json.Number` explicitly.)
- `int8 + int8` overflowing 127: error, or promote? `CodeOverflow` says "exceeded int64 or uint64", implying promotion to 64 bits, but the prose says "integer with integer stays integer and overflow is an error."
- `uint64(1<<63) + int64(-1)`: the result is representable but in neither operand's type.
- `json.Number("9007199254740993") + 1`: is `json.Number` parsed to `int64` first? To `*big.Int` on overflow? To `float64` if it has a fraction? If `json.Number("0.1000000000000000000001")` becomes a `float64`, that is a silent approximation and violates README rule 8.
- `float32` is admitted but the promotion rule only names `float64`.
- `NaN` or `Inf` in the input (legal `float64` values): carried, or refused on admission?

Wording I would ship, roughly:

> Literals are int64 and float64. Integer arithmetic is performed in int64, promoting to uint64 only when both operands are unsigned, and an unrepresentable result is CodeOverflow; there is no promotion to *big.Int. A json.Number is parsed on first use: as int64 if it fits, else *big.Int if it is an integer, else float64 if the decimal round-trips exactly, else CodeUnsupportedValue. float32 operands widen to float64. A non-finite input is refused on admission.

Whatever the rules actually are, the table has to exist, and `go doc jsonata` has to show it.

In the same section: state map iteration order. `encoding/json` sorts map keys; say you do the same, or `$keys`, `$each`, `$spread`, and object construction from a `map[string]any` input are nondeterministic across runs, which no conformance suite will catch because fixtures happen to run once.

### 3. Fix the import path: the last element is the keyword `go`

`text.go` imports `jsonata "github.com/openbindings/jsonata/go"`. The package name defaults to the last path element, and `go` is a keyword, so the alias is mandatory at every import site in every consumer forever. That is a permanent papercut and an unusual one; `golang.org/x/*` and every stdlib package have a last element that is the package name. Also, the README says the repository is `jsonata-evaluator` with the Go member at `go/`, and the package doc cites `contract/HOST-NATIVE-EVALUATORS.md`, which is not in the README's layout. Three different stories about where this lives.

Use `github.com/openbindings/jsonata-evaluator/go/jsonata` (module root at `go/`, package in `go/jsonata/`), so the import is `"github.com/openbindings/jsonata-evaluator/go/jsonata"` and `jsonata.Compile` needs no alias. The subdirectory tag scheme (`go/vX.Y.Z`) already assumes the module root is `go/`, so this costs nothing.

### 4. Repair the `Evaluation` lifecycle and the names around it

- `Close() error`: nothing here can fail. Releasing references is the collector's job. If the real reason is a `sync.Pool` of scratch state, then `Close()` with no result and "may be called to return scratch memory to the pool; otherwise the collector reclaims it" is the honest contract. "The Evaluation must be closed" is a MUST with no consequence, and Go readers know that (`bytes.Buffer`, `strings.Builder`, `regexp.Regexp` have no Close). Precedent for `Close() error` is io.Closer, where the underlying resource can genuinely fail to close (`*os.File`, `*sql.Rows`).
- `const Complete Mode` and `func (v *Evaluation) Complete(...)`: same identifier, same package, different meanings. Two modes is a bool: `Plan{Selective bool; Reason Reason; Fields []string}`. If you keep an enum, prefix it (`ModeSelective`, `ModeComplete`), as `reflect.Kind` and `token.Token` do.
- `Plan.Reason string` with a documented list of magic strings is un-Go. Make it a typed constant with a `String()` method, or, since a reason is "why selection was refused", make it an `error` value and let callers `errors.Is` it.
- `Prepare` collides with `database/sql`'s `Prepare`, which means "compile" there; a reader arriving from sql will assume it parses. Something like `e.Bind(input, opts...)` is closer to what it does, though it competes with `WithBindings`; `e.Over(input)` is odd but unambiguous. I hold this one loosely; the first three I do not.
- `Source()` should be `String()`, exactly as `(*regexp.Regexp).String()` returns the source text. It then satisfies `fmt.Stringer` for free and shows up in `%v`.

### 5. Modernize `Object` and thin out `Access`

`Object`:
- `Range(fn func(k string, v any) bool)` is the pre-1.23 `sync.Map` shape. Since Go 1.23 the idiom is `func (o *Object) All() iter.Seq2[string, any]` and `Keys() iter.Seq[string]` (see `maps.Keys`, `maps.All`, `slices.All`). A new library shipping `Range` in 2026 will look dated on day one.
- Make the zero value usable, as `bytes.Buffer` and `strings.Builder` are, so `&jsonata.Object{}` works and `NewObject` is a convenience, not a requirement. The proverb is "make the zero value useful."
- `MarshalJSON` without `UnmarshalJSON` is asymmetric: the type is advertised as "a valid input where key order matters", but there is no path from ordered JSON text into it. Implement `json.Unmarshaler`.

`Access`:
- Nine methods on one interface is a hand-rolled `reflect.Value`. The bigger the interface, the weaker the abstraction. More concretely, no method can fail: `Get` on a lazily decoded protobuf field, or `Number` on a `decimal128` that has no exact admitted representation, has no way to say so except by lying. Add error returns where the operation can fail (`Number(v any) (any, error)`), or invert the design and let foreign values describe themselves, as `driver.Valuer` and `json.Marshaler` do:

```go
// Valuer is implemented by values outside the admitted set.
type Valuer interface{ JSONataValue() (any, error) }
```

with `Access` kept only for types the caller cannot add methods to.

## What I would keep

- **`Compile` / `MustCompile` / immutable `*Expression` safe for concurrent use.** This is `regexp` verbatim, and it is the right precedent: the compile-once contract and the "panics on error, for build-time constants" doc line are exactly what a Go reader expects.
- **`context.Context` as the only cancellation and deadline mechanism.** No `WithTimeout` option, no separate cancel channel. Correct.
- **The `(value, present, err)` triple, reluctantly.** The stdlib precedent for "no result" is `sql.ErrNoRows`, and that precedent is widely regretted because it makes a common, non-exceptional outcome look like a failure. Absence is the ordinary result of `$.missing`; it is not an error, and using a sentinel value would leak into the value space where `nil` already means null. Three results is unusual but `(*bufio.Reader).ReadRune` and `runtime.Caller` show it is not forbidden. Defend it, but fix the doc gap noted below.
- **The closed environment.** No host functions, no callbacks. `regexp` has no callbacks either, and that is why it is safe to hand an untrusted pattern to it. This is the single most important property for the OpenBindings use case and the API surface honors it.
- **No JSON text on the evaluation path, with `text` as an explicitly non-authoritative subpackage.** This is the correct layering and the "it adds nothing the native API lacks" sentence is the right disclaimer.
- **Refuse rather than approximate**, with overflow and non-finite results as errors. Go's own integer arithmetic silently wraps; opting out of that at the language boundary is right for a value-carrying transform.
- **`Error` as a struct with `errors.As`, carrying the language's own codes.** Good. Position as a byte offset with -1 for unknown matches `json.SyntaxError.Offset` in spirit.
- **`BytesEncoding` as a one-method interface satisfied by `*base64.Encoding` unchanged.** Textbook small-interface design; keep it exactly as is.
- **`Reads()` and the selective-evaluation plan being inspectable rather than magic.** A caller that can ask "what will you read" and "how will you serve this" can build the lazy-fetch and partial-evaluation stories the project needs without the library guessing.
- **A default bound on expression size.** Untrusted input; 256 KiB is a reasonable ceiling.

## Things the doc comments leave unclear

- **The tuple on error.** When `err != nil`, what are `value` and `present`? Convention says zero values, but nothing states it, and "present is false for an absent result" invites the reading that `present == false && err != nil` might mean something.
- **Context errors.** Does a cancelled evaluation return `ctx.Err()` directly, or an `*Error` wrapping it? `errors.Is(err, context.Canceled)` must work; say which, and if it is `*Error`, add `Unwrap`.
- **Literal numeric types, promotion, `json.Number`, `float32`, NaN/Inf.** See change 2. This is the largest gap.
- **Map iteration order** for `map[string]any` inputs. See change 2.
- **The `Reads()` path grammar.** Are paths dotted strings? How is a key containing `.` or a backtick escaped? Is `$$.a.b` reported as `a.b` and does a wildcard or predicate make `known == false`? `[]string` with unstated syntax is a parser the caller has to guess at; `[][]string` would remove the question.
- **`Select` with zero path elements.** `Select(ctx)` is legal by signature; is it `Complete`? An error?
- **`Select` below constructor granularity into arrays.** "A structural lookup within the deepest selected field" with `string` path elements: how is index 3 spelled?
- **What `Select` does when the path names a key not in `Plan.Fields`** while `Mode` is `Selective`: absent, error, or silent fallback to whole evaluation?
- **Aliasing of carried values.** The doc says carried values keep identity. It does not say the consequence: mutating a returned `[]any` or `*Object` mutates the caller's input, and mutating the input during `Eval` from another goroutine is a race. Both need a sentence; `Object` says "not safe for concurrent mutation" but the input contract does not.
- **`WithBindings` copies or references the map?** If a caller mutates the map after `Prepare`, is the `Evaluation` affected? Are binding values also subject to `Access` and the admitted-set check?
- **`Evaluation` concurrency versus `Close`.** "Safe for concurrent use" and "must be closed" together raise: what does a `Select` racing `Close` do?
- **Defaults for `WithMaxDepth`, `WithMaxRecursion`, `WithMaxOutputNodes`.** Only `WithMaxExpressionBytes` states a default. For an untrusted-expression library, "unbounded unless set" is the footgun; "bounded by default, here is the number" is what `go doc` must show.
- **Intermediate size.** `MaxOutputNodes` bounds the result; `[1..100000000]` inside a `$count` never reaches the result. Is there an intermediate budget, or is ctx the only defense?
- **`$length`, `$substring`, and what a character is.** The README says "how long a string is" belongs to the host. The documentation says `$length` returns the number of characters, which is not silent. JS counts UTF-16 code units; Go would count runes. This is a place where documentation and reference implementation disagree and the host has a third answer; the package doc should say which.
- **`[]byte` under `$type`, equality, and ordering.** Is `[]byte` a string for `$type`? Does `bytes = "aGVsbG8="` compare equal after encoding, or is comparison by raw bytes? "Presents as a string when an expression uses it as one" does not decide these.
- **`Access.Number` when exactness is impossible.** "Exactly" with no error return: what does the implementer do with a value that has no exact admitted representation?
- **`text.Evaluate`.** How `[]byte` and `json.Number` are encoded on output (presumably `encoding/json` rules, which base64-std `[]byte`, matching the default `BytesEncoding`; say so). "No bytes" for absent: `nil` or `[]byte{}`? No way to pass compile options, so the 256 KiB bound cannot be tightened from the convenience path. And the name is `Evaluate` where the parent package says `Eval`.
- **Package-level `Eval` cannot take `CompileOption`.** Same limitation; either accept both option types or document that one-shot evaluation runs with compile defaults.

## On the concept

The regex framing is sound for the part of the claim it is actually making: a notation with a shared meaning, engines written natively per host, each declaring what it does not support. Go's `regexp` is in fact the best available precedent, because it made exactly this move and paid for it in public: RE2 syntax, no backreferences, documented as a deliberate non-support rather than a bug. "Declared divergences pinned to fixtures" is the same idea with better bookkeeping, and it is the right idea. The analogy also correctly predicts the API shape (`Compile`, `MustCompile`, immutable compiled object, no callbacks into the host), and the API follows it. That much I would defend.

Documentation-as-authority holds up less cleanly than the README presents it, and the README should say so rather than let a reader discover it. docs.jsonata.org is explanatory prose with examples, not a specification; there is no grammar, no evaluation-order rules, no formal definition of sequence flattening. The conformance suite is derived from the reference implementation, and membership rule 1 verifies against that suite. So the operative authority is "the fixtures, minus exceptions you can justify from the prose." That is a perfectly good stance and it is how every second implementation of an informally specified language has ever been built, but it is not "the documentation wins." Where the prose is silent or ambiguous, the fixtures will decide, and the fixtures encode the reference's value model. The `*Object` decision is the first casualty: the fixtures pass order-sensitive expectations because JS objects are ordered, the docs say nothing, and the draft chose the reference's answer while claiming the host's. Expect more of these, and expect them precisely in the corners the README calls "the value model's business."

"The host defines what a number is" is the right call for exact carriage and the wrong call for portability, and the README should stop treating those as the same virtue. Carriage exactness (a 64-bit ID selected and returned unchanged) is a host-native property and is unambiguously correct for a transform language sitting behind an API boundary; the reference implementation cannot offer it and that alone justifies the class. Arithmetic is different. A transform that computes `$.id + 1` yields an exact integer in Go and a rounded double in JS, and `$string(1/3)` may well format differently. The README's answer is the "portable core", which is an honest observation dressed as a footnote; it should be the headline, because for OpenBindings the audience is transform authors who will run one expression through whichever member the consumer happens to have. This is where the regex analogy breaks: a regex engine's output is match positions over the host's own strings, which never leave the host, so nobody cares that `.` matches differently across engines; a JSONata evaluator's output is data that crosses systems, so value-model divergence is visible to the recipient. The class has the right instinct (refuse rather than approximate, declare rather than hide) but it needs to state the trade in one sentence: exactness is guaranteed, arithmetic beyond the portable core is host-defined and may differ between members, and that is a feature the author opts into by leaving the core.
