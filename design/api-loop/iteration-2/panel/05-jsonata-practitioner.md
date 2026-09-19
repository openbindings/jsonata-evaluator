# Iteration 2 cold read: jsonata practitioner

> Given only the class README and go/jsonata at 4cb2f5d; told not to read design/. Verbatim.

## Grades

| Criterion | Grade | One-line justification |
|---|---|---|
| Idiomatic fit for Go | A- | Compile/MustCompile/String mirror `regexp`, `(value, present, err)` is the map-lookup idiom for undefined, `iter.Seq` on Object; the map-vs-*Object two-arm switch is the one wart. |
| Ergonomics of the common path | B | `Eval(ctx, src, input, nil)` is one line; but a JSONata author's common path is "paste an expression that works in the playground," and several of those will now error or render differently with nothing in the API to warn them. |
| Correctness and footgun risk | C+ | Mixed integer/float refusal (as worded) rejects `ts_ns / 1e6`; `$round(2.675, 2)` silently disagrees with every JSONata a user has run; `$keys` order depends on which Go container the caller happened to use; regex dialect is never named; `ReasonArrayInput` describes semantics the language does not have. |
| Performance headroom the API permits | A- | Zero-copy admission, views for protobuf, Prepare/Select, `Reads()` for fetch narrowing, budgets instead of timers; the only ceiling is that per-comparison charging at 8M work caps default sorts around 400k elements. |
| Concept soundness (JSONata-as-regex, documentation-as-authority, host-native values) | B | Right for the OpenBindings problem (exact IDs, bytes) and honest about divergence, but the "documentation" is tutorial prose, and its silences (regex dialect, key order, case mapping, float rendering, midpoint rounding) are exactly where practitioners get hurt; the README's "portable core" claim is false for two of its own listed functions. |
| Overall | B | I would use it for transforms over API payloads today, and I would want a "differences you will hit" page before I let anyone else. |

## Would my expressions mean the same thing?

1. `"Total: " & $sum(Order.Price)` with prices `0.1` and `0.2`
   jsonata-js: `"Total: 0.3"` (the reference renders numbers via `Number(x.toPrecision(15))` before stringifying). This library: `"Total: 0.30000000000000004"` (shortest round-trip, as `JSON.stringify` actually does). **Different.** The doc comment is documentation-correct (the docs say `JSON.stringify`; the reference departs from its own docs), but `&` casts through `$string`, so every string-concatenated float changes appearance. This will be the first divergence anyone notices.

2. `$string(id)` or `id & ""` with `id = 1234567890123456789`
   jsonata-js: `"1234567890123460000"` (15 significant digits, then rounded). This library: `"1234567890123456789"`. **Different, in my favor.** This is the motivating case and it is a real gain; jsonata-js is simply wrong here.

3. `$.ts_ns / 1e6` with `ts_ns = 1758000000000000123` (a nanosecond timestamp; every one of these is above 2^53 today)
   jsonata-js: `1758000000000.0002` or thereabouts, silently rounded on parse. This library, per the doc comment: `1e6` is a float64 literal, so "any float64 operand yields float64; the operation is an error (D1001) when its exact result is not representable as float64," and 1758000000000000123 is not exactly representable, so **D1001**. Now write it as `$.ts_ns / 1000000`: both integers, division not exact, "float64 otherwise." Is that an exact rational rounded once (succeeds), or convert-then-divide with the same exactness rule (fails)? **Undefined by the doc comments.** Note also that read literally, "exact result not representable as float64" would reject `0.1 + 0.2`, whose exact sum is not a double; the README says that succeeds. The sentence means "an integer operand that does not convert exactly," and it needs to say so.

4. `$round(2.675, 2)`
   jsonata-js: `2.68` (the reference shifts the decimal through a string, `"2.675e2"`, so the midpoint is exact before half-to-even). This library, stated: `2.67`. **Different.** The docs' own worked examples are decimal, and "round half to even" applied to what the author typed gives 2.68. Users doing currency rounding will hit this and call it a bug.

5. `$keys({"b": 1, "10": 2, "a": 3, "2": 4})`
   jsonata-js: `["2", "10", "b", "a"]` (V8 hoists integer-like keys in numeric order). This library: literal constructors are `*Object`, so `["b", "10", "a", "2"]`; the same object arriving as a `map[string]any` input: `["10", "2", "a", "b"]`. **Different, and three-way.** Same expression, same JSON, different answer depending on the Go container the caller used. Grouping keyed by numeric strings (`Order{$string(OrderID): Price}`) and `$string` of such objects follow suit.

6. `$match(text, /(?<=ID:)\d+/)` or `$replace(s, /(\w)\1/, "$1")`
   jsonata-js: works. This library: "a regex literal that does not compile fails at Compile," and Go's `regexp` is RE2, so both fail. **Undefined by the doc comments:** the regex dialect is never named. `$replace(s, /(\w+) (\w+)/, "$2 $1")` is the same in both, as documented.

