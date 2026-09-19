# Iteration 1 cold read: jsonata practitioner

> Given only the class README and go/jsonata + go/text at feb3743; told not to read design/. Verbatim.

## Grades

| Criterion | Grade | One-line justification |
|---|---|---|
| Idiomatic fit for Go | A- | ctx-first, functional options, `iter.Seq`, `regexp`-shaped `Compile`/`MustCompile`/`String`; the `(value, present, err)` triple is unusual but is the honest encoding of JSONata's undefined. |
| Ergonomics of the common path | B+ | `Compile` once, `Eval` many is right; the tax is the `map[string]any` / `*Object` two-arm switch on every object result, and no way to bind a reusable JSONata function library without string concatenation. |
| Correctness and footgun risk | B- | The number model contradicts its own "never of the representation" rule at the int64 boundary; regex dialect, `$round` basis, `$string` escaping, and `$replace` group syntax are all unstated, and each is a place a real expression changes meaning. |
| Performance headroom the API permits | A- | By-reference admission, `Reads()` for lazy fetch, memoizing selective evaluation, bounds on the compiled artifact, and `Access` for protobuf without copying; nothing in the surface forces a tree materialization. |
| Concept soundness | B | Host-native values are the right call for API payloads; "documentation as authority" overclaims because the JSONata docs delegate to JavaScript in exactly the places that matter (`$string` is defined via `JSON.stringify`, regex syntax via JS) and are silent on order, rounding, and the number model. |
| Overall | B+ | I would use it inside OpenBindings today for transforms over IDs and money; I would not assume an expression I tested in the Exerciser or in Step Functions means the same thing here without reading the four unclear sections below. |

## Would my expressions mean the same thing?

Inputs below are assumed to arrive as `encoding/json` decodes them (`map[string]any`, `json.Number`) unless stated, because that is how a decoded API body reaches this evaluator.

**1. Rename.** `{ "orderId": id, "total": amount }` with `id: 9007199254740993` in the JSON.
Same shape, different value. jsonata-js: `orderId` is `9007199254740992` (the double). This library: `int64(9007199254740993)`. Different, in this library's favour, and it is the whole reason to want it. Note the README's rule 6 permits a declared divergence only "where the value model cannot represent a reference value"; here the host is more precise than the reference, not less, and the wording does not cover that direction.

**2. Nested constructor with a carried subtree.** `{ "customer": { "name": name, "address": address } }`.
Same values. `address` is the input map, aliased, not an `*Object`. When the caller encodes, `address` keys come out sorted (Go maps always do), whereas jsonata-js emits document order. As JSON objects these are equal; as text they are not. Equal-as-value, different-as-bytes.

**3. Predicate on an ID.** `orders[id = 9007199254740993].total`.
Different. jsonata-js collapses both `...992` and `...993` to the same double, so this predicate matches two distinct records if both exist. Here it matches exactly one. This is a correctness gain, but a user who "knows" the Exerciser result gets a different count.

**4. Envelope building.** `$merge([ $sift($, function($v, $k) { $k != "password" }), { "ts": $now() } ])`.
Same membership, different order. `$sift` walks a map in sorted key order, so the result `*Object` is sorted-then-`ts`; jsonata-js is document-order-then-`ts`. If anything downstream compares `$string(...)` of this envelope (a signature, a cache key, a test fixture), the strings differ. Also `$now()` format is left to the JSONata docs' example (`2017-05-15T15:12:59.152Z`); the doc comments never say milliseconds are always three digits.

**5. `$keys` and `$each`.** `$keys($)` over `{"b":1,"a":2}`.
Different value, not just different text. jsonata-js: `["b","a"]`. This library over a map: `["a","b"]`. This is an array, so the difference is observable inside the expression (`$keys($)[0]`). The README lists `$keys` in the "portable core" with "identical results from every member"; that is false for map inputs. It is only true if the caller feeds `*Object` (for example through `text.Decode`). Also, jsonata-js orders integer-like keys first (`{"b":1,"1":2}` gives `["1","b"]`), a JS quirk the docs never mention; `*Object` will give insertion order. Document that this quirk is intentionally not reproduced.

**6. Grouping.** `Order{ status: $sum(total) }`.
Same. Group keys appear in first-encounter order in both, and sums over the same doubles match. Exception: `Order{ $string(year): $count($) }` where keys look like integers; jsonata-js sorts them numerically first, this library keeps encounter order.

