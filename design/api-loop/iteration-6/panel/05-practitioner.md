# Iteration-6 panel: JSONata practitioner

Cold read of `81e3693`. Lens: a JSONata practitioner (Node-RED, Step Functions, jsonata-js).

## Grades

| Criterion | Grade | One-line justification |
| --- | --- | --- |
| Idiomatic fit for Go | B+ | `Compile`/`MustCompile`, `Limits` as a `net.Dialer`-style receiver, `iter.Seq` on `Object`, sentinel-plus-`errors.As` are all recognisable; two result shapes for "object" (`map[string]any` vs `*Object`) is the tax. Not my lens; the purist will go deeper. |
| Ergonomics of the common path | B | The `(value, present, err)` triple is exactly what jsonata-js users needed and never had in a typed form; `Unmarshal` as the front door is right. Loses points because every Go caller must learn that `{ "a": $.a }` and `$` return different object types. |
| Correctness and footgun risk | C+ | Several data-dependent divergences that pass tests and fire in production: E1008 on `x * 0.001` but not `x / 1000`; every float64 at or beyond 2^53 rendered and computed as exact binary digits (`$string(1e23)`, `1e300 * 1e300`); a pending regex dialect; a pending `$round`; one wrong reference value in the ledger. |
| Performance headroom the API permits | A- | By-reference carriage, views over protobuf without conversion, compile-once, `Reads()` for fetch pruning, selective fields with shared memoisation. jsonata-js offers none of this. |
| Concept soundness | B- | Host-native values and documentation-as-authority are right for the OpenBindings use; the trouble is that the JSONata documentation is a tutorial, not a spec, so "documentation wins" collapses into "the ledger decides" for anything interesting, and the ledger currently has two pendings and one factual error. |
| Overall | B- | I would use it for OpenBindings transforms over API payloads and would not use it as a drop-in for Node-RED expressions without the changes below. |

## Would my expressions mean the same thing?

Ten expressions I would actually write. "Same" means the same value for realistic inputs; the interesting cases get both answers.

| # | Expression | jsonata-js | This library | Declared? |
| --- | --- | --- | --- | --- |
| 1 | `Account.Order.Product.(Price * Quantity)` then `$sum(...)` | sequence flattening, singleton unwrapping, float arithmetic | Same. Integer quantities times decimal prices stay float64; integer times integer is now exact, which only differs above 2^53. | n/a |
| 2 | `"Total: " & (unit_price * 3)` with `unit_price` = 1.1 | `"Total: 3.3"` (15-significant-digit rendering) | `"Total: 3.3000000000000003"` | Yes, row 1. This is the divergence every Node-RED user will hit first, because concatenating a computed decimal into a label is the single commonest JSONata idiom outside path navigation. |
| 3 | `$round(2.675, 2)` | `2.68` | `2.67` | Yes, row 3, marked pending. |
| 4 | `id = 1234567890123456788` where the payload has `"id": 1234567890123456789` | `true` (both sides round to the same float64; this is the bug that makes people stop using JSONata on IDs) | `false`, and `$string(id)` yields the exact digits | Yes, row 4. But row 2's reference column is wrong: with the 15-significant-digit step row 1 describes, `$string(1234567890123456789)` in jsonata-js is `"1234567890123460000"`, not `"1234567890123456800"`. The latter is what plain `JSON.stringify` would print; jsonata-js's `$string` applies `toPrecision(15)` to the root value too. Same for `$string(9007199254740993)`: `"9007199254740990"` there. |
| 5 | `{ "ms": ts_ns / 1000000, "ms2": ts_ns * 0.000001 }` with `ts_ns` = 1758000000000000123 | `ms` = `1758000000000`, `ms2` = `1758000000000` (the input already lost its low digits) | `ms` = `1758000000000.0001` (exact quotient rounded once), `ms2` = error E1008 | The E1008 half is row 6, pending. The fact that the two spellings of the same computation give a float and an error respectively is stated in the doc comment, not in the ledger. |
| 6 | `$` or `{ "mass": mass }` over `{"mass": 1.989e30}`, then `Marshal` or `$string(mass)` | `1.989e+30` | a 31-digit integer whose low digits are the binary representation, because 1.989e30 is an integral float64 and "an integral float64 is its integer" | Row 2 declares `$string(1e21)`. It does not declare that a carried value re-encoded by `Marshal` changes its JSON spelling, nor that `1.989e30 = 1989000000000000000000000000000` is true in jsonata-js and false here. |
| 7 | `$keys({"10": 1, "2": 2, "a": 3})` | `["2", "10", "a"]` | `["10", "2", "a"]` for `*Object` (document order) and coincidentally the same for a map (byte order sorts `"10"` before `"2"`) | Yes, row 8. |
| 8 | `$split("a😀b", "")` | four elements, two of them surrogate halves | three elements | Yes, row 9. Nobody wants the reference's answer. |
| 9 | `$match(line, /(?<=\$)\d+(\.\d\d)?/)` | works (JavaScript dialect) | undefined; row 15 is pending and the recommendation on record (Go `regexp`) would refuse this at `Compile` | Pending, not decided. |
| 10 | `$x ?? "n/a"` with `$x` bound to `nil` | I would expect `"n/a"` if `??` follows the JavaScript meaning its spelling advertises; I cannot swear from memory what the 2.1 documentation says | `null`, per the doc comment ("falls through on absent only") | Not in the ledger, and the doc comment asserts a reading without citing the documentation sentence. If the documentation says undefined only, cite it, because every JavaScript user will guess the other way. If it says null too, the stub is wrong. |

