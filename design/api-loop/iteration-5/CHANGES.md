# Iteration 5: changes

Triage of the iteration-4 panel (five cold reads of the stub at `e57816b`,
under `iteration-4/panel/`; grades in `iteration-4/grades-after.md`).
Reviewer keys: idiom, integrator, perf (this round's rotating lens), pl,
practitioner. Facts were verified before being declared: the exact
quotient `1758000000000000123 / 1000000` rounds once to the float64 whose
shortest rendering is `1758000000000.0002` (the stub said `.0001`);
`float64(9007199254740994) + 1` under the iteration-4 cap rounds to
`9007199254740996` while the equal int64 yields `9007199254740995`;
jsonata-js 2.1.1 renders `$string(1234567890123456789)` as
`"1234567890123456800"` (the ledger was right; the practitioner's
`toPrecision(15)` claim predates 2.1.1), reads `$0` in a `$replace` string
as the whole match and `$12` with one group as group 1 followed by `2`,
returns null for `$x ?? "d"` with `$x` bound to null, and gives `12` for
both `$round(11.5)` and `$round(12.5)`.

## Applied

| Change | Finding | Reviewers |
| --- | --- | --- |
| The 2^53 integrality cap removed: any integral float64 is the integer it denotes (`1e23 + 1` is `99999999999999991611393`); the decode boundary stated as where a decimal's precision is fixed (`9007199254740993.0 = 9007199254740993` is false because the left side is the float64 `9007199254740992`); `$string` of an integral value renders digits, so the notation thresholds apply to decimals only | with the cap, equal values gave unequal results under `+ 1`, violating the doc's own "never of the representation"; "the mathematical value of the token" was claimed where the model fixes precision at decode | pl (demonstrable), **flagged for veto** as a numeric-model change; reverses iteration 4's cap (js, practitioner), which was optional in their proposals |
| The `/` example corrected to `1758000000000.0002` | the stated rendering was not the shortest rendering of any float64 | pl (verified) |
| The `x / 1e9` versus `x * 1e-9` asymmetry stated in one line | an undeclared consequence of the refusal rule | practitioner (the fallback the reviewer asked for; the rule itself is ruling 8b) |
| `$replace`: `$0`, the longest-prefix rule for `$NN`, empty for no such group | unstated; the reference's behavior is documented-adjacent and verifiable | practitioner (verified) |
| `?? ` falls through on absent only; a nil binding is null; no way to bind absent | unstated, and the binding case is the one a Go caller hits | practitioner (verified), pl |
| Carriage is not a read: neither classifies nor validates | "first read" undefined; if carriage validated, the rename path became O(bytes) | perf (a behavior the API forces but did not state) |
| `json.RawMessage` decoded on each read, charged per byte; nil or empty is malformed | "first read" promised a cache a `[]byte` cannot hold | perf, integrator |
| The clock fixed at most once and only when observable; `Env.Now` not called otherwise | a vDSO call and a 24-byte `time.Time` per Eval for expressions that cannot observe it | perf (unobservable rewording) |
| Cancellation checked at intervals bounded by `MaxWork` units | "between operations" as written is a mutex per node | perf |
| Resolver charges the strings and bytes the engine observes through a view; does not memoize; a Resolver may memoize by identity | "the bytes it returns" is unmeasurable for a view; "once per read" forbade a memo nobody needs the engine to own | perf, integrator |
| "Before the allocation" qualified: before each growth of a growing buffer | unimplementable as literally read for incremental builders | perf |
| The plan fixed at Compile except `ReasonArrayInput`; `Expression.Fields()` reports the static keys; `Prepare` runs no prelude; `Select` of an unknown key versus a field with no value distinguished by `Fields` | planning per `Prepare` was per-call work for a static property; a misspelled key was indistinguishable from an absent field at config-load time | perf, integrator (third panel asking for a static view) |
| Post-error `Evaluation` semantics: budget errors persist, `CodeInternal` closes, language errors are per field, `Complete` fails when any field would; a waiter's ctx returns while the owner's computation continues; budget exhaustion may differ under concurrency | unstated | idiom, integrator, perf |
| `Close` reworded: the borrow ends when Close returns and any in-flight call completes | "a call in flight completes" implied Close waits | perf |
| `Unmarshal` doc matches the classification rule (integer tokens versus tokens with a fraction or exponent); bounded by `DefaultLimits` depth and bytes; `EvalJSON` decodes under the expression's Limits; `Unmarshal` copies once and results may share the copy | `Unmarshal` said "integral numbers as int64", contradicting "a token with a fraction is float64"; nesting was unbounded; memory retention was unstated | idiom (contradiction), integrator (unbounded resource), perf |
| `Marshal` has no trailing newline; float32 rendering also applies to `$string`; `MustCompile` accepts nil; a Resolver that does not recognize a value returns an error; views are never passed to `Resolve`; `(nil, true, nil)` from a view is null and an error takes precedence over ok; `Error.Value` for `$error`/`$assert` is the supplied message and `Error()` does not render it; binding names are what the language accepts after `$`; `Object` nil-receiver reads, `Map` cost; `Member`'s two arms plus reflection; `Reads` includes predicate paths, an empty non-nil list for no reads, and bindings no longer make it unknown; cache on `Expression.Limits()`; sub-constructor `Select` reads through views and is charged; `ReasonArrayInput` counts a view; sentinel `Error()` text; `Int64` and `Float64` | the "unclear" lists, and a `Reads` rule that was conservatism without a reason | idiom, integrator, pl, practitioner |
| The process-survival claim narrowed to the calling goroutine and bounded allocation, with the caller's own mutation and Resolver panics excluded | "cannot terminate the process" is stronger than Go can promise | idiom |
| `Int64(v)` and `Float64(v)` | every integrator writes the ten-case switch and gets `*big.Int` or `json.Number` wrong; asked in three consecutive panels by the integrator lens | integrator (third time), idiom (normalization need; the fuller answer is ruling 9) |
| "A declared divergence" phrases replaced by pointers to the ledger; the ledger's pending marker defined as not a declaration | doc said "declared" where the ledger said "pending" | pl (contradiction), idiom |
| Conformance paragraph: departures from the reference suite "or any other reference behavior this package has found" | "observable behavior" alone is not enumerable; the README's rule 6 says suite | pl (reverses iteration 4's wording, from the previous pl reviewer; the README is frozen and says suite) |
| Ledger: an **incorporated** authority class; the `$string` row split into non-integral (incorporated) and integral-in-the-reference (value model); the `$round` row notes the docs' ties are exactly representable; rows added for integral float64 arithmetic beyond 2^53 and for ill-formed strings | the `$string` row cited the documentation for an algorithm it applies over a different value domain; the `$round` row's "only non-tie examples" was false; two undeclared divergences | pl, practitioner |
| `ExampleExpression_Eval_languageError` | no example showed the class a handler hits most | integrator |

