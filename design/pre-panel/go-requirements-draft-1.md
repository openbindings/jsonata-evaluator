# openbindings/jsonata/go: requirements

> **Draft for review, 2026-09-18.** What the Go member of the
> [host-native evaluator class](../contract/HOST-NATIVE-EVALUATORS.md) must
> do, stated before any design. Every requirement below is either a fact
> about the pinned JSONata documentation or a fact about Go. None is a
> policy the project has to defend.

## 1. Identity

The Go member of the class. It supersedes the current text-boundary public
executor (`Evaluate(ctx, expression, inputJSON, bindingsJSON)`), which
exists to keep two engines byte-comparable and pays a serialize-then-parse
round trip on every call for it. The engine underneath stays: the gnata-derived
lexer, parser, evaluator, and standard library that already pass the reference
suite on the native path. What changes is the boundary, the seams where
values are compared and combined, and the addition of selective evaluation.

## 2. Value domain

**R2.1 Admitted input.** Go values, by reference, with no serialization:

| Kind | Go types admitted directly |
| --- | --- |
| null | `nil` |
| boolean | `bool` |
| string | `string` |
| number | `json.Number`, `int`, `int8`…`int64`, `uint`, `uint8`…`uint64`, `float32`, `float64`, `*big.Int` |
| bytes | `[]byte` |
| array | `[]any` |
| object | `map[string]any`, the engine's ordered object type |

**R2.2 Foreign values.** Anything else enters through an accessor the caller
supplies, of the shape `kind`, `get`, `keys`, `length`, `numberToken`,
`scalar`, and optionally `bytes`, so a protobuf message or an arbitrary struct
can be evaluated without first being converted. The engine reads through the
accessor lazily; it never walks the caller's tree up front.

**R2.3 Bytes.** `[]byte` is carried as bytes. Its string view is the boundary
string supplied by the caller's binding adapter (Base64 by default, base64url
where the binding says so), materialized only when an expression uses it as a
string. Pure copy never encodes.

**R2.4 Output domain.** The same kinds. Copied input values return as the
value that was carried, unchanged in type. Constructed objects return as an
exported ordered object type that preserves insertion order and implements
`json.Marshaler`, because JSONata object semantics observe order and a Go map
cannot.

**R2.5 No serialization anywhere.** No JSON text is produced or consumed on
the evaluation path. A text convenience may exist as a wrapper over the
native API; it is not the API.

## 3. Operations

**R3.1 Language.** The JSONata 2.1 documentation Core pins.

**R3.2 Suite.** The full jsonata-js 2.2.2 reference suite (2.2 added no
language; its fixtures are regression tests toward the 2.1 documentation),
run through the **public API**, not only the internal evaluator. Today's
suite runs the internal native path and cannot see public-boundary defects;
one such defect (grouped paths through the byte fast path) is live in the
current public executor.

**R3.3 Declared divergences.** Permitted only where Go cannot represent a
reference value, each recorded with reason and pinned to fixture hash as the
current overlay mechanism does. Known at drafting time:

- `$split(s, "")` on strings containing astral characters. The reference
  yields UTF-16 halves (lone surrogates) that a Go string cannot hold.

The current engine's 20 declared deltas are all its exact-decimal arithmetic
policy. Under R4 that policy goes away and those deltas close.

## 4. Numeric model

Go's arithmetic, so that `a + b` in a transform equals `a + b` in Go. Go does
not define mixed-type or token arithmetic, so seven rules complete it:

| | Rule |
| --- | --- |
| R4.1 | A `json.Number` token classifies once before arithmetic: integral and within `int64`, `int64`; integral and within `uint64`, `uint64`; otherwise `float64` if it has a fraction or exponent; otherwise an error (integral, exceeds `uint64`, and not supplied as `*big.Int`). |
| R4.2 | Integer with integer yields integer. Overflow is an error, never a wrap. |
| R4.3 | Any `float64` operand yields `float64`, by Go's conversion of the other operand. |
| R4.4 | `*big.Int` with `*big.Int` yields `*big.Int` via `math/big`. `*big.Int` with `float64` follows R4.3. |
| R4.5 | `/` yields an integer when both operands are integers and the division is exact; otherwise `float64`. (The documentation requires `1/2` to be `0.5`; Go's integer division cannot be used unqualified.) |
| R4.6 | `%` follows Go's `%` for integers and `math.Mod` for floats, which matches the documentation's `x - trunc(x/y) * y`. |
| R4.7 | Non-finite results (`Inf`, `NaN`) are errors, as in the reference. |

`$round` is half-to-even (`math.RoundToEven`); `$sum`, `$average`, `$power`,
`$sqrt`, `$formatNumber`, and the rest follow from the table and Go's `math`.

Consequence stated plainly: `0.1 + 0.2` is `0.30000000000000004`, as it is in
Go, in the reference, and in every JSON system. The current engine's exact
`0.3` is not carried forward.

## 5. String model

Go's, which the documentation agrees with: characters are codepoints (`rune`),
so `$length("😀")` is `1`, matching the reference. Unicode 16 casing tables
stay. The UTF-16 code-unit emulation layer (`jstring`, 46 call sites) is
removed; lone surrogates are not representable and are the single declared
divergence in R3.3.

## 6. Comparison

**R6.1** Equality, `<` `>` `<=` `>=`, `in`, sort keys, and `$type` are decided
by mathematical value across the whole numeric set in R2.1 and by codepoint
content for strings. `9223372036854775807` as a token equals it as an `int64`
equals it as a `*big.Int`. This is the fix for the defect both gnata and the
current engine share, where `items[k=9223372036854775807]` returns records
that do not match.

**R6.2** Comparison never converts through `float64` when both operands are
exact.

## 7. Carriage

A value that an expression only selects, copies, or rearranges is returned as
the same Go value, same type, same identity where the type is a reference.
This holds through object and array constructors, `$merge`, `$sift`, `$map`,
and the transform operator.

## 8. Selective evaluation

**R8.1** A caller may request one field of the result, by path, and receive
it without the engine computing the fields not on that path.

**R8.2** Eligible when the expression's root (after bindings) is an object
constructor with distinct literal keys and the requested field's
subexpression is pure: no `$random`, `$now`, `$millis`, `$eval`, no transform
operator, no partial application, no unknown function. Ineligible
expressions fall back to complete evaluation transparently.

**R8.3** Unobservable: the selected field is identical to the same field of a
complete evaluation. Qualified by witnesses that assert this for every
selectable fixture.

**R8.4** The result reports which mode ran (`selected` or `complete`) and,
on fallback, why, so callers and benchmarks can see it.

**R8.5** Selection composes with the existing input fast path: where a
selected field's subexpression is fast-path eligible, it is answered without
decoding the rest of the input.

**R8.6** Memoized per compiled node within one context, so selecting several
fields shares work; a failed selection is never cached.

## 9. Closed environment

No host functions on the public surface. `CustomFunc`, `NewCustomEnv`,
`EvalWithCustomFuncs`, and `StreamEvaluator.WithCustomFunctions` are removed
from the module. `$eval` is available exactly as the documentation defines
it, over the closed standard library.

## 10. Resource bounds

`context.Context` cancellation honored cooperatively at compile, evaluation,
and function boundaries; limits on expression size, evaluation depth, output
node count, and elapsed time; a compile cache with a bounded entry count.
Exhaustion is an error (§8 of the class floor).

## 11. Errors

Three outcomes, distinguishable by the caller: a value (including `null`), an
absent result (JSONata undefined; not a Go `nil` value), and an error. Errors
carry the reference's code where one exists (`T2001`, `D3137`, …). Refusals
under R4, R6, and R10 are errors with their own codes.

## 12. Public API (requirements, not design)

- `Compile(expression) (*Expression, error)`, with options for limits.
- `(*Expression).Eval(ctx, input any, opts ...) (any, error)` with an option
  for bindings (a Go map, names without `$`).
- `(*Expression).Context(ctx, input any, opts ...)` returning a handle with
  `Select(ctx, path ...) (Selection, error)`, `Complete(ctx) (any, error)`,
  and `Close()`.
- The syntax-only `Validate(expression) error` for OBI-D-18 stays, without
  loading the evaluator.
- No JSON text in any signature above. A `jsonata/text` convenience package
  may wrap them.

## 13. Performance

Benchmark-gated. Single-digit allocations for a simple path lookup on the
native path (today: 90). Complete evaluation over native values no slower
than the current native path. Selective evaluation of one field of an
N-field object costs that field, not N.

## 14. Removed

The text executor and JSON admission (`json_executor.go`, `json_admission.go`,
`executor.go`); `StreamEvaluator`; custom-function environments; the
`jstring` UTF-16 layer; the exact-decimal arithmetic policy and its `apd`
dependency; the 20 arithmetic deltas.

## 15. Open decisions

Each is yours; each has a default I would take absent a ruling.

1. **R4.5, exact integer division.** Default: yes (gojq's rule). Alternative:
   `/` always yields `float64`, which is closer to the reference and simpler.
2. **R4.1, tokens beyond `uint64`.** Default: error. Alternative: promote to
   `*big.Int`, which is still Go (`math/big`) but is arbitrary precision the
   author did not ask for.
3. **R2.3, bytes when the adapter supplies no encoding.** Default: Base64,
   per the catalog. Alternative: refuse to present as a string.
4. **R2.2, accessor shape.** Default: the six-method interface above,
   mirroring the JS `ValueAccess`. Alternative: reflection-based automatic
   admission of structs, which is convenient and slower.
5. **R8.5, fast-path composition.** Default: in scope. Alternative: defer to
   a second pass once R8.1 through R8.4 land.