Everything else I tried in my head (grouping constructors, predicates, `$sift`, `$merge`, `$each`, the transform operator, `$now` in UTC, `$eval`, sort stability, `%` sign semantics, `$fromMillis` truncation, `$replace` group references, `$number` with `0x`/`0o`/`0b`) reads as the language I know. The doc comments are careful and mostly right; the problems are concentrated in the number model's edges.

## What a JSONata user gains and loses

**Gains versus jsonata-js**

- Exact int64 and beyond. This alone justifies the library for API payloads: protobuf int64 IDs, nanosecond timestamps, snowflake IDs, and `$parseInteger` all survive. In jsonata-js `id = id + 1` can be true for large ids. I have personally lost IDs to this in Step Functions.
- `present` as a first-class return. jsonata-js users learn the undefined/null distinction by being bitten; the API makes it unmissable.
- Member order that means something. `*Object` preserves document order; the reference silently hoists integer-like keys, which corrupts any object keyed by numeric strings (order IDs, years) when it is rendered.
- Regex literals compiled at `Compile`, and syntax positions at evaluation time. jsonata-js compiles regex literals lazily too, but you find out at runtime.
- Bounded evaluation. `[1..1e9]`, deep recursion, giant `$string` results, all refused under a stated budget, plus context cancellation. jsonata-js has no cancellation at all.
- `Reads()` and selective `Select`. Nothing in the reference offers static read analysis; for a gateway that can fetch fewer fields this is a real capability.
- Bytes as a value. jsonata-js has no bytes; you get whatever the binding already stringified.
- A closed environment. For document-supplied expressions this is what you want; `registerFunction` is a liability, not a feature, in that setting.

**Losses versus jsonata-js**

- try.jsonata.org stops being a truthful playground. Every practitioner develops expressions in the Exerciser and pastes them in. For anything numeric beyond 2^53, any `&` on a computed decimal, any `$round` on a midpoint, any lookaround regex, the Exerciser now lies about what this library will do. This is the biggest practical cost and the README does not mention it.
- The 15-digit "pretty" rendering. The reference's `toPrecision(15)` is undocumented but deliberate: it hides float noise in the commonest formatting idiom. Under this library, `"Price: " & price * 1.1` shows `3.3000000000000003`. The documentation supports the library (`$string` is defined by `JSON.stringify`), but authors will experience it as a regression and reach for `$formatNumber`, which they never had to before.
- The JavaScript regex dialect, if Go `regexp` is chosen: lookahead, lookbehind, backreferences, and a different `\s` (RE2's is ASCII; JavaScript's includes U+00A0, U+FEFF, U+2028). These are used in real `$match`/`$replace`/`$split` expressions.
- `$round` on decimal midpoints, if the pending stays as it is.
- Silent float arithmetic on mixed big-integer and decimal operands becomes a data-dependent error (E1008). An expression that passes every test fails on the first payload with an id above 2^53 next to a `* 0.01`.
- Duplicate keys in input become a hard `Unmarshal` failure. Sloppy upstream JSON does exist; the reference's last-wins is forgiving. Defensible, but it will be met at 3 a.m.
- Binding an absent variable. Minor.

**Versus a faithful port**

A faithful port (dashjoin's jsonata-java, blues' jsonata-go, the various Rust and Python ones) gives you the Exerciser's truth and the reference's number model together, including the loss of integer precision. For OpenBindings that number model is disqualifying, so the port is not actually on the table. What a port would have given you, and this class must earn instead, is that the test suite is the spec and no one has to read a ledger.