**7. `$match` with a regex.** `$match(email, /^([^@]+)@(.+)$/)`.
Same. `$match(s, /(?<=\$)\d+/)` or `/(\w)\1/`: undefined by the doc comments. Nothing says which regex dialect is compiled. If it is Go `regexp` (RE2), lookbehind and backreferences fail at `Compile`; the JSONata docs say the syntax is JavaScript's. And `index` in code points differs from jsonata-js's UTF-16 offset for any text with astral characters. `$replace(phone, /(\d{3})(\d{4})/, "$1-$2")`: same, provided the implementation parses `$N` the JSONata way; Go's own `Expand` reads `"$1x"` as group `1x`, so `$replace(s, /(\d+)/, "$1x")` is the first thing I would test.

**8. Money.** `$round(price * qty * (1 - discount), 2)` with `price: 2.675, qty: 1, discount: 0`.
Different, and the doc comments do not let me predict which way. jsonata-js does the documented half-to-even on the decimal string (`"2.675e2"` becomes 267.5, rounds to 268, gives `2.68`). If this library rounds the binary value, `2.675 * 100` is `267.49999999999997`, and the answer is `2.67`. "$round is half to even" names the tie rule but not what a tie is. The integer path is a gain: `price_cents * qty` with `int64` cents stays exact, and `1250 / 100` is `12.5` in both.

