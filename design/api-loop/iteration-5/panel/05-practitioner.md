# Iteration-5 panel: JSONata practitioner

Cold read of `7f8b4fe`. Lens: a JSONata practitioner (Node-RED, Step Functions, jsonata-js).

## Grades

| Criterion | Grade | One-line justification |
| --- | --- | --- |
| Idiomatic fit for Go | A- | `Compile`/`MustCompile`/`String()` mirror `regexp`, `(value, present, err)` is the right shape for undefined-vs-null, `iter.Seq` on `Object`, sentinels plus `errors.As`; the map-vs-`*Object` duality is the one wart. |
| Ergonomics of the common path | B+ | `Compile` → `Unmarshal` → `Eval` → `Marshal` is four lines and correct; but every result-consuming caller has to know two object types, and `$error` messages are hidden behind `Error.Value`. |
| Correctness and footgun risk | B- | Big-int carriage and code-point strings fix real bugs, but the two loudest practitioner-facing questions (regex dialect, `$round` ties) are still "pending", `??` on null is asserted without a citation, and `$string` of any computed decimal now prints float noise that 15-digit rendering used to hide. |
| Performance headroom the API permits | A | Compile-once, `Reads()` for fetch pruning, selective evaluation with shared work, views over protobuf with no conversion, carriage by reference, explicit budgets: more than jsonata-js gives me by a wide margin. |
| Concept soundness (JSONata-as-regex, documentation-as-authority, host-native values) | B | Host-native values is the right call for this job and the regex analogy carries it; "documentation as authority" is overstated because the JSONata docs are a reference manual, not a spec, and the class's own rule 1 quietly admits the test suite is what actually governs. |
| Overall | B+ | I would use it for OpenBindings transforms today inside the portable core; I would not yet trust it with regex-heavy or money-rounding expressions until the two pending rulings land. |

## Would my expressions mean the same thing?

1. `{ "id": user_id, "name": display_name }` over `{"user_id": 9007199254740993, ...}`
   jsonata-js: `id` is `9007199254740992` (lost at `JSON.parse`). Here: `9007199254740993`, exact, as `int64`. **Different, declared** ("Integers above 2^53"). This is the whole reason to want this library.

2. `"Total: " & $sum(items.(price * qty))` with prices `0.1`, `0.2`, qty `1`
   jsonata-js: `"Total: 0.3"`. Here: `"Total: 0.30000000000000004"`. **Different, declared** (`$string` row). This is the divergence that will hit the most people, because `&` with a computed decimal is in every second transform I have ever written, and nobody who writes it knows they are relying on `toPrecision(15)`. The README's "portable core" lists `$string` as part of the subset where "documentation and the reference agree without exception". That is false for any non-integral number; see change 4.

3. `$round(price, 2)` with `price` `2.675` in the payload; also `$round(1.015, 2)`
   jsonata-js: `2.68` and `1.02` (it shifts the decimal spelling, then ties go to even). Here: `2.67` and `1.01`. **Different, ledger says pending.** Note the internal inconsistency: this package's own `$string(2.675)` renders `"2.675"` (shortest round-trip, per JSON.stringify), so the language tells me the value is 2.675, then `$round` treats it as below 2.675. The reference is consistent with itself here and this package is not.

4. `$keys(Account.Order{$string(OrderID): $})` with OrderIDs `10`, `2`
   jsonata-js: `["2", "10"]` (integer-like keys hoisted, numeric order). Here: `["10", "2"]` (first-seen grouping order). **Different, declared.** Anyone who used grouping-by-numeric-key as a free sort loses that; I have seen that trick in Step Functions definitions.

5. `$match(email, /^(?=.{1,64}@)[^@]+@[^@]+$/)`
   jsonata-js: works. Here: **undefined by the doc comments**; the ledger says the regex dialect is a pending ruling. If the answer is Go `regexp` (RE2), this expression fails at `Compile` with a syntax error, as does anything with `(?!`, `(?<=`, or `\1`. That is a language-level break, not a value-model one, and it is currently filed under "value model: the documentation is silent". I do not think the documentation is silent; the regex page describes the JavaScript syntax. Verify and re-file.

6. `$split(name, "")` with `"😀a"`
   jsonata-js: `["\ud83d", "\ude00", "a"]`. Here: `["😀", "a"]`. **Different, declared, and the new answer is the one the documentation describes.** Gain.

