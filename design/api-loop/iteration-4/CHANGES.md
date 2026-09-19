# Iteration 4: changes

Triage of the iteration-3 panel (five cold reads of the stub at `b501e30`,
under `iteration-3/panel/`; grades in `iteration-3/grades-after.md`).
Reviewer keys: idiom, integrator, js (the JavaScript member's author, this
round's rotating lens), pl, practitioner. Facts the changes rest on were
verified before being declared: `encoding/json` renders float64 with
`JSON.stringify`'s notation thresholds (`1e6` as `1000000`, `1e21` as
`1e+21`, `1e-7` as `1e-7`) but escapes U+2028; jsonata-js 2.1.1 binds
caller bindings in a child frame of the built-ins frame, so they shadow;
its range cap D2014 is 1e6 and `$power` overflow is D3061.

## Applied

| Change | Finding | Reviewers |
| --- | --- | --- |
| Integrality by value: a float64 that is integral and of magnitude at most 2^53 takes part in arithmetic and comparison as that integer, so `x / 100.0` is `x / 100` and `json.Number("1.0") + 9007199254740993` is exact; the exact-integer-versus-decimal comparison stated | the rule was keyed to spelling and contradicted "never of the representation"; a JavaScript member cannot implement a spelling-keyed rule | js, pl, practitioner; **flagged for veto** as a numeric-model change (the 2^53 cap keeps `1e300` a decimal) |
| The operand-versus-result principle stated: an operand is never converted inexactly; a non-integral result of exact operands is rounded once because the language defines division to yield fractions | `+` refused and `/` rounded with no stated principle | pl (the alternative, round once everywhere, is recorded under ruling 8) |
| Inexact conversion is `CodeInexact` (E1008); `MaxIntegerBits` overflow is `CodeBudget`; `Code` doc states that a language code is raised only where the language defines it and that `ClassEngine` identifies member-specific behavior | engine refusals wore language codes, defeating `Class()`; every other `Limits` field yields E1002 | pl (a contradiction with the `Class` doc's own definition) |
| `$string` of a float64 pinned to `JSON.stringify`'s notation: plain from 1e-6 up to 1e21, unpadded exponent otherwise, with `encoding/json` named as matching and `strconv`'s 'g' as not | "shortest round-trip" fixes digits, not notation; `$string(1e6)` was at risk of `"1e+06"` | js, pl, practitioner (verified) |
| U+2028 and U+2029 unescaped in `$string` and `Marshal` | `encoding/json` escapes them; `JSON.stringify` does not | js (verified) |
| `BytesEncoding` nil means none; observing a `[]byte`'s content without one is `CodeUnsupportedValue` | the stub's base64 default contradicted the README's "never chooses the encoding itself" | js (README contradiction; the README is the frozen authority, so the stub yields) |
| `[]byte` ordered by its encoding, against strings and against other `[]byte`; T2010 for bytes-versus-string removed | "is a string for every purpose" contradicted "ordering against a string is an error"; rule 5 forbids ordering by host type | pl, practitioner (reverses the iteration-3 by-bytes ordering on the strength of the contradiction; equality by bytes agrees with equality of encodings) |
| `Unmarshal` and `Object.UnmarshalJSON` decode to int64, `*big.Int`, and float64; a `json.Number` is classified on each read | "classified once" is impossible: a `json.Number` is a string with no identity and the containers may not be mutated | idiom (demonstrable), integrator (asked how) |
| `CodeMalformedInput` (E1006) with `Offset` into the input; `Unmarshal` accepts surrounding whitespace and top-level scalars; an unpaired-surrogate escape is malformed | bad input text was an "unsupported value" and indistinguishable from a Resolver gap; `Offset` was defined only for the expression | idiom, integrator, pl |
| `CodeResolver` (E1007) wrapping the Resolver's error; `(*Error).Unwrap` | "wrapped" named no code | idiom, integrator |
| `Prepare` takes ctx and classifies the root through the Resolver | `ReasonArrayInput` needs the root classified, which for a foreign root needs the Resolver and ctx | pl (demonstrable inconsistency) |
| Sentinels are an unexported type; `(*Error).Is` matches sentinels only; the `errors.Is(err, &Error{Code: ...})` recommendation replaced by `errors.As` | `ErrBudget.(*Error).Code = ""` compiled; the throwaway-struct target is not an idiom | idiom (`io.EOF` precedent) |
| "Integers are exact up to Limits.MaxIntegerBits" | "at any size" contradicted the bound | idiom |
| `Close` doc reconciled with Ownership: returned values may still alias the input and bindings | `Close` said "may be mutated"; Ownership said results alias shared bindings | integrator (contradiction) |
| `Materialize(ctx, v, env, limits)`; `EvalJSON` documented as Unmarshal, Materialize, Marshal; `Materialize` returns a new container only where one contained a foreign value | callers pulled the Resolver back out of the Env they already held; identity of a materialized map was undefined | integrator, idiom |
| `[]byte` classified by exact type before the `[]T` rule; other named `[]byte` types are bytes; `json.RawMessage` decodes; nil `[]byte` is empty bytes | `[]uint8` was both bytes and a number array | pl, integrator, practitioner |
| `Member(v, key)` reading a map, `*Object`, or `map[string]T` | every consumer writes the two-arm switch | idiom (`Lookup`), integrator (`Member`); does not prejudge ruling 1 |
| `NoInput` for evaluating with `$` absent | the language state exists and the API could not express it | practitioner (demonstrable; the reference's `evaluate()` with no argument) |
| `ReasonImpure` renamed `ReasonOpaque`; the planner's allowlist stated as this package's and growable | `$eval` is deterministic; "impure" was false for one of its two members; `Plan` stability across versions unstated | pl |
| `$sum` left to right; `$sort` stable; `$sqrt` correctly rounded; `$power` with a non-integer exponent is `math.Pow` and may differ across hosts in the last unit | unstated; two members would silently disagree | js |
| Bindings shadow built-ins, as the reference does | unstated | idiom (verified in jsonata-js 2.1.1) |
| Views admitted wherever they appear; foreign values in bindings and inside admitted containers resolved on read; `Get` and `Range` must agree; Resolver-returned bytes charged to `MaxBytes` | unstated | integrator |
| `Encoder` reusable and may write partial output on error; `Error.Value` visible to loggers that walk the struct; `Limits` comparable; `MaxExpressionBytes` in UTF-8 bytes per `$eval` source; `Complete` returns selected values by identity; `Reason.String` names listed; `Compile` failure form; zero `Env` equals nil; `Env.Now` rendered in UTC; `String()` verbatim; `Prepare` reports binding errors; `Reads` paths nil when unknown; `Fields` empty when not selective; a concurrent `Select` of a field in progress waits; `Close` lets an in-flight call complete; an invalid-UTF-8 `Object` key is refused on read; carried values are not charged to `MaxBytes` | the "unclear" lists | idiom, integrator |
| Ledger: rows carry an authority class (documentation, interpretation, value model, engine); `$round` row rewritten as an interpretation of text that speaks; `$uppercase` row removed; rows added for string ordering, duplicate member names, `$power`, the range cap, and function-valued results | four rows cited "silence" for documentation-over-reference; a non-divergence sat in the ledger; four undeclared divergences found by reading the stub | pl, practitioner, js |

## Ruled (added to RULINGS.md)

- Ruling 4, regex dialect: fourth panel, all five reviewers; blocked.
- Ruling 6, the class README: four reviewers converge that the README
  attributes to "the host" decisions the Go doc made for the class; 6e
  (the profile) escalated from "defer" to "required before a second
  member"; new sub-items 6g through 6m.
- Ruling 8 (new): the numeric model's two flagged vetoes become a ruling
  with the arguments: `$round` basis, refuse versus round-once, the 2^53
  integrality cap.
- Rulings 1, 3, 5: evidence added.

## Rejected

| Finding | Reason |
| --- | --- |
| Remove the `nil` from `Compile`/`Eval`; `Limits` as a receiver (idiom) | one reviewer; the nil-options shape was chosen in iteration 2 on two reviewers with `slog.HandlerOptions` and `tls.Config` precedent; recorded for re-raise by another lens |
| Flatten `Plan` onto `Evaluation` (idiom) | one reviewer; `Plan` was reshaped last iteration at the same lens's request |
| Rename `Prepare` to `Begin` (idiom) | fourth consecutive panel, the same lens each time; the `sql.DB.Begin` argument is the best so far and is recorded; naming is not re-triaged on one lens |
| `Select` with a required first key (idiom) | one reviewer; ruling 2 (`Field` handles) would moot it |
| `Marshal` resolving foreign values through the Resolver (integrator) | no ctx and no budget on `Marshal`; an unbounded walk under expression control is why `Materialize` got limits in iteration 2; `Materialize` takes the Env instead |
| `Int64` collapse and `Decode` helpers (integrator, second time) | the numeric union narrowed with the `Unmarshal` change; `Member` applied; `Int64` still one reviewer |
| `Error.Path` into the input (integrator) | one reviewer; a tracking cost on every read; recorded |
| `MaxBytes` default 8 MiB (integrator) | one reviewer; a number, not a defect |
| An executable portable-core check at `Compile` (integrator) | a feature; depends on the profile ruling (6e); recorded |
| `$round` on the decimal spelling (practitioner) | ruling 8 |
| Round once on every inexact conversion (pl) | ruling 8 |
| Regex wording (practitioner, js, pl) | ruling 4 |
| README precedence order, rule 6, rule 7, rule 9, member order, class E table, position unit, portable-core claim (pl, js, practitioner, idiom) | ruling 6; the README is frozen |
| The last-step singleton rule and the `$number` grammar (practitioner) | language semantics owed to the suite under rule 1, not API statements; noted for `suite/` |