## Ruled (added to RULINGS.md)

- Ruling 3, `Evaluation` concurrency: the performance engineer's costing is
  the strongest argument yet for single-goroutine by rule.
- Ruling 7, `Close`: a second idiom reviewer argues for deletion.
- Ruling 8, the numeric model: 8b (refuse versus round once) now has three
  reviewers across two panels; 8a gains the practitioner's decimal-spelling
  argument and the verified tie examples; 8c is updated for the cap's
  removal.
- Ruling 9 (new): the typed exit, normalizing `Materialize`, `Canonical`,
  or `Decode` versus helpers.
- Rulings 1, 4, 5, 6: evidence added; 6 gains the authority ladder, rule 3
  vacuity, and rule 1 versus rule 6 circularity.

## Rejected

| Finding | Reason |
| --- | --- |
| Split `Limits` into parse constants and a per-Env `Budget` (idiom) | one reviewer; the performance engineer's keep list defends compile-time Limits as the thing that makes every per-call resolution per-compile; the iteration-2 decision stands |
| Delete `NoInput` (idiom) | one reviewer, against the practitioner's iteration-3 request; the language state exists; both sides recorded |
| Delete `Close` (idiom) | ruling 7 |
| Drop concurrent safety on `Evaluation` (perf) | ruling 3 |
| Fold `Materialize` into `Marshal` (perf; integrator in iteration 3) | re-triaged as a re-raise by a different lens: still rejected, because a view can be unbounded and `Marshal` has no ctx or Limits; the double walk is removed instead by `EvalJSON` doing one pass |
| `Expression.UnmarshalInput` skipping members outside `Reads` (perf) | a feature, one reviewer; recorded for after the loop as the largest remaining lever |
| Normalizing `Materialize` or a `Canonical`/`Decode` (idiom, integrator) | ruling 9; `Int64`/`Float64` applied as non-prejudging helpers |
| Trim sentinels to three; delete `Encoder` for `AppendJSON`; delete `Plan.Selective` and `Member` (idiom) | one reviewer; the sentinels were requested by the integrator lens; `AppendJSON` recorded as a possible addition, not a replacement |
| `$eval` knob (integrator, third time, same lens) | still one lens; the security lens sits on the iteration-5 panel and decides whether to re-raise it |
| Ship proto and struct Resolvers (integrator) | ruling 5 |
| `$round` on the decimal spelling; remove E1008 (practitioner) | ruling 8 |
| Regex wording (practitioner, pl) | ruling 4 |
| README ladder, rules 1, 3, 6, 7, 9, portable core (pl, practitioner, idiom) | ruling 6; the README is frozen |
| The ledger's `$string(1234567890123456789)` example is wrong (practitioner) | verified against jsonata-js 2.1.1: the ledger is right |
| Sequence-versus-array survival, `items[1.5]`, `$number` grammar, `$sum([])`, picture strings, undefined in `=` (practitioner) | language semantics owed to the suite under rule 1; noted for `suite/` |