7. `$split("😀👍", "")` and `$match("a😀b", /b/).index`
   jsonata-js: four surrogate halves; index 3. This library: `["😀", "👍"]`; index 2. **Different, doc-correct.** The docs say "characters"; the reference counts UTF-16 units in these paths.

8. `$uppercase("straße")`
   jsonata-js: `"STRASSE"`. This library, stated: `"STRAßE"` (simple case mapping). **Different.** The docs are silent, so the host wins, and the host's answer is the worse one for German text.

9. `{"a": x}` over input `[{"x": 1}, {"x": 2}]`
   jsonata-js: `{"a": [1, 2]}`; an object constructor over a sequence groups, and with a literal key every item lands in one group. This library: the value is presumably the same, but `ReasonArrayInput` says "the constructor maps over it and the result is an array." **That description is wrong for the language.** The result is an object, and since each field's value is just its subexpression over the whole input, selection is possible; the reason should not exist.

10. `$states.input.note ?? "n/a"` (Step Functions habit) where `note` is JSON `null`
    jsonata-js 2.1: `??` is sugar for `$exists(lhs) ? lhs : rhs`, and `$exists(null)` is true, so `null`. This library, stated: `null`, with `?:` falling back. **Same.** I would still cite the pinned doc line, because "coalescing" carries C#/JS baggage and this sentence is doing load-bearing work.

11. `9007199254740993 = 9007199254740992`
    jsonata-js: `true`. This library: `false`. **Different, doc-correct**, and the kind of thing a fixture in the reference suite may assert the other way.

## What a JSONata user gains and loses

Versus jsonata-js, gained: integers are exact, so IDs, cents, and nanosecond timestamps survive selection, comparison, and `$string`, which is the single biggest defect of the reference for API work; strings are code points everywhere, not code units in some functions and code points in others; no silent overflow or NaN; a bounded evaluator (no expression can hang or exhaust the host, which Node-RED and Step Functions authors have wanted for years); bytes as a first-class value that presents as the binding's string; `Prepare/Select` and `Reads()`, which have no counterpart anywhere; `$now` and `$millis` fixed per evaluation is preserved; object equality ignoring order is preserved; duplicate-key JSON is rejected instead of last-wins.

Versus jsonata-js, lost: the 15-significant-digit pretty rendering of floats through `$string` and `&`, which people rely on more than they know; decimal-intuitive `$round`; full Unicode case mapping; JS regex features (lookaround, backreferences, Unicode-aware `\s`); integer-key hoisting (nobody will miss it, but it changes `$keys` and `$string` output); `registerFunction`/`assign` of host functions and the Node-RED extras (`$env`, `$flowContext`, `$moment`), by design; `expr.ast()`; and the mental model that one expression has one answer. I now have to know which member I am on and which Go container my caller used.

Versus a faithful port (blues/jsonata-go style): a port gives me every jsonata-js quirk verbatim, including the 2^53 loss and the truncated `$string`, and passes the upstream suite by construction. For general scripting that is comforting. For OpenBindings' stated job (exact carriage of decoded payloads) it is disqualifying, so the trade is right; what a port has that this lacks is a single answer to "what will my expression do," and this design needs a ledger that reads like an author-facing dialect note to replace it.

## What I would change

1. **Name the regex dialect in the package doc, and declare its divergence.** Under "Strings and bytes": "Regex literals use Go's `regexp` (RE2 syntax) with the `i` and `m` flags. Lookahead, lookbehind, and backreferences are not supported and fail at Compile (S-code). `\d`, `\w`, and `\s` are ASCII; Unicode classes (`\p{L}`) are accepted. Case-insensitive matching uses Unicode simple folding." Add the same paragraph to the README's "The split," because the regex dialect is host-defined under this class and that is the sharpest portability edge in the whole design.

2. **Rewrite the mixed-representation arithmetic rule and settle integer division.** Replace "the operation is an error (D1001) when its exact result is not representable as float64" with: "When one operand is float64, an integer operand must convert to float64 exactly (magnitude at most 2^53); otherwise the operation is an error (D1001). Division of two integers that is not exact computes the exact rational quotient rounded once to the nearest float64, so `1758000000000000123 / 1000000` succeeds." Then reconsider whether `bigInt * 1.5` should be a refusal at all when the caller can `$number($string(x))` nothing out of it; at minimum the doc must give the workaround.

3. **Fix the README's portable-core claim and add "Differences you will hit."** `$string` and `$keys` are in the listed core, and `$string` on floats is precisely where the reference departs from its docs (item 1), while `$keys`/`$merge`/`$sift` order departs for numeric-like keys (item 5). Change "agree without exception" to "agree for integer values and objects without integer-like keys," and add a short section listing, with one line each: float rendering through `$string` and `&`, `$round` on decimal midpoints, `$keys` order (including that map inputs are observed sorted), `$uppercase` on ß, regex dialect, and literal integers above 2^53.