7. `items^(>price)[0].name` with prices `10` (integer) and `9.5`
   Same answer both sides: ordering is by value across `int64`/`float64`/`json.Number`, and both sorts are stable. **Same.**

8. `nullable_field ?? "default"` where the field is JSON `null`; also `$x ?? "d"` with `$x` bound to nil in `Env.Bindings`
   The doc comment asserts `??` "falls through on absent only", so the answer here is `null`. My recollection of both the 2.1 operators page and the reference is that `??` returns the RHS when the LHS is undefined **or** null, like the JavaScript operator it is named after, which would make jsonata-js answer `"default"`. **Possibly different, not declared, not cited.** This is the first thing I would check against the pinned docs, because if I am right it is an undeclared divergence inside the portable core.

9. `$number(id_str)` with `"9007199254740993"`
   jsonata-js: `9007199254740992`. Here: **undefined by the doc comments.** The numeric-literal rule is spelled out; the string-to-number rule is not. Round-tripping IDs through strings is routine.

10. `{ "count": $count($), "first": $[0].name }` over an array root
    Same result both sides (constructor groups over the array; each field sees the whole group). **Same**; the package just reports `ReasonArrayInput` and evaluates whole. Fine.

11. `$fromMillis(ts_ns / 1000000)` with a nanosecond timestamp above 2^53
    jsonata-js: the literal is already rounded before division; `new Date` truncates the fraction. Here: the division yields an exact quotient rounded once to a non-integral `float64`; whether `$fromMillis` accepts a non-integral argument is **undefined by the doc comments.**

12. `$string(1e21)` and `1e23 + 1`
    jsonata-js: `"1e+21"` and `1e23`. Here: `"1000000000000000000000"` and `99999999999999991611393`. **Different, declared.** I understand the rule (an integral float64 is an integer), and it is internally consistent, but the ledger labels the `$string(1e21)` row "value model" when it is in fact the value model overriding the incorporated authority (`JSON.stringify(1e21)` is `"1e+21"`). Label it honestly.

## What a JSONata user gains and loses

**Gains versus jsonata-js**

- Exact integers. In Step Functions I have watched Snowflake IDs and Stripe amounts-in-minor-units get silently rounded; AWS documents the 2^53 trap. This library makes `id = 9007199254740993` mean what it says. For protobuf `int64` fields this is not a nicety, it is the difference between correct and wrong.
- Characters are code points everywhere (`$split` with `""`, `$match.index`, ordering), which is what the documentation says and what every non-JS user assumes.
- Document order for objects out of `Unmarshal`, with no integer-key hoisting. `$string($)` of a decoded payload prints it in the order it arrived.
- Refusal instead of approximation: overflow, non-finite, inexact mixed arithmetic, and budget exhaustion are errors with classes I can switch on. jsonata-js gives me `Infinity` or a silently rounded number.
- Bounded evaluation and cancellation via `ctx`. jsonata-js has a timeout hook and a depth guard and nothing else; `$range` to 1e6 and `$pad` to 1e9 are how you DoS a Node-RED flow.
- Selective evaluation and `Reads()`. Nothing like this exists in the reference; for an API gateway that only needs two fields of a transform, this is real money.
- No JSON round trip, no intermediate tree: views over protobuf messages read in place.

**Losses versus jsonata-js**

- The 15-significant-digit smoothing on `$string`. Practitioners do not know they depend on it, and they do. Every `"Price: " & (a * b)` that used to print `"3.3"` will print `"3.3000000000000003"`. The documentation says JSON.stringify, so the package is right and the reference is wrong, and it will still generate the most support tickets.
- The regex dialect, until ruled. If it is RE2: no lookaround, no backreferences, and expressions copied from the Exerciser fail to compile.
- Host functions. Node-RED's `$env`, `$flow.get`, `$global.get`, and every `expr.registerFunction` idiom are gone by design. Values in `Env.Bindings` only. For OpenBindings' closed transforms that is correct; a JSONata user migrating a flow will feel it.
- `$round` on decimal ties, until ruled (currently the wrong way, in my view).
- Integer-like key ordering, for the people who leaned on it.
- Mixed arithmetic past 2^53 now errors (`E1008`) where the reference silently works. `x * 1e-9` failing while `x / 1e9` succeeds is a rule I can learn, but it is a new rule.
- Function-valued results are errors. Nobody will miss this.
- Duplicate member names in input are refused. Rare, but some real APIs emit them and `JSON.parse` tolerated it.
- Two object representations on the way out (`map[string]any` when carried, `*Object` when constructed). jsonata-js hands me one kind of object.