**9. `$string` of a number.** `$string(3.0)` is `"3"` in both; `$string(1e21)` is `"1e+21"` in both (Go's `encoding/json` mirrors ES6 float formatting, including `1e-7` cleanup). `$string(100000000000000000000000)`: jsonata-js `"1e+23"`, this library `"100000000000000000000000"` because the literal is `*big.Int`. Different. And `$string(price)` where `price` arrived as `json.Number("1.50")` is `"1.5"` (function of value), while `price` copied through `text.Evaluate` is emitted as `1.50` (carried token). Same expression, two spellings of the same number depending on whether it went through `$string` or the encoder.

**10. Sorting.** `products^(>price, name)`.
Same for numbers (by value across `int64`/`float64`) and for BMP strings. Different for strings containing astral characters: this library orders by code point, jsonata-js by UTF-16 code unit, so `"！"` (U+FF01) and `"😀"` swap places. Rare in API payloads, real in product names.

**11. `??` default.** `nickname ?? name`.
Same, provided the docs' rule for null is followed. The doc comments are silent on `??` and `?:`, and because this library goes out of its way to make a `nil` binding null rather than absent (`WithBindings` with `"x": nil`), the difference between "null is absent for `??`" and "null is present for `??`" is exactly the case a Go caller will hit.

**12. Arithmetic that used to work.** `id * 1.0` with `id` above 2^53, or `$millis() * 1000000 * 10`.
Different: jsonata-js returns a rounded double; this library returns D1001. Refusal is defensible under rule 8, but the first case is a trap: `id * 1` succeeds and `id * 1.0` errors because the literal `1.0` is `float64`.

## What a JSONata user gains and loses

**Versus jsonata-js, gained:** exact integers past 2^53 (IDs, snowflakes, cents in `int64`) and exact big literals; decimal tokens carried untouched (`1.50` stays `1.50` when only copied); code-point string semantics for `$length`, `$substring`, `$split("")` and `$match.index` instead of surrogate-pair accidents; `Reads()` and `Prepare`/`Select`, neither of which exists anywhere in the JSONata ecosystem; per-expression bounds so a hostile transform cannot take the process down; typed errors with the JSONata codes plus line and column; evaluation directly over protobuf messages through `Access` without a JSON round trip; `errors.Is(err, context.Canceled)`.

**Versus jsonata-js, lost:** `registerFunction` and any host extension (no `$env`, no `$moment`, no `$uuid` unless the docs have it); document key order for anything that arrived as a map, which changes `$keys`, `$each`, `$spread`, `$string`, `$merge` and `$sift` results; JavaScript regex features, probably (unstated); silent success on overflow and on int-float mixing past 2^53; JS number formatting for large integers in `$string`; function values as results (the package doc lists output kinds and a function is not one, so `$f := function(){1}; $f` has no stated Go value); `JSON.parse`'s last-wins on duplicate keys in `text.Decode`; and, the big one, Exerciser and Step Functions parity: what I test there is no longer a guarantee of what runs here.

**Versus a faithful port:** a port gives me every quirk I have already learned to work around (2^53 rounding, integer-key reordering, UTF-16 offsets, string-shift `$round`) and gives me the test suite as proof. This library gives me correctness on the values OpenBindings actually carries and a declared-divergence ledger instead of silent parity. For transform expressions over API payloads I want this one. For anything I also need to run in Node-RED, a port.

## What I would change

1. **Name the regex dialect and the replacement syntax in the package doc.** Under "# Strings and bytes" add: "Regex literals and the pattern arguments of `$match`, `$contains`, `$split` and `$replace` are compiled by Go's `regexp` (RE2 syntax) with the `i`, `m` and `s` flags. Lookahead, lookbehind, backreferences and `\p{...}` outside RE2's set are compile errors (S0302). `$replace` replacement strings follow the JSONata documentation: `$N` refers to group N, `$$` is a literal dollar, and Go's `${name}` form is not recognized." If it is not RE2, say what it is. This is the single largest source of "my expression changed meaning" and the text says nothing.

2. **Make integer arithmetic a function of value, as the Numbers section promises.** Today `9223372036854775807 + 1` is D1001 but `(0 * 18446744073709551616) + 9223372036854775807 + 1` succeeds with a `*big.Int`, so the result depends on operand representation, which the same section says never happens. Replace "Integer with integer yields int64, or *big.Int when either operand is big; an int64 result that does not fit is an error (D1001), never a wrap and never a promotion" with "Integer with integer yields an exact integer: int64 when the result fits, *big.Int otherwise. Integer results are bounded by WithMaxWork, not by width." Promotion is not approximation, so rule 8 is untouched. Keep the mixed-with-float refusal, but state the `id * 1.0` trap in the doc with that exact example.

3. **Define `$round`'s tie.** Add to "# Numbers": "$round is half to even applied to the decimal value denoted by the number's shortest round-trip representation, shifted by precision, so $round(2.675, 2) is 2.68 and $round(2.5) is 2." That matches the documentation's intent and what every JSONata user has observed; rounding the binary value instead produces 2.67 and should be called out as a divergence if that is the choice.

4. **Specify `$string` of compound values and stop calling `$keys`/`$string` portable over maps.** Add: "$string of an object or array renders JSON with no whitespace and no HTML escaping (encoding/json's default escaping of <, > and & is not used), member order as this package observes it, numbers as above, []byte through the BytesEncoding in effect; with prettify true, two-space indentation." In the README, change the portable-core sentence to "Authors who stay inside it, and who supply order-preserving objects, get identical results" or drop `$keys` and `$string` from the list. Also reconcile `text.Evaluate`, which says `[]byte` is emitted with `base64.StdEncoding` even when the caller passed `WithBytesEncoding(base64.URLEncoding)` in `opts`; the same evaluation would then spell one byte slice two ways depending on whether `$string` touched it.

5. **Let bindings carry JSONata functions.** The closed environment is right, but "no host functions" does not have to mean "no shared library". Extend `WithBindings`: "A value of type *Expression is evaluated once, over the same input and options, when the variable is first read; this is the way to supply a library of JSONata functions (`$fmtMoney := function($n) {...}`) to many transforms without concatenating source." And state what a function-valued result is: either "a result that is a function is an error (T1006-style engine code)" or return an opaque `Function` type; today it is unspecified.

## Things the doc comments leave unclear to a JSONata user

- Which regex dialect, which flags, which constructs are unsupported, and what error code a rejected pattern gets.
- `$replace` group references (`$N`, `$$`), and whether a Go-style `${1}` is accepted or rejected.
- Whether `$match.index`, `Error.Position` and `$substring` all agree on code points (Position says "characters"; say code points).
- What `$round` rounds: binary value or shortest decimal; behaviour with negative precision.
- `/` when division is inexact: are the operands converted to `float64` (refusing an `int64` past 2^53) or is the quotient computed exactly and rounded once? `9007199254740993 / 2` is representable as a double even though the dividend is not.
- `$sum`, `$average`, `$max`, `$min` over mixed `int64`/`float64` arrays: accumulation type and order, whether an int overflow inside `$sum` is D1001, and whether `$average` of integers that divides exactly returns `int64` or `float64`.
- `$number` on the documented hex/octal/binary string forms and on `*big.Int`; whether `$number("1e400")` is D3030 or D1001.
- `$string` of objects and arrays: escaping, prettify indentation, rendering of `json.Number` (value or token), `*big.Int`, and `[]byte`.
- `$keys` sort order for maps (byte order? code point? locale never?), and `$keys` over an array of objects: union in which order.
- `$merge` when a later object repeats an earlier key: does the member keep its original position (JS behaviour) or move to the end of the `*Object`.
- The transform operator `~> | ... |`: whether the deep copy converts nested maps to `*Object` or aliases them, and whether the copied root is an `*Object` with sorted keys.
- `??` and `?:` with a null (present, nil) left operand, since bindings can be null but not absent.
- What Go value an expression that evaluates to a function returns, and what `$type` and `$string` of it are.
- `$now()` exact format (fractional digits, `Z`), `$fromMillis`/`$toMillis` picture strings and time zone database source.
- `$trim` whitespace definition (JS `\s` is Unicode; Go `unicode.IsSpace` is close but not identical), `$pad` width in code points.
- `$uppercase`/`$lowercase` with "simple case mapping": `ß` stays `ß`, dotted/dotless i not locale-aware; say so.
- `$distinct` and `in` across representations: which representation survives dedup; `in` with `[]byte` members.
- `$split` with an empty separator (code points, presumably), with a regex, and with a limit.
- `$boolean` of an empty `[]byte`.
- `$eval` of a JSON literal: does the parsed object come back as `*Object` with `json.Number`, and do the bounds count its parse.
- `Reads()` in the presence of `$$` inside functions, wildcards `*` and `**`, the parent operator `%`, `$lookup` with a literal key, and `$` alone (is that `[[]]`?).
- Whether an impure call in a block prelude (`$seed := $random()`) leaves the plan selective, since the prelude is shared; the text implies yes but only says the field's subexpression must be pure.
- Whether `WithBindings` values pass through the same admission and `Access` as the input.
- Whether `Compile` rejects `$registerFunction`-style unknowns at compile time (T1006) or at evaluation.

## On the concept

The regex analogy is seductive and, from where I sit, it is also the cautionary tale. Regex "notation" fractured into PCRE, RE2, JavaScript, POSIX and .NET dialects precisely because each host implemented it natively and declared what it did not support, and every one of us has shipped a pattern that meant something else on the next platform. JSONata's entire pitch, the reason it got into Step Functions and Node-RED and a dozen ports, is that one expression evaluates to one answer everywhere, and the shared test suite is the contract that makes that true. So no, I do not think "native per host, documentation as authority" is what JSONata always should have been. The documentation cannot be the authority when it defines `$string` as "converted using `JSON.stringify()`" and regex syntax as JavaScript's, and says nothing about key order, string ordering, rounding ties, or what a number is. In those places this library's own doc comments become the specification of a dialect, and the README's claim that the docs "win where they speak" quietly means "this package decides where they do not".

That said, the ecosystem is already fractured at the value layer, just silently. Step Functions restricts and extends the function set; Node-RED adds `$env` and `$moment`; every port that "reproduces the reference" also reproduces JavaScript's double as the only number, which is wrong for the values this project carries. An evaluator that refuses instead of rounding, carries a 64-bit ID untouched, and writes down every departure against a pinned fixture is more honest than a port that passes the suite while corrupting IDs above 2^53. For OpenBindings transforms specifically, where the inputs are decoded API payloads and the expressions are the small, order-insensitive subset the README calls the portable core, I would trust this over a faithful port, and the `Reads()` and selective-evaluation features are things I have wanted for years.

What I would not trust is the framing that this is simply JSONata. It is JSONata with an exact-integer value model, a Go regex dialect, sorted map keys, and undefined rounding ties, and the README should say that plainly rather than let "documentation as authority" imply that a reader of docs.jsonata.org can predict its output. If the number, regex, rounding and `$string` sections are nailed down as concretely as the absence and selection sections already are, and if the portable-core claim is narrowed to what is actually portable, then this is a better tool for its job than a port would be, and a JSONata user can read the package doc once and know exactly which of their habits to unlearn.
