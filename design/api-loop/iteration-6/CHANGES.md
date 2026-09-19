# Iteration 6: changes

Triage of the iteration-5 panel (five cold reads of the stub at `7f8b4fe`,
under `iteration-5/panel/`; grades in `iteration-5/grades-after.md`).
Reviewer keys: idiom, integrator, security (this round's rotating lens),
pl, practitioner. Iteration 6 is the loop's cap; its panel is the last.

## Applied

| Change | Finding | Reviewers |
| --- | --- | --- |
| `Limits` gains methods (`Compile`, `MustCompile`, `Eval`, `Unmarshal`, `Marshal`, `NewEncoder`, `Materialize`, `Clone`) and the package-level functions of the same names run under `DefaultLimits`, on the `net.Dial`/`net.Dialer.DialContext` pattern; every `*Limits` parameter and every `nil` on the common path is gone | three lenses hit the same shape from three sides: the idiom reviewer (third panel proposing a receiver form; `net.Dialer`, `http.Client` precedent), the integrator (second panel asking for a bounded `Unmarshal`), and the security reviewer (an unbounded `Marshal`) | idiom, integrator, security |
| A unit of work is a bounded-cost step: size-proportional operations are charged per byte or value visited, including carried and caller-supplied values; regex operations charged the subject length and program size; `json.Number` reads charged per byte | `$count([1..1000000].$contains($$.blob, "☃"))` was one million units for ten terabytes of scanning; the budget implied a CPU bound it did not deliver | security (demonstrable) |
| `MaxDepth` bounds every value traversal (operations, `Marshal`, `Materialize`, `Clone`), so a cyclic value is `CodeBudget`; the process-survival claim names the three fatal runtime errors and which the package prevents | `$string($)` over a Resolver graph with a parent link exhausted the goroutine stack, which is fatal, not a panic | security (demonstrable) |
| `MaxIntegerBits` enforced on a token's digit count before parsing, at `Unmarshal`, `json.Number` reads, literals, and `$number` | a twenty-million-digit integer token decoded in minutes under the byte bound, uncancellable | security (demonstrable) |
| `Marshal` and `Encoder` bounded by `MaxBytes` and `MaxDepth`; the serialized size of a result stated as unbounded by the node count | `[1..1000000].$$` passed evaluation at one million nodes and marshaled a terabyte | security (demonstrable) |
| `MaxBytes` is cumulative | "bytes allocated ... including intermediates" admitted a peak reading under which repeated encoding of one blob was unbounded | security |
| `Close` blocks until in-flight calls return | "ends when Close returns and any call then in flight completes" named two instants; the failure mode is a fatal map race | security, idiom (iteration 4) |
| `Clone(ctx, v, env)` | results alias `Env.Bindings` and there was no owned copy; asked as a footgun in three consecutive integrator reports | security, integrator |
| `Token` empty for `CodeMalformedInput`; `Value` stated as able to hold payload content with redaction guidance; `Error()` format stated as unstable | `$error("k=" & $$.api_key)` moves payload into logs through `Value`; `Token` for malformed input was unstated | security, idiom, practitioner |
| Cancellation checked at least every 1024 work units | "at intervals bounded by MaxWork units" read as once per budget | security, integrator |
| `Unmarshal` refuses invalid UTF-8 text; `Marshal` refuses a string that is not valid UTF-8 | a carried invalid string reached `Marshal`, which was silent | security |
| `Bindings` doc: every binding is readable and can appear in any result | an exfiltration surface the API did not name | security |
| Binding `"eval"` to nil disables `$eval`, stated as supported | asked in three consecutive integrator reports; the security reviewer prefers the idiom to a knob | integrator, security |
| Binding names validated against the language's identifier form; negative `ArrayView.Len` is E1001; a panic in `Now` propagates; `$random` is the top-level generator | unstated | security |
| The refusal-versus-rounding text rewritten as two policies rather than one principle: the refusal is where an identifier would lose digits entering the decimal domain; integer division yields a fraction rounded once because a fraction is float64 | the stated principle ("an operand is never converted inexactly; a non-integral result of exact operands is rounded once") implied rounding `9007199254740993 + 0.5`, since `0.5` is exact, and so did not justify the refusal it introduced | pl (a contradiction in the text; the rule itself is ruling 8b) |
| Exactness is a property of the value, not its provenance: a rounded quotient that lands on an integer is thereafter an exact integer, stated with the `9007199254740993 / 2` example | the model let a rounded result masquerade as exact without saying so | pl |
| The comparison example corrected to `9007199254740993 > 4503599627370496.5` | `9007199254740992.5` is not representable and became the integer `9007199254740992`, so the example did not exercise the rule | pl (demonstrable) |
| Member order stated as representation, not value; "only carriage preserves representation" qualified | `$keys` observed representation without carriage, contradicting the sentence | pl |
| Nil `*Object` is the empty object | three meanings for one datum, and the two unset-object representations disagreed | pl, idiom (iteration 4) |
| Purity is plan-level; `ReasonUnsupportedCall` deleted: every standard function except `$random` and `$eval` is pure for selection, `$now` included | purity was stated per field and per plan; the integrator could not predict at config time which transforms would be selective | pl, integrator |
| `Resolve` must return an equal view for the same value for the life of an evaluation; a field cancelled by ctx is not memoized and is recomputed | rule 9 depended on a determinism the API did not require; retry after cancellation was unspecified | pl, integrator (iteration 4) |
| `$type` and equality of `[]byte` need no encoding; `$length` of a `[]byte` is the length of its encoding; "injective encoding such as base64" | equality "observes content" and content needs an encoding, so `a = b` on bytes without one was ambiguous | pl |
| `Plan.Fields()` returns `[]string`, matching `Expression.Fields` | two methods named `Fields` with different shapes | idiom |
| Selection-versus-guard stated as plan-dependent | "Select is not a guard" flipped with the plan, which flips with the input | integrator, idiom |
| `EvalMarshal(ctx, input, env)`; `EvalJSON` defined as `EvalMarshal` over `Unmarshal` | the protobuf path was `Eval`, `Materialize` with a second budget, `Marshal`; the one pass already existed inside `EvalJSON` | integrator (second panel), perf (iteration 4) |
| The sequence rules on the returned value stated (one is the value, more is `[]any`, empty is absent, a constructed or `[]`-kept array is `[]any`) | unstated | practitioner (second panel) |
| `$number` follows the literal rule with the documented prefixes; `$formatNumber`/`$formatInteger`/`$formatBase` exact for integers with the incorporated picture-string rules; `$parseInteger` exact; `$fromMillis`/`$toMillis`/`$millis` int64 milliseconds, non-integral truncated toward zero; `$base64encode` of `[]byte` encodes the bytes; `$base64decode` refuses non-UTF-8 | unstated; `$number("9007199254740993")` is routine | practitioner (second panel) |
| `Eval` doc defines "carried" in place and says "a type outside the admitted set" | method docs depended on package-doc vocabulary | idiom |
| Admission stated as by exact type, not structure, with `[]MyStruct`, `[16]byte`, `time.Time`, and `json.Marshaler` implementers named as foreign | a Resolver author learned it from a footgun | pl, integrator, idiom |
| `Compile` beyond `MaxExpressionBytes`/`MaxDepth` is `CodeBudget`; `Marshal` of typed containers and nil collections stated; `ReasonDynamicShape` `Select` on a non-object is absent; negative `Limits` fields are zero; `Object` safe for concurrent reads; `Range` order must not change between calls; `Encode(nil)` writes null; `Env.Now` and `Resolver` called concurrently when shared; `Error.Offset` for runtime errors; `Value` type for `$error` | the "unclear" lists | integrator, idiom |
| "Borrow" vocabulary replaced by "must not be modified while in use" | Rust vocabulary | idiom |
| Ledger: the `$string(1e21)` row relabeled as the value model overriding an incorporated rendering; `1e23` added; integer division and multiplication overflow added to the exact-integer rows; `D3061` never raised; ill-formed strings row extended; the regex row's authority marked pending on whether the documentation names the dialect; the `$round` doc comment says the basis is current, not settled | rows cited the wrong class; the doc declared what the ledger called pending | pl, practitioner |

## Ruled (added to RULINGS.md)

- Ruling 4, regex dialect: sixth panel; the security reviewer names it the
  ReDoS decision and asks that unsupported constructs be refused at `$eval`
  as well as at Compile; the practitioner and PL skeptic both believe the
  documentation's regular-expression page names JavaScript syntax, which
  would move the authority class from silent to incorporated.
- Ruling 7, `Close`: a third idiom reviewer argues for deletion; the
  security reviewer wants it kept and blocking.
- Ruling 8b: fourth reviewer in three panels (pl) shows the refusal is not
  derivable from the stated principle; the doc now states two policies.
- Ruling 8a: the practitioner and PL skeptic both argue that the number the
  package prints (`$string(2.675)` is `"2.675"`) should be the number it
  rounds.
- Ruling 9: the integrator asks again for `Canonical`.
- Ruling 1, 3, 5, 6: evidence added.
- Ruling 10 (new): the decimal decode boundary (idiom: carry
  non-round-tripping decimal tokens as `json.Number`; pl: state the decode
  boundary as the value function).

## Rejected

| Finding | Reason |
| --- | --- |
| Replace `(value, present, err)` with an `Undefined` sentinel (idiom) | one reviewer, against the practitioner and PL skeptic, who both defend the triple this panel; the doc's honesty about `v, _, err` stands |
| Carry non-round-tripping decimal tokens as `json.Number` (idiom) | ruling 10; one reviewer; a representation-preserving decoder is a value-model question |
| Delete `Close` (idiom) | ruling 7 |
| `Select` with a required first key; restrict `path` to constructor keys; rename `Prepare` (idiom) | one lens across five panels; ruling 2 covers the path shape |
| `Project(ctx, keys...)` multi-select (integrator) | one reviewer; the loop every integrator writes has a partial-failure policy the package should not choose for them; recorded |
| Ship the protobuf Resolver (integrator, fifth panel) | ruling 5 |
| `Canonical` (integrator) | ruling 9 |
| Round once on every mixed operation and delete `E1008` (pl, practitioner) | ruling 8b |
| `$round` on the decimal spelling (practitioner, pl) | ruling 8a |
| Regex wording (security, practitioner, pl) | ruling 4 |
| README rules 3, 6, 8, 9; the authority ladder; the portable core (pl, practitioner, idiom, integrator) | ruling 6; the README is frozen |
| A class-level semantic core under `suite/` (pl, second panel) | outside the loop's artifact; recorded as the first post-loop task under ruling 6 |
| Split `E1001` into four codes (pl) | one reviewer; the four situations share the remedy "the value cannot be read as given" from the caller's seat, and `Class()` already separates them from language errors; recorded |
| `??` on null: cite the documentation (practitioner) | verified against jsonata-js 2.1.1 (null is a value; `$x ?? "d"` with `$x` null is null); the documentation's wording is a suite matter |