**Versus a faithful port**

A faithful Go port cannot be faithful: it would need JavaScript numbers (so it loses big ints), JavaScript regex (so it needs a backtracking engine that breaks the no-unbounded-allocation promise), UTF-16 strings, and ECMAScript own-property ordering. The existing Go ports already diverge on all of these, silently. So the real comparison is "declared divergences with a value model chosen for the job" against "undeclared divergences with a value model chosen by accident". That is not close.

## What I would change

1. **Rule the regex dialect now, and file it as a language-level departure, not a value-model one.** Pick Go `regexp` (RE2) because it is the only choice consistent with "cannot allocate without bound", then say so at the top of the package doc, not in a pending ledger row. Wording for the "Strings, bytes, and regular expressions" section: *"Regular expressions are Go regexp syntax (RE2). The documentation describes JavaScript regular expressions; the constructs RE2 does not implement, lookahead `(?=` `(?!`, lookbehind `(?<=` `(?<!`, and backreferences `\1`, are rejected at Compile with an S code, and `\d`, `\w`, `\s`, and `\b` are ASCII as in JavaScript. See DIVERGENCES.md."* And the ledger row's authority column becomes "engine: RE2 for bounded matching; the documentation names JavaScript syntax".

2. **Follow the reference on `$round` ties by judging the tie on the shortest round-trip decimal.** The package already declares that spelling to be the number's rendering under `$string`; rounding should agree with it. Doc-comment wording: *"`$round` judges a tie on the value's shortest round-trip decimal spelling, the spelling `$string` renders, so `$round(2.675, 2)` is `2.68` and `$round(1.015, 2)` is `1.02`, as the reference gives: the number the language rounds is the number it prints."* Delete the pending row.

3. **Cite the documentation for `??` on null, and fix it if the docs say null falls through.** Replace *"?? falls through on absent only, so $x ?? "d" with $x bound to nil is null"* with either a quote of the operators page sentence that supports absent-only, or *"`??` yields the right operand when the left is absent or null, as the documentation defines it; a binding whose value is nil is null and therefore falls through."* If absent-only is what the docs say, keep it, but add the ledger row anyway with authority "documentation" so nobody wonders.

4. **Fix the README's portable-core claim.** `$string` is listed in the subset where "the documentation and the reference agree without exception"; expression 2 above is a counterexample inside that subset. Wording: *"`$string` and `&` over strings, integers, booleans, and objects and arrays of those; over a non-integral number the reference departs from the documentation (see each member's ledger)."* Same qualifier for `$merge` and `$sift` regarding member order with integer-like keys.

5. **Specify the number-and-bytes functions the doc comments skip.** Add to "Numbers": *"`$number` parses a string by the numeric-literal rule: an integral token is an exact integer, a token with a fraction or exponent is the nearest float64, with the `0x`, `0o`, and `0b` prefixes the documentation lists. `$formatNumber`, `$formatInteger`, and `$formatBase` render exact digits for any integer; `$parseInteger` yields an exact integer. `$fromMillis` and `$toMillis` take and yield int64 milliseconds, and a non-integral argument is truncated toward zero as the reference does."* Add to "Strings, bytes": *"`$base64decode` yields a string and is an error when the decoded bytes are not valid UTF-8; `$base64encode` of a []byte encodes the bytes themselves, not their BytesEncoding presentation."*

## Things the doc comments leave unclear to a JSONata user