4. **Correct `ReasonArrayInput` or remove it.** Proposed comment: "ReasonArrayInput: the input is a sequence, so the constructor groups every element under each literal key; the field's value expression is evaluated over the whole input." Then decide whether that is actually a bar to selection; by the language's grouping rule it is not, and the empty-input case (value evaluated over undefined) is the only wrinkle.

5. **Two small semantic corrections.** `$fromMillis`/`$toMillis`: "use UTC unless a picture string specifies an offset" should read "use UTC unless the `timezone` argument (`$fromMillis`, `$now`) or the parsed text (`$toMillis`) supplies an offset; a picture only renders the offset." And `$string` of a function: the pinned docs say functions cast to the empty string; if T1006 is meant for an expression whose *result* is a function, say that and pick a code that means it.

## Things the doc comments leave unclear to a JSONata user

- Which regex syntax, which flags, and what happens to `\s`, `\b`, and case folding.
- `$number("12345678901234567890")` and `$number("1.50")`: int64, *big.Int, or float64?
- Result representation of `$floor`, `$ceil`, `$abs`, `$sqrt`, `$power`, `$sum`, `$average`, `$max`, `$min`. Is `$power(2, 64)` a *big.Int? Is `$power(2, -1)` "integer with integer yields an exact integer"?
- Non-exact integer division: exact-rational-then-round, or convert-then-divide with the exactness refusal (item 3 above).
- Whether `0.1 + 0.2` succeeds, given the literal wording of the float64 rule.
- `$formatNumber`, `$formatInteger`, `$formatBase` with *big.Int arguments; and whether `$formatNumber` picture parsing is locale-free.
- `$base64decode` on a `[]byte` input: raw bytes as a string? refused if not UTF-8? And `$length`/`$contains` on `[]byte` operate on the base64 text, which should be said outright.
- Whether a binding named `string` or `count` shadows the built-in, and what a bound Go value that is not admitted does (error at bind or at first use?).
- Whether a result that is a function value (`$string` unapplied, a lambda) is an error, and under which code.
- Result container of `$each`, `$sift`, `$merge`, `$spread`, and the transform operator (always `*Object`?), and whether the transform operator deep-copies nested maps or carries them by identity (the docs promise a clone; the ownership section allows aliasing).
- The order an `ObjectView.Range` reports, and whether `$keys` on a view uses it (protobuf field order?).
- String ordering under `^()` and `<`: code point (stated for ordering) versus jsonata-js code unit; and sort stability.
- What error code a failing `Access`, `ObjectView.Get`, or `ArrayView.At` surfaces as, and whether it is wrapped so `errors.Is` still works.
- Duplicate member names in JSON input: rejected by `Unmarshal` (stated) but what about `json.Unmarshal`-produced maps (last wins upstream); the boundary now differs by decoder.
- `MaxRecursion` 1024 against recursive user functions and deep descendant walks; `MaxOutputNodes` 1M against `$map` over large arrays; `MaxWork` 8M against `$sort` and `$distinct` on large inputs. Practitioners need the rough element counts at which defaults trip.
- Whether `$assert` and `$error` in a block prelude are the sanctioned guard pattern under `Select` (the text implies yes; say it as advice).

## On the concept

As a framing, "JSONata like regex, native per host, documentation as authority" is one I would trust more than the status quo, because the status quo is a fiction. jsonata4java, jsonata-go, jsonata-rs, and the two managed hosts I use already disagree with jsonata-js and with each other; they just pretend otherwise by calling the reference the spec. And the reference disagrees with its own docs in ways that hurt real work: `$string` truncating IDs, `$split` shredding emoji, 2^53. Pinning the docs and being honest about the host is the only stance that lets a Go engine keep a 64-bit ID intact without apology. For the OpenBindings problem, I would not take a faithful port; a faithful port carries the exact defects the project exists to avoid.

But the regex analogy cuts both ways, and the README only tells the flattering half. Regex is the cautionary tale of notation fragmentation: PCRE, RE2, JS, POSIX, each declaring "what it does not support," and every practitioner carrying a mental table of which lookbehind works where. That is what "each member declares its divergences" produces. The JSONata docs are tutorial prose, not a spec, and the places they are silent are not the exotic corners; they are float rendering, midpoint rounding, key order, case mapping, and the regex dialect, which is to say the things a transform author touches daily. "Documentation wins where it speaks, value model where silent" therefore hands the host far more of the language than the README's `1/2` example suggests. The design is sound; the description of how much is host-defined is undersold.

What would make me trust it as an author rather than as a library consumer is a divergence ledger written for people who write JSONata, not for people who maintain fixtures, plus the portable-core paragraph corrected so it stops promising that `$string` and `$keys` are safe. Is this what JSONata should have been? Yes on numbers and strings, unambiguously. On the rest, it is what JSONata always was once you looked closely, now with the honesty of saying so; the risk is that saying so out loud invites members to be casual about the silences, and the value of the language is that authors did not have to think about them.
