# Iteration 3: changes

Triage of the iteration-2 panel (five cold reads of the stub at `4cb2f5d`,
under `iteration-2/panel/`; grades in `iteration-2/grades-after.md`).
Reviewer keys: idiom, integrator, writer, pl, practitioner. Several changes
this round correct defects introduced in iteration 2's new prose; those are
marked **(iteration-2 defect)**. Reference behaviors cited by reviewers were
verified against jsonata-js 2.1.1 before being declared.

## Applied

| Change | Finding | Reviewers |
| --- | --- | --- |
| Float-mixing rule restated: an integer operand must convert to float64 exactly; two-integer inexact division computes the exact quotient rounded once | as worded, the rule refused `0.1 + 0.2` and the `9007199254740993 / 2` example was false **(iteration-2 defect)** | pl, practitioner |
| `uint` and `uint64` share the rule; `uint` is 64-bit | rule false on every target platform **(iteration-2 defect)** | idiom |
| `-0` rendered as `0` attributed to JSON.stringify, not `encoding/json` | `encoding/json` renders `-0` **(iteration-2 defect)** | pl (verified) |
| `ReasonArrayInput` reworded: the constructor groups over an array input | described map-over semantics JSONata does not have **(iteration-2 defect)** | pl, practitioner (verified: `{"a": x}` over `[{x:1},{x:2}]` is `{"a":[1,2]}`) |
| `Class()` table-driven; `ClassUnknown` added | "derived from form" false for `$error`'s D code **(iteration-2 defect)** | pl, idiom |
| Malformed `json.Number` is `CodeUnsupportedValue`, not D3030 | admission fault misclassified as a language cast error | pl |
| `$string` of a function is `""`; a function-valued result is `CodeUnsupportedValue` | docs say functions stringify to empty; T1006 was the wrong code | practitioner (verified in docs) |
| Full Unicode case mapping | "simple mapping" was the host's cheapest function, not the host; reference and every user expect `STRASSE` | pl, practitioner (verified) |
| Tail calls eliminated and not counted by `MaxRecursion` | documented behavior; a suite fixture likely depends on it | pl, practitioner (verified in docs) |
| `$fromMillis`/`$now` timezone via the documented `±HHMM` argument | wording attributed it to the picture | practitioner (verified in docs) |
| Integer-domain function list | undefined which functions stay exact | pl, practitioner |
| `[]byte` equality and order by content; bytes versus string ordering is T2010 | base64 alphabets are not order-preserving, so ordering depended on `Env` | pl, security (panel 1) |
| Results never contain views; a carried foreign value returns as the original | four-arm switch; views reaching encoders | pl, idiom, integrator |
| `Resolver` called once per read; errors wrapped so `errors.Is` sees them; ctx errors as `ctx.Err()` | "once per value" needs identity foreign values lack; error handling unspecified | idiom, pl, integrator |
| `Access` renamed `Resolver` | single-method interfaces are named by method | idiom (stdlib precedent) |
| `Materialize` takes `*Limits` | unbounded walk under expression control | integrator, idiom |
| `[]T` and `map[string]T` of admitted `T` admitted; `json.RawMessage` decoded as JSON | `[]string` refused; `RawMessage` rendered as base64 | idiom, integrator |
| `Marshal` and `NewEncoder` exported | `encoding/json` over a native result disagreed with `EvalJSON` on escaping and bytes | integrator, writer, idiom |
| `Plan`: `Mode` removed, `Selective()`, `Fields()` as iterator; `Reads()` returns fresh slices | `Mode` was a function of `Reason` with inconsistent zero values; shared slices | idiom |
| `Error`: `Offset` in bytes plus `Line`/`Column`; `Position` removed | every stdlib position is bytes; the editor-column rationale was wrong (LSP is UTF-16) | idiom (stdlib precedent; reverses an iteration-1 choice on the strength of the factual correction) |
| `Env.Now` | no way to pin the clock in tests | idiom (`tls.Config.Time` precedent) |
| `MaxOutputNodes` counts a carried value once | ambiguity between O(1) carriage and the bound | idiom, integrator |
| `Select` descending into an array yields absent | undefined | idiom, integrator, pl |
| `Close` idempotent, stated | unstated | integrator |
| `present` discard warning; `Compile`/`Eval`/`Prepare` say nil means defaults; `EvalJSON` absent is `out == nil`; `EvalJSON` materializes before encoding | first-call guesses | writer, idiom |
| Package doc reordered to the writer's outline; first-call and selection code blocks; `admitted`, `foreign`, `carried`, `prelude` defined before use; the `UseNumber` trap stated; duplicated statements removed; status line; documentation URL; `DIVERGENCES.md` named | structure was a specification a reader had to reverse-engineer | writer |
| Example functions | pkg.go.dev's Examples section was empty | writer |
| Synopsis says the member defines a value model built from Go's types, not that Go defines it | "Go defines how they combine" contradicted the Numbers section | writer, idiom, pl |
| Declared divergences recorded in `DIVERGENCES.md`: `$string` float rendering, `$round` midpoint, integers above 2^53, mixed arithmetic, integer-like key order, code points | the README promises a member ledger and none existed; several divergences were now known and verified | pl, practitioner, writer |

## Ruled (added to RULINGS.md)

- Regex dialect: third panel running, all five reviewers; ruling 4 is now
  the one item on which the loop is blocked.
- Output object type: the idiom reviewer now argues from the README's own
  rules 2 and 6 that `*Object` should not exist; recorded under ruling 1.
- `Close`: the idiom reviewer argues for deletion; three panel-1 reviewers
  wanted it kept without an error. Recorded under a new ruling 7.
- `Evaluation` concurrency: unchanged, ruling 3.

## Rejected

| Finding | Reason |
| --- | --- |
| Numeric collapse helpers `Int64`/`Float64`/`BigInt` (integrator) | one reviewer; belongs to the SDK's value handling, not the evaluator |
| Static `Expression.Plan()` and `Env.Validate()` (integrator) | one reviewer; `Prepare` reports both; revisit if re-raised |
| `Limits.NoEval` (integrator) | one reviewer; a real knob for document-supplied expressions, recorded for re-raise by the security lens |
| `Usage()` readout (integrator) | one reviewer; additive later |
| Move Numbers and Strings out of the package doc into a README (idiom) | conflicts with the writer, who wants them in the rendered doc; pkg.go.dev does not render a sibling README |
| Rename `Prepare`/`Complete` (idiom) | third time, one reviewer each time; precedent adequate |
| Drop `*Object` (idiom) | frozen item 1; argument recorded |
| Delete `Close` (idiom) | conflicts with panel 1; recorded as ruling 7 |