- Does `??` fall through on null? (Above.)
- What does `$number` do with `"9007199254740993"`, `"1e400"`, `"0x1F"`, `" 12 "`, and `"12abc"`?
- Do `$formatNumber`, `$formatInteger`, `$formatBase`, and `$parseInteger` handle `*big.Int`, and are the XPath F&O picture-string semantics the documentation points at taken as incorporated authority (they should be listed in the ledger's authority vocabulary if so)?
- What do `$base64encode`/`$base64decode` return, and how do they interact with a `[]byte` input and the `BytesEncoding`?
- Does `$fromMillis` accept a non-integral number, and is `$toMillis`/`$millis` an `int64`?
- Sequence versus array on the way out: "a sequence result is a []any and a singleton is the value itself" does not tell me whether `[1]` (a constructed array) comes back as `[]any{1}` or `1`, whether `items.name` over one item comes back as the string (it should), whether `items.name[]` keeps the array, and whether an empty sequence is absent while `[]` is an empty array. Say that the documentation's singleton and `[]` rules apply and give the four cases.
- `$string` of an object containing a decimal: the ledger row names only the scalar case and `&`; "numbers as above" implies nested numbers render the same way. Say so explicitly, because the reference's replacer applies `toPrecision(15)` inside objects too.
- `$match` result shape: are unmatched optional groups absent or empty strings in `groups`? Are named groups exposed? What is the `index` when the subject is a `[]byte`?
- `$replace` with a function replacement: what does the function receive, and does the `$0`/`$N`/`$$` rule apply only to string replacements (it should)?
- Runtime error positions: `Offset` is described as an offset into the expression source; does a `D`/`T` error raised mid-evaluation carry `Offset`, `Line`, and `Column`, as the reference carries `position`?
- `$error("Order 123 invalid")`: the message is in `Error.Value` and not rendered by `Error()`. This needs to be shouted, because every practitioner will log `err` and lose their message.
- `$merge([map, *Object])`: result type and member order when the operands have different representations.
- `$keys` of `map[string]int64` and other typed maps: sorted, presumably, but the "Objects and member order" paragraph only names `map[string]any`.
- `$uppercase`/`$lowercase`: "full Unicode case mapping" implies `x/text/cases`; is it locale-independent (`language.Und`), as `toUpperCase()` is?
- `$type` of a `*big.Int`, a `json.RawMessage`, and an `ObjectView`.
- `$distinct` and `=` on arrays and objects: order-insensitive for objects is stated; is array equality positional and deep?
- `$sort` with a comparator that errors, and `^()` with mixed-type keys: presumably `D3070`, but nothing says the language codes are raised in exactly the reference's situations for these.
- What "the reference's table is the catalogue" means for codes the reference raises in situations this package cannot reach (D2014 for range) and situations it reaches differently (D3061 for `$power` overflow, now `E1002`): the ledger has the range row; it should also state that `$power` never raises D3061.

## On the concept

The three-part framing has one strong leg, one adequate leg, and one that is dressed up. The strong leg is host-native values. A regex engine really is the right analogy for that: nobody expects Go's `regexp` to model JavaScript strings, and nobody should expect a Go JSONata engine to model JavaScript numbers. For the job this library is being built for, decoded API payloads with `int64` IDs, the JavaScript value model is a defect, and every "faithful" port that inherits it is faithfully wrong. I would trust this leg, and the declared-divergence discipline around it (refuse rather than approximate, pin each departure to a fixture) is exactly what I want from a second implementation.

The dressed-up leg is "documentation as authority". Regex notations have real specifications; ECMAScript §22.2 and the RE2 syntax page define every construct. The JSONata documentation is a function reference with examples and a handful of prose pages. The behaviors that make my expressions mean what they mean, sequence flattening, the singleton and `[]` rules, what a predicate does over an array of arrays, how grouping merges values under a shared key, parent and index bindings, sort with absent keys, are defined by jsonata-js and its ~1500-group test suite, not by any sentence in the docs. The README's own membership rule 1 concedes this ("verified against the reference test suite"). So the honest statement is: the test suite governs, the documentation breaks ties in the value-model direction, and the value model is the host's. I would trust that statement. I trust the current wording less, because when the docs are silent (which is most of the time) it leaves me guessing which of the two the engine picked, and the ledger's "value model: the documentation is silent" on the regex row shows the wording being used to defer a language-level decision.

Would I rather have a faithful port? No, because there is no such thing in Go; there are only ports that hide their divergences. What I want from this one is smaller than a philosophy: rule regex and `$round` before the first tag, cite the docs for `??`, fix the portable-core claim about `$string`, and publish the suite pass count with the exact fixtures skipped. A ledger with fifteen rows before the suite has run is a draft; if it still has fifteen rows after the suite has run I will not believe it, because I have written enough `$formatNumber` and `$fromMillis` picture strings to know where every port bleeds. Show me the number, and I will move my expressions over.
