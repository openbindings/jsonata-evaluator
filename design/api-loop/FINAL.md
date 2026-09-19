# The API loop: outcome

The loop defined in `LOOP.md` ran six iterations on 2026-09-18 and stopped
on its cap. It did not converge: every panel produced Apply-bin findings,
and the grade floor on the loop-controlled rows never rose above B-. The
grades that fell short cite, in every panel, items the loop was not allowed
to decide, so the practical outcome is the ruling queue in `RULINGS.md`
plus the stub as it stands at the final commit.

## Grade trajectory (medians)

| Panel | Go fit | Ergonomics | Correctness | Performance | Concept | Overall |
| --- | --- | --- | --- | --- | --- | --- |
| pre-loop | B+ | B | C | B+ | B | B- |
| iteration 1 | B+ | B | C+ | A- | B | B |
| iteration 2 | B+ | B- | C+ | A- | B+ | B |
| iteration 3 | B+ | B | B- | A- | B | B |
| iteration 4 | B+ | B | B- | A- | B+ | B+ |
| iteration 5 | B+ | B | B- | A- | B | B |
| iteration 6 | B+ | B | C+ | A- | B | B- |

Performance rose to A- in the first iteration and stayed there. Go fit was
B+ throughout. Ergonomics stayed at B, deducted every round for the
`map[string]any` versus `*Object` result split (ruling 1) and the
`(value, present, err)` triple, which the idiom lens attacked in three
panels and the practitioner and PL lenses defended in two. Correctness
rose from C to B- as iterations 2 through 5 closed the bounds, ownership,
error-surface, and admission gaps, and fell back to C+ in the last panel
on the numeric model (ruling 8c below).

## What the loop settled

Everything in the stub that no panel re-raised after it was applied. In
rough order of weight:

- The lifecycle: `Compile` once, immutable and concurrent-safe
  `Expression`, `Limits` baked into it and readable back, `MustCompile`,
  `String()`, `ctx` on every evaluation and none on `Compile`, `ctx.Err()`
  returned bare.
- The value boundary: inputs by reference, admission per value on first
  read, carriage by identity, carriage neither classifies nor validates,
  nil collections as empty, exact type before structure, `[]byte` as
  bytes, `json.Number` and `json.RawMessage` admitted with their costs
  stated, foreign values through a single-method `Resolver` and two view
  interfaces.
- The closed environment: values-only bindings that shadow built-ins, no
  function registration, the clock fixed once and only when observable,
  `$random` named, UTC by default, `$eval` inside the budget.
- Bounds: every bound finite by default, comparable `Limits`, work charged
  per byte visited so `MaxWork` bounds processor time, `MaxDepth` on every
  value walk, `MaxBytes` cumulative, integer tokens bounded before parsing,
  budgets raised before allocation, cancellation at least every 1024 units,
  engine panics recovered and caller panics propagated.
- Errors: `*Error` with codes classified by table, sentinels of an
  unexported type, `Message` free of input content, `Value` charged and
  flagged for redaction, engine refusals under E codes, `Unwrap` for the
  Resolver's cause.
- Selection: prelude once, one budget, plan fixed at Compile except the
  array-root case, `Expression.Fields` for validation before input exists,
  `Select` not a guard with the plan-dependence stated, post-error
  semantics stated.
- Strings: code points, full case mapping, stable sort, `$replace` group
  rules as the reference does them, bytes encoded only when observed and
  never by default.
- The ledger: fifteen rows, each with an authority class, two marked
  pending.

## What the loop could not settle

The ruling queue, in `RULINGS.md`. The blocking item since iteration 1 is
the regex dialect (ruling 4), which every one of thirty reviewers across
six panels flagged. The numeric model (ruling 8) accumulated the most
argument: refusal versus rounding once for mixed arithmetic (8b, four
reviewers across three panels for one rule), the `$round` tie basis (8a,
now with the "the number the package prints is the number it should round"
argument from two lenses), and the integrality boundary (8c, three
positions after iteration 6). The class README (ruling 6) drew the same
finding from every PL reviewer and, by iteration 5, from every other lens:
the Go doc's numeric and string choices are class-level decisions the
README attributes to "the host", and a second member cannot be started
from the README alone.

