# Iteration 1: changes

Triage of the 2026-09-18 panel (five reads of the pre-panel stub, saved
under `design/panel-2026-09-18/`). Each applied change names the finding and
the reviewers behind it. Reviewer keys: idiom, integrator, perf, pl,
practitioner.

## Applied

| Change | Finding | Reviewers |
| --- | --- | --- |
| Package moved to `go/jsonata/`; `go/doc.go` removed | import path ended in the keyword `go` | idiom, integrator |
| `# Numbers` section: value function, admission widening, `json.Number` classification, literal typing, three result types, exact division rule, `$round`, `$string` formatting | numeric model unspecified; literal typing decides whether `[id = 9007199254740993]` works | all five; **flagged for veto** as a consequence of the standing "Go's arithmetic" ruling |
| Integer operand not exactly representable as `float64` is refused (D1001), not rounded | rule 8 contradicted "any float64 operand yields float64" | idiom, pl, practitioner |
| `map[string]any` keys observed in sorted order; `*Object` insertion order; stated | nondeterministic `$keys`/`$merge`/`$sift` | idiom, integrator, pl, practitioner |
| Non-finite `float64` input refused at admission | rule 8 covered results only | pl, idiom |
| Carried-versus-constructed object types stated with the two-arm switch | forced behavior not stated | idiom, integrator, practitioner, pl (the choice itself is frozen; only the statement is applied) |
| Ownership and aliasing paragraph; `WithBindings` by reference | missing ownership statement | idiom, integrator |
| Typed nil pointers, slices, maps are null | forced behavior not stated | integrator |
| Bounds moved to `CompileOption` with stated finite defaults; `WithMaxWork` added; "no expression can terminate the process" | unstated defaults, no intermediate budget, process death | perf, integrator, idiom (two would block release) |
| ctx errors returned as `ctx.Err()`; tuple zero on error; stated | `errors.Is` had no defined behavior | idiom, integrator, pl |
| `Close()` returns nothing; GC reclaims an unclosed `Evaluation` | `Close() error` with nothing to fail | idiom, perf, integrator |
| `Access.Number` returns `(any, error)`; `Keys` replaced by `Range`; `Get` ok==false is absence; foreign values route back | cannot fail; allocates per visit; absence undefined | idiom, perf, pl, integrator |
| `Object`: zero value usable, sized `NewObject`, `Keys()`/`All()` iterators, `UnmarshalJSON`, `Get` O(1) stated, `Map` shallow stated | pre-1.23 `Range`, zero value unusable, asymmetric marshal, complexity unstated | idiom, perf, integrator |
| `String()` replaces `Source()` | `regexp.Regexp.String` precedent | idiom |
| `Reason` typed with constants and `String()`; `ModeSelective`/`ModeComplete` prefixed | magic strings; constant collided with method `Complete` | idiom, integrator |
| `(*Expression).With(opts...)` | per-request option allocation | perf, integrator |
| `Reads()` returns `[][]string`, computed at Compile, shared | dotted-string ambiguity; per-call allocation | idiom, pl, perf |
| `Prepare` fixes the `$now`/`$millis` timestamp; block preludes selectable with the prelude shared; duplicate literal keys fail as Complete would; rule 9 stated as "when Complete would succeed"; empty path is Complete; unknown key is absent | selection semantics unstated or vacuous | perf, practitioner, pl, idiom |
| `$eval` compiles under the enclosing bounds | unstated | integrator, practitioner |
| `text.Evaluate` takes a compiled `*Expression`, returns `([]byte, bool, error)`; `text.Decode` exposed and decodes objects to `*Object` | recompiles per call; absent indistinguishable; order lost | integrator, pl, practitioner |
| `Version` and `DocumentationCommit` constants | caller cannot verify the pinned language | pl, integrator |
| Overflow and non-finite under the language's `D1001`; engine codes renumbered | handlers key on D1001 | practitioner (idiom's separate-namespace preference yields to caller precedent) |
| `Error` gains `Line`, `Column`, `Value`; `Position` in characters | byte offsets misalign with editors; `$error` payloads | practitioner, integrator |
| Regex literals compiled at `Compile`; `$match.index` in code points; string semantics section (code points, ordering, casing, `[]byte` under `$type` and equality) | regex failure timing and `index` unit unstated; `[]byte` semantics unstated | practitioner, integrator, idiom |

## Ruled (added to RULINGS.md)

The six frozen items, plus new evidence from this panel: the `$round(1.015, 2)`
midpoint case, the `$number` grammar case, and the reference's numeric-key
ordering for `$keys`, all of which turn on the authority tiebreaker.

## Rejected

| Finding | Reason |
| --- | --- |
| Rename `Prepare` (idiom, held loosely) | `database/sql` precedent is adequate; one reviewer |
| `Object` as an interface (idiom) | no caller has asked; more surface for no consumer |
| A compile cache in the library (integrator) | the performance engineer's argument against it is stronger: a global cache keyed by attacker-chosen strings is an unbounded-memory surface |
| `Field` handles for `Select` (perf) | frozen item 2; recorded in RULINGS with the allocation argument |
| Single-goroutine `Evaluation` (perf, integrator) | frozen item 3; recorded in RULINGS with the lock-cost argument |
| Uniform `map[string]any` output (idiom) | frozen item 1; the split is now documented, the choice is not made |
| `WithAdmittedOutput` / `Decode(v, dst)` materializers (integrator) | one reviewer; foreign values in output follow from carriage-by-identity, which is a stated commitment; revisit if a second panel raises it |
| Ship `Access` implementations (integrator) | frozen item 5 |