## What I would change

Ranked from a JSONata user's seat.

**1. Stop treating a float64 at or beyond 2^53 as an exact integer.** The rule "a value is an integer when it is integral, whatever its representation" is true of binary64 and false of intent. Numbers spelled with an exponent are magnitudes, not counts; `1e23` in an expression or a payload denotes ten to the twenty-third to its author, and rendering it as `99999999999999991611392`, or computing `1e300 * 1e300` as an exact 601-digit integer where the reference raises D1001, exposes binary representation in the output text. It also breaks the spirit of rule 4: `Unmarshal` then `Marshal` of `{"x": 1e23}` changes the JSON spelling of a value the expression only carried. Proposed doc-comment wording, replacing the sentence beginning "A value is an integer when it is integral":

> A float64 whose value is integral and whose magnitude is below 2^53 takes part in arithmetic and comparison as that integer, so x / 100.0 is x / 100 and json.Number("1.0") + 9007199254740993 is exact. A float64 at or beyond 2^53 is a decimal: it combines by float64 arithmetic, so 1e23 + 1 is 1e23 and 1e300 * 1e300 is D1001 as the reference yields, and it renders as JSON.stringify does, so $string(1e21) is "1e+21". An int64 or *big.Int renders as its exact digits.

Then delete ledger row 5 (integral float64 beyond 2^53 in arithmetic), narrow row 2 to the exact-integer representations, and correct row 2's reference column to `"1234567890123460000"`. Add a row for `1e23 = 100000000000000000000000` (true there, false here), which survives under either rule because comparison stays exact.

**2. One rule for entering the decimal domain, not two.** The doc comment's own example is the wart: `x / 1e9` succeeds and `x * 1e-9` fails for the same `x`. The justification ("the language defines division to yield fractions") applies equally to multiplication by a decimal. Either rounding once is the documented decimal arithmetic, in which case E1008 has no place in `+ - * /`, or rule 8 forbids it, in which case `/` must refuse too. From my seat, round once: a data-dependent refusal inside a transform is the worst failure class, and the reference already rounds. Wording:

> An integer operand combined with a decimal operand, and an integer quotient that is not an integer, converts to float64 rounded once; the rounding is observable only for magnitudes at or beyond 2^53, and it is the decimal domain's arithmetic, not an approximation of an integer. Float64 reports whether a value converts exactly; the expression language does not refuse.

Retire `CodeInexact` from evaluation (keep it for `Float64` if you want a typed reason), and close ledger row 6 with the decision.

**3. Settle `$round` on the decimal spelling and match the reference.** The pending note says the documentation's tie examples do not decide. The documentation also says the strategy "is commonly used in financial calculations"; financial rounding is done on decimal digits, and nobody in finance accepts "2.675 is not a tie because binary". The decimal spelling is well defined in this model: it is the shortest round-trip rendering, the digits `$string` itself produces. Wording:

> $round judges a tie on the value's shortest round-trip decimal digits, the digits $string renders, so $round(2.675, 2) is 2.68, as the reference yields.

Delete row 3.

**4. Rule the regex dialect now, and reconcile `Compile`'s error contract with it.** If Go `regexp` is the answer, the ledger row must enumerate what is refused (lookahead, lookbehind, backreferences, `\Z`, possessive and atomic groups, `(?<name>)` only on Go 1.22 or later) and what differs silently (`\s`, `\b` and `\w` are ASCII in both, case-insensitive folding is Unicode in RE2 and simple in JavaScript without the `u` flag). The `Compile` doc comment currently promises only `ClassSyntax` or `CodeBudget`, and the `Code` doc comment says every refusal the package adds is an E code; a refused regex construct is neither S nor budget. Add the code (`CodeUnsupportedValue` fits: "a value the engine cannot read") and list it on `Compile`. Also confirm the D2014 cap in row 13; I recall the reference's limit as 1e7 entries, not 1e6, and pinning to the fixture will settle it.

**5. Expose the divergence surface per expression.** The README's "portable core" is an observation authors cannot act on. Give them something to act on:

> `func (e *Expression) Divergences() []Divergence` reports every ledger entry an expression can exercise, by construct: `$string` or `&` applied to a numeric operand, `$round` with a precision, a regex literal using a construct outside the dialect, `$split` with an empty separator, `$keys`/`$each`/`$spread`/`$string` over an object. Each entry carries the ledger identifier and the expression offset. An empty result means the expression stays inside the portable core.