## The iteration-7 Apply bin

Findings from the iteration-6 panel that pass the triage test and would
have been applied had the loop continued. Recorded so they are not lost;
none is applied, because the cap is the cap.

| Finding | Reviewers |
| --- | --- |
| `Reads` is unsound for a bare `$` or `$$` outside a member path (`$` alone reports an empty list with `known` true); such a reference must make `known` false | pl (demonstrable) |
| `BytesEncoding` must be injective, stated as a requirement; equality, ordering, and `$string` of bytes are consistent only then | pl |
| `Error.Offset` unknown is -1 while `Line`/`Column` unknown are 0; one convention | writer, idiom |
| "Every package-level function runs under DefaultLimits" overclaims; `Member`, `Int64`, `Float64`, `NewObject` do not | writer |
| "class" and "binding" used in the Go doc in the README's senses, colliding with the `Class` type and `Env.Bindings` | writer |
| `Eval` and `Limits.Eval` should warn that a `[]byte` input is a byte string, not JSON text | writer (a bug a reader will ship) |
| The quick start's `jsonata.Marshal(out, nil)` runs under `DefaultLimits`, not the expression's; lead with `EvalJSON`/`EvalMarshal` | writer, integrator |
| Examples need `// Output:` lines; add `ExampleExpression_EvalJSON`, `ExampleEnv`, `ExampleMember` | writer |
| `ReasonArrayInput`'s stated reason does not establish non-selectability; the real reason is that an empty array root yields `{}` | pl |
| `Object.MarshalJSON` through `encoding/json` re-escapes `<`, `>`, `&`, U+2028/9 and cannot see an `Env`, so a `[]byte` member fails; state both | idiom |
| `Marshal` renders a carried `json.Number` as its token but a float64 as its value; state the asymmetry | pl |
| `$base64encode` of a `[]byte` "encodes the bytes themselves" contradicts "a string for every purpose that observes its content"; bytes are a third domain with two representation-observing operations | pl |
| `MaxIntegerBits` "bounds the magnitude" but is enforced on digit count; say which | pl |
| Full case mapping is not Go's stdlib (`strings.ToUpper("straße")` is `"STRAßE"`, verified); declare the dependency | pl |
| `Plan.Fields` returns `[]string` while `Expression.Fields` returns `([]string, bool)`; align | idiom |
| `Class` has no `String()`; `Reason` does | idiom |
| The package doc's structure: value-model sections last and shortened, a vocabulary section defining admitted, foreign, view, carried, observed, and order-observing once, each promise stated once on its owning symbol, the "reference implementation does X" sentences moved to the ledger | writer, idiom (the largest deferred item) |
| The ledger's shape for a transform author: open questions first, grouped by area, plain-language "why", Go names demoted, a stable heading per row | writer |

Findings rejected on verification: the practitioner's claim that
jsonata-js renders `$string(1234567890123456789)` as `"1234567890123460000"`
(2.1.1 renders `"1234567890123456800"`, verified in iteration 5) and that
the range cap D2014 is 1e7 (the 2.1.1 message says 1e6, verified in
iteration 4).

Findings routed to rulings: `Undefined` sentinel replacing the triple
(idiom, third panel; defended by pl and practitioner); `Unmarshal` renamed
`Decode` with `NewDecoder` (idiom); recovering Resolver panics into an error
(integrator; against the idiom lens's keep list); `Marshal` of a carried
`float32` at 32-bit shortest digits (integrator; ruling 10 material);
splitting E1001 by caller action and moving duplicate-name and
surrogate-escape refusal to a declared decoder stance (pl; ruling 11);
`Divergences()` per expression or a portable-core lint (practitioner,
integrator, three panels; the post-loop feature that ruling 6e makes
possible); the decimal decode boundary and the integrality rule (idiom,
pl, practitioner; ruling 8c).

## Landing

Per `LOOP.md`: one pull request from `design/api-loop` to `main`,
squash-merged with the branch deleted; each open ruling filed as an issue
on this repository and referenced from `RULINGS.md`; the stub keeps its
current answer on every ruled item until a ruling lands.
