# Iteration 2: changes

Triage of the iteration-1 panel (five cold reads of the stub at `feb3743`,
saved under `iteration-1/panel/`; grades in `iteration-1/grades-after.md`).
Reviewer keys: idiom, integrator, security, pl, practitioner.

## Applied

| Change | Finding | Reviewers |
| --- | --- | --- |
| `Limits.MaxBytes` (default 64 MiB) charged before allocation for strings, bytes, encoded forms, big integers, `$eval` sources, regex programs, and Error values; `Limits.MaxIntegerBits` (default 4096) | the work bound counts nodes; `$pad("a",262144)` doubled is two nodes and 64 GiB; big-integer squaring is unbounded | security (demonstrable defect against the headline claim) |
| One budget per Eval or per Evaluation; `$eval` draws from it; parsing charged per byte; Access, BytesEncoding, sort comparisons, and descendant visits charged as work; bounds count work and bytes, not time; ctx checked at intervals inside size-scaling operations | budget scope ambiguous; `$eval` a compile amplifier; long builtins uninterruptible | security, integrator |
| `Access` redesigned: one method `Resolve(ctx, v) (any, error)` returning an admitted value or an `ObjectView`/`ArrayView`, both with ctx and error returns; `Kind` removed; documented as the boundary of the closed environment, allowlist not reflection, charged, concurrent | cannot report failure so guards fail open; nine required methods; no ctx; ordering unstated | security, pl, idiom (idiom's shape, with security's ctx and error; stdlib precedent: optional-interface capability as in `fs.FS`, `io.WriterTo`) |
| Prelude statements evaluated before the first selection; prelude failure fails every selection; sibling `$error`/`$assert` is not a guard under selection, stated; rule 9 stated as one-way | guard bypass via `Select`; rule 9 vacuous when Complete fails | security, pl, practitioner |
| Ownership section: input and Env borrowed until Close; returned values shared with the Evaluation until Close; bindings may appear in results by identity; constructed containers new, members may be carried; concurrent map mutation is fatal, stated | "owned by the caller" contradicted memoization and sharing | security, integrator, pl, idiom |
| Error hygiene: `Error()` renders code, message, position; Message never input-derived; Value charged to bounds; Token capped at 64; unsupported-value errors name the type | exfiltration and amplification through the error channel | security, integrator |
| Nil `[]any` is the empty array, nil `map[string]any` the empty object; only nil pointers are null | `$count(null)` is 1, flipping collection predicates | security, integrator |
| Timestamp fixed at Eval start as well as Prepare | timing oracle; reference semantics | security, idiom, practitioner |
| Integer with integer yields an exact integer, `*big.Int` when `int64` does not fit; refusal for float mixing decided on the result, not the operand | result depended on operand representation, contradicting the value-function rule; refusing representable results | practitioner, pl; **flagged for veto** as a numeric-model change |
| Decimal-token admission stated as the model's representation, with refusal scoped to results | rule 8 read as violated by admission | idiom, pl |
| Division and remainder by zero are D1001 | "Go's %" panics | pl (demonstrable) |
| Invalid UTF-8 strings refused at admission | `$length` had two possible answers | pl (demonstrable ambiguity) |
| `[]byte` compared and ordered by encoded form throughout | mixed comparator inconsistent | security, pl |
| `EvalJSON` encodes bytes with the Env's BytesEncoding and does not HTML-escape; `$string` of compound values specified per the documentation's JSON.stringify reference (no whitespace, no HTML escaping, two-space prettify) | two encodings of one value in one output; `$string` unspecified | idiom, integrator, security, pl, practitioner |
| `Limits` struct with nil-means-defaults on Compile and `Expression.Limits()`; `Env` struct replacing evaluation options and `With`; all functional options removed | bounds must be a comparable, readable value for caching and logging; option precedence undefined; per-call option allocation | idiom, integrator (stdlib precedent: `slog.HandlerOptions`, `tls.Config`; removes three open questions) |
| `text` folded into the package as `Unmarshal` and `EvalJSON` | `Decode` on bytes contradicts `encoding/json` vocabulary; `Evaluate` versus `Eval`; no dependency isolation gained | idiom (stdlib precedent) |
| `Materialize(ctx, v, a)` | results can carry foreign values into encoders; every integrator writes the same walker | integrator (second panel), security (aliasing through Access) |
| Named types with admitted underlying types are admitted | `type ID int64` refused mid-evaluation | integrator, idiom |
| `Reads()` soundness stated and the constructs that make `known` false enumerated; a key applies to array elements | lazy fetch requires soundness | pl, idiom, integrator |
| `Close` defines the end of the borrow and sharing window; Select and Complete after Close return `CodeClosed` | undefined after Close; no defined end to the mutation window | idiom, security, integrator |
| `type Code string` with `Class()`; sentinels and `(*Error).Is`; `Position` unit stated with reason | classification by string prefix; `errors.Is` unusable | idiom, integrator |
| "Closed except the clock, `$random`, and the supplied Access and BytesEncoding" | the closed claim was false as written | pl, security |
| No goroutines, no process-wide state; engine panics recovered into `CodeInternal`; Access panics propagate | headline claim unbacked | security |
| `$fromMillis`/`$toMillis` UTC by default, local zone never consulted; `$random` source named | host-state leak | security |
| `$replace` group syntax per the documentation; `$uppercase("ß")` example; function-valued result is T1006; `??` versus `?:` with a null binding; `Delete` cost; `UnmarshalJSON` replaces and rejects duplicates; map key order named as byte order; object equality order-insensitive; `Mode.String` | unstated behaviors | practitioner, idiom, pl |
| `$round` rounds the binary float64, with the `2.675` example | unstated; two answers possible | practitioner; **flagged for veto**: the documentation is silent on the midpoint basis (it gives ties-to-even and only non-tie examples), so this follows the standing "Go's arithmetic" ruling; the reference's decimal-shift behavior is recorded under ruling 6a |

## Ruled (added to RULINGS.md)

- Regex dialect: escalated. All five reviewers say leaving it unstated is
  the worst state; three endorse RE2 declared with a compile-time code.
- `$round` midpoint basis: evidence for ruling 6a.
- Portable core over map inputs: `$keys` and `$string` are not portable
  unless the caller supplies ordered objects (practitioner, second time).

## Rejected

| Finding | Reason |
| --- | --- |
| Bindings carrying `*Expression` as a shared function library (practitioner) | one reviewer; a feature, not a defect; would reopen the closed-environment discussion |
| Rename `Prepare` to `Begin` (idiom) | one reviewer, held loosely, second panel; `database/sql` precedent adequate |
| `Position` in bytes (idiom) | conflicts with the practitioner's editor-column argument from panel 0; characters kept, reason now stated |
| Ship struct and protobuf `Access` implementations (idiom, integrator) | frozen item 5; named-type admission applied instead |
| Panic recovery is a bug hider (security's caveat) | the alternative is process death, which the headline claim forbids; recovered panics carry `CodeInternal` so they are visible, not hidden |