OpenBindings tooling can then lint document transforms at authoring time instead of authors discovering the dialect in production. This is the API change that would most raise my trust.

## Things the doc comments leave unclear to a JSONata user

- Whether `??` falls through on `null`, with the documentation sentence that decides it. Same for `?:`: is it `$boolean` truthiness (so `0 ?: 5` is `5`), and is that from the documentation?
- Whether `$boolean` of empty `[]byte` is false, and whether a `[]byte` in key position of an object constructor is accepted as a string or is T1003.
- Equality between a `[]byte` and a `string`: ordering "against strings" is stated; equality is only stated for two `[]byte` values. Is `blob = "aGVsbG8="` compared through the encoding? And is `$base64decode(blob)` the sanctioned way to get the raw content as text?
- `$string` versus `Marshal` on a carried `json.Number`: `$string(json.Number("1.10"))` renders the value (`"1.1"`), `Marshal` renders the token (`1.10`). Both are defensible; say it. And what does `Marshal` do with a `json.Number` holding an invalid token that the expression never observed?
- The practical consequence of byte-ordered map keys: `"10"` sorts before `"2"`, `"Z"` before `"a"`, and this is what `$keys`, `$each`, `$spread`, and `$string` see for every `map[string]T` input. One sentence of warning would save a support ticket.
- `$merge` order: first-seen position with last-seen value? It follows from `Object.Set` replacing in place, but `$merge`'s doc is where a user will look.
- Whether `$type` can ever distinguish an integer from a decimal (I believe not, and that `1.0 = 1` is true; say so, because users will ask how to tell an exact id from a rounded one).
- Predicate indexing with a non-integer or a `*big.Int` (`items[1.5]`, `items[9007199254740993]`): floor and absent presumably, but unstated.
- Comparison of `[]byte` with a number: T2010, presumably.
- What `Select` returns when a selected field's subexpression yields an array of one element: is singleton unwrapping applied per field exactly as `Complete` would apply it to that member? Rule 9 says identical; the doc comment on `Select` does not restate it for this specific case, which is the one users will test.
- Whether `$replace` recognises JavaScript's `$<name>` named-group reference. The stub rules out Go's `${name}`; the JavaScript form is what a Node-RED author would try.
- Whether `$formatNumber` picture strings render an int64 beyond 2^53 with all digits (stated) and what `$formatNumber` does with a `*big.Int` beyond `float64` range when the picture has a decimal part.
- Whether the reference's `U1001` (recursion limit) maps to `CodeBudget` on `MaxRecursion` or to a language code; the U class is absent from `Class`.

## On the concept

The regex analogy is more honest than the README seems to realise. Every practitioner knows that regex "notation" does not travel: lookbehind works in PCRE and not in RE2, `\d` means different things under `u`, and the first hour of any cross-engine migration is spent on exactly these edges. The analogy therefore predicts fragmentation, and fragmentation is the thing JSONata users fear most, because JSONata's whole appeal is that there is one implementation and the Exerciser tells you the truth. What makes the analogy trustworthy anyway is that the world already has the fragmentation: jsonata-java, jsonata-go, jsonata-rs, and Step Functions' engine each differ from the reference and mostly do not say how. A class that declares its departures, pins them to fixtures, and refuses rather than rounds is the first honest version of a situation that already exists. I would trust that, on the condition that the ledger is complete and correct, and today it is neither: one reference value is wrong, two entries are pending, and the `Marshal`-of-a-carried-float consequence is undeclared.

"Documentation as authority" is weaker than the README makes it sound, because the JSONata documentation is a tutorial. It defines `$string` by reference to `JSON.stringify`, which is an ECMAScript specification over binary64, so the "documentation" already incorporates a JavaScript value model for decimals; the package accepts that and then overrides it for large integral floats, which is where the ugliest results come from. It does not name a regex dialect, does not decide midpoint ties, does not specify key order, and may or may not decide what `??` does with null. In practice the authority for anything interesting will be this package's ledger, and the ledger's authors should write it knowing that it, not the documentation, is what practitioners will read.

Host-native values is the part I would not give up, and it is the part a faithful port cannot provide. For Node-RED I would still want the port, because Exerciser fidelity matters more there than the 2^53 edge. For OpenBindings, where the payload is an API response with int64 identifiers and the transform is written once and run over every call, a port is disqualified on the first id it corrupts. So: yes to the concept for this project, with the number model fixed so that it stops leaking binary digits into output, one rule for the decimal domain, and a way for authors to find out at compile time whether their expression has left the portable core.
