# Panel cold read: jsonata practitioner

> 2026-09-18. Given only the class README and the vetted API stub; told not to read anything else. Verbatim.

## Grades

| Criterion | Grade | One-line justification |
| --- | --- | --- |
| Idiomatic fit for Go | B+ | Compile/MustCompile, functional options, ctx, `errors.As`: fine; the `(value, present, err)` triple is unusual but it is the honest shape for undefined-vs-null. |
| Ergonomics of the common path | B | `Eval(ctx, src, input)` plus a bindings map is what jsonata-js users expect; but every constructed object comes back as `*Object`, so every consumer grows a conversion layer the doc comment does not acknowledge. |
| Correctness and footgun risk | C- | Map-iteration order leaks into `$keys`/`$each`/`$merge`/transform output nondeterministically; numeric literal typing is unspecified, so `[id = 9007199254740993]` may silently never match; `$number` grammar, `$string` float formatting, `-0`, and regex dialect are all left to "the host", which in practice means "whatever strconv/regexp do". |
| Performance headroom the API permits | B+ | By-reference admission, no JSON text on the path, `Reads()`, and Prepare/Select with memoization are exactly the right levers; `$now`/`$random` handling under selection is unaddressed. |
| Concept soundness | C | The value-model split (exact integers, ordered objects, bytes) is the genuinely good idea; "host wins where the docs are silent" is not, because the JSONata docs are silent about almost everything a practitioner relies on, and membership rule 6 (verified against the reference suite) quietly contradicts "documentation as authority". |
| Overall | B- | I would use this for exact IDs and would fight it everywhere else unless the silence rule is flipped. |

## Would my expressions mean the same thing?

Input for these is a decoded JSON body or protobuf message; assume `$$` is a `map[string]any` unless noted.

1. **Rename / reshape.** `{ "orderId": id, "customer": customer.name }`
   Same. Better, in fact: if `id` is an int64 above 2^53, jsonata-js already lost it at `JSON.parse` before the expression ran; here it is carried exactly. Output is `*Object`, which a JSON encoder handles; a Go consumer doing `v.(map[string]any)` will panic. That is not a semantic difference but it will be the first bug report.

2. **Nested constructor with per-item math.** `{ "lines": items.{ "sku": sku, "cents": qty * unitCents } }`
   Same, including the singleton trap (one item yields an object, not an array; `items[].{...}` fixes it in both). Difference in Go type only: `qty * unitCents` stays int64 here and is a double in jsonata-js. Invisible through JSON, visible to a protobuf encoder, and overflow past int64 is `E1001` here versus silent rounding there.

3. **Predicate on a large ID.** `orders[id = 9007199254740993]`
   jsonata-js: the literal and any input id are both 9007199254740992 after parsing, so this matches both ...992 and ...993 records. Go member: *undefined by the doc comment.* "Equality is by value across every admitted numeric type" covers the input side, but nothing says what Go type a numeric literal in the expression is. If the parser stores literals as float64 (as every port does), the literal is 9007199254740992.0, the input id is int64 9007199254740993, value comparison is false, and the predicate silently matches nothing. This one line decides whether the library delivers its headline promise.

4. **Envelope building.** `$merge([$$, {"status": "ok"}])` and `$sift($$, function($v, $k) { $k != "secret" })`
   Values: same. Order: *different and nondeterministic.* jsonata-js preserves the input key order. Here `$$` is a `map[string]any`; Go randomizes map iteration per range, so two evaluations of the same expression on the same input can produce `*Object`s with different member order, and `text.Evaluate` (if it decodes to maps, which the doc comment implies) will emit differently ordered JSON run to run. The README concedes "as the language requires" for constructed objects, then admits an input type that cannot honor it.

5. **Grouping.** `orders{customer: $sum(total)}`
   Same. Group order follows the array order in both. `$sum` of int64 stays int64; a sum overflowing int64 is `E1001` here versus a rounded double in jsonata-js. Acceptable, but the doc comment should say `$sum` participates in integer arithmetic rules.

6. **Regex.** `$match(email, /^([^@]+)@(.+)$/)` is the same. `$match(price, /(?<=\$)\d+(\.\d\d)?/)` is *different*: jsonata-js returns the match; Go's RE2 has no lookbehind, so this fails at compile time with a code the draft does not name. Also undefined: whether `index` in the `$match` result counts bytes, code points, or UTF-16 units. jsonata-js gives UTF-16 units; `$substring` counts code points in both; a Go member that reports byte offsets breaks `$substring($s, $m.index)` for any non-ASCII string.

7. **Money.** `$round(unitPrice * qty, 2)` with `unitPrice = 1.015, qty = 1`
   jsonata-js shifts by string (`Number("1.015e2")` is exactly 101.5), sees a midpoint, rounds half to even, returns 1.02. A Go member using the binary value computes `1.015 * 100 = 101.49999999999999`, which is below the midpoint, and returns 1.01. The documentation says "round half to even" and nothing about decimal versus binary midpoints, so by the README's own rule the host wins and the answer is 1.01. That is a real divergence in the one function every invoice transform calls.

8. **String of a number.** `$string(orderNumber)` with `orderNumber = 12345678901234567`
   jsonata-js: `"12345678901234568"`. Go int64: `"12345678901234567"`. Different, and a gain. But the documentation says `$string` uses `JSON.stringify` semantics, so for float64 the member must reproduce ES6 `Number::toString` (`1e+21`, `1e-7`, shortest round-trip). Go's `encoding/json` happens to match ES6 for float64, but `strconv.FormatFloat(f, 'g', -1, 64)` does not, and the doc comment does not say which. `$string(-0)`: docs-by-JSON.stringify say `"0"`; Go `encoding/json` says `"-0"`.

9. **Sorting.** `items^(>price, name)`
   Numbers: same, and mixed int64/float64 prices sort by value as promised. Strings: jsonata-js orders by UTF-16 code unit, Go by code point. They disagree only when an astral character (emoji, CJK extension B) is compared against a BMP character above U+E000. Rare, real, and the README's "textual value" does not say which.

10. **Default.** `config.timeout ?? 30` and `name ?: "anonymous"`
    Same. These are pure language semantics and the docs define them. One edge the doc comment leaves open: `WithBindings(map[string]any{"x": nil})` presumably makes `$x` null, not undefined, so `$x ?? 30` gives 30 in both but `$exists($x)` is true here and would be false if the caller meant "unbound".

Bonus: `$number(rawId)` with `rawId = "18446744073709551615"`. jsonata-js: 18446744073709552000. Go member: uint64 exactly? `E1001`? float64? Undefined by the doc comment. And `$number("0x1F")`, `$number(".5")`, `$number("+5")`, `$number("Infinity")`, `$number("1_000")`: jsonata-js rejects all five with D3030 via an anchored JSON-number regex; Go's `strconv.ParseFloat` accepts every one of them (hex floats, leading dot, sign, case-insensitive Inf/NaN, underscores). The docs only say "a valid string representation of a number". If the host decides what "valid" means, `$number("Infinity")` becomes a non-finite result and a D1001 instead of a D3030, and `$number("0x1F")` becomes 31.

## What a JSONata user gains and loses

**Gains versus jsonata-js:**
- Exact integers. Snowflake IDs, protobuf int64/uint64, and cents in int64 survive selection, predicates, `$string`, and arithmetic. This alone justifies the library for the OpenBindings use case; jsonata-js cannot do it at any setting because `JSON.parse` has already rounded.
- Insertion-ordered constructed objects as a first-class type with `MarshalJSON`, rather than relying on V8's property-order accident.
- Refusal on overflow and non-finite results instead of `Infinity` sneaking into a payload.
- A closed environment. As a Step Functions user this is familiar and reassuring; there is no `$env` to leak the host.
- Selective evaluation and `Reads()`, which have no analogue in jsonata-js and matter when the input is fetched lazily.
- Context cancellation and budgets (`WithMaxRecursion`, `WithMaxOutputNodes`) instead of jsonata-js's timeboxing hack via a custom function.

**Losses versus jsonata-js:**
- Regex. Lookahead, lookbehind, backreferences, and JS-specific escapes are gone unless the member ships its own backtracking engine or translates and declares. Every `$match`/`$replace`/`$contains` I have written against messy vendor strings uses at least one of these.
- Custom functions. Node-RED users lose `$env`, `$flow`, `$global`; Step Functions users lose `$states`, `$partition`, `$hash`, `$uuid`. Declared and intentional, but it means expressions do not port from either environment without rewriting.
- Predictability where the docs are silent. Today I can answer "what does `$number(".5")` do" by trying it in the exerciser. Under this library the answer is "read Go's strconv", and the README has decided that is correct.
- Key order of input objects, unless the caller hands in `*Object`. jsonata-js users rely on `$keys($$)` order constantly, whether or not the docs promise it.

**Versus a faithful port** (a Go port that reproduces jsonata-js including doubles):
- Gains: everything under exact integers above; the faithful port would round IDs just like the original.
- Losses: none that a faithful port would give you, *provided* the silence rule is flipped so the reference decides the non-value questions. As drafted, the faithful port wins on `$number` grammar, `$round`, `$string` formatting, string ordering, and regex dialect, and the class wins on values. There is no reason those have to trade off.

## What I would change

1. **Flip the tiebreaker.** Replace the README's "the value model wins where the documentation is silent" with: "Where the documentation is silent on *what is done*, the reference implementation decides. The value model decides only what a value is and how primitives combine, and is the only ground on which a member may depart from the reference suite. A departure on any other ground is a defect." This removes the contradiction between "documentation as authority" and membership rule 6, and it stops `$number`, `$round`, `$string`, string ordering, and regex grammar from meaning "whatever Go's standard library does".

2. **Specify literal typing and `$number` in the package doc, as a "# Numbers" section.** Concrete wording: "A numeric literal with no fraction and no exponent is int64 when it fits, uint64 when it fits only there, and *big.Int otherwise. Any other literal is float64; one that is not finite as float64 is S0102. `$number` accepts exactly the JSON number grammar (an optional leading minus, no leading plus or dot, no hex, no Inf or NaN, no separators); an integral string in range yields int64 or uint64, any other accepted string yields float64, anything else is D3030. `$string` renders integers as decimal digits and float64 as ES6 Number::toString, so `-0` renders as `0`." Without this, expression 3 above is undefined.

3. **Make object order deterministic and honor it in `text`.** Change the `text` doc comment to: "Objects decode to `*jsonata.Object`, preserving member order; numbers decode to exact tokens." And add to the `# Values` section: "A `map[string]any` has no member order; the engine iterates it in sorted key order, and `$keys`, `$each`, `$spread`, `$string`, `$merge`, `$sift`, and the transform operator all observe that order." Sorted is not what jsonata-js gives, but it is deterministic and declarable; random is neither.

4. **Declare the regex dialect at the API.** Add to the package doc: "Regex literals use JSONata's JavaScript syntax with flags `i` and `m`, translated to Go's RE2. Lookahead, lookbehind, and backreferences are refused at compile time with code `CodeUnsupportedRegex` ("E1005"). `$match` `index` counts characters, consistent with `$substring` and `$length`." Add the constant. If the answer is instead a backtracking engine, say so; a practitioner needs to know before writing `$replace`.

5. **Tighten `Error` for the codes I actually handle.** Drop `E1001` and use the language's `D1001` ("Number out of range") for integer overflow as well as non-finite results, because every JSONata error handler I have seen already keys on D1001 for numeric range. Add `Value any` (jsonata-js errors carry the offending value, and `$error()` payloads need it). Make `Position` a character offset or add `Column`, since expressions with emoji in string literals will otherwise report byte offsets that do not line up with any editor. And capture the `$now()`/`$millis()` timestamp at `Prepare`, so those functions are selectable under the documented rule that all calls in one evaluation return the same instant; "unsupported-effects" should then be reserved for `$random()` and `$eval()`.

## Things the doc comments leave unclear to a JSONata user

- What Go type a numeric literal in the expression is (int64? float64? does `1.0` differ from `1`? is `1e2` an integer?).
- Whether `$number` follows the JSON grammar or `strconv` (`".5"`, `"+5"`, `"0x1F"`, `"1_000"`, `"Infinity"`, `"NaN"`), and what type it returns for `"123"`, `"1e400"`, `"18446744073709551615"`, and `"12345678901234567890123"`.
- What `$string` produces for int64, uint64, `*big.Int`, `json.Number("1.0")`, `json.Number("1E2")`, float64 `1e21`, `1e-7`, and `-0`; and what `$string(x, true)` indents with, since the docs point at `JSON.stringify` and Go's `MarshalIndent` spacing differs.
- What `$string` produces for a `map[string]any` input: sorted keys (Go's `encoding/json`) or iteration order.
- Whether `json.Number` is a number kind or a string kind, when it is parsed, and what happens to an invalid `json.Number("abc")` on admission versus on use.
- Whether `int64` and `uint64` mix, what the result type is, and whether `uint64(1) - 2` is an overflow or an int64.
- Whether `-0` (float64) survives carriage, compares equal to `0`, and what `$type` and `$boolean` say for it.
- What `$type` returns for `[]byte`, `*big.Int`, `json.Number`, `*Object`, a lambda, a partially applied function, and a typed nil (`[]any(nil)`, `map[string]any(nil)`, `(*Object)(nil)`, `(*big.Int)(nil)`).
- Whether a typed nil is null, an empty array, an empty object, or an unsupported value.
- What "used as a string" means for `[]byte`: does `$type`, `$exists`, `=` against a string, `$length`, `$boolean` of empty bytes, `&` concatenation, or `in` count? Does `$length(bytes)` return the byte count or the base64 length?
- What unit `$length`, `$substring`, `$pad`, `$split(s, "")`, and `$match.index` use: bytes, runes, UTF-16 units, or grapheme clusters. jsonata-js uses code points for the first three and UTF-16 units for `index`.
- Whether `$uppercase`/`$lowercase` use full Unicode case mapping (`"ß"` to `"SS"` in JS) or Go's per-rune simple mapping (`"ß"` stays `"ß"`).
- String ordering for `<`, `>`, `$sort`, and `^()`: code point (Go) or UTF-16 code unit (JS).
- What regex syntax is accepted, which flags (`i`, `m` per the docs; Go also has `s` and `U`), how `\/`, `\u{...}`, `\cX`, named groups `(?<n>)`, and `$1`/`$<name>`/`$$` in `$replace` replacement strings are handled, and what error code an unsupported construct raises.
- Whether `$round` treats the argument as its binary value or its shortest decimal representation (the `1.015` case).
- Whether `$formatNumber`, `$formatInteger`, `$parseInteger`, `$now(picture)`, `$fromMillis(picture)`, and `$toMillis(picture)` implement the XPath picture strings the docs reference or Go's `time` layouts.
- Whether `$eval` exists, and if so how `WithMaxExpressionBytes` and `WithMaxDepth` apply to a string compiled at run time.
- Whether `WithMaxRecursion` counts tail calls, given that the docs describe tail-call optimisation for recursive lambdas.
- What `$now()` returns across two `Select` calls on one `Evaluation`, and whether `Prepare` captures the timestamp.
- Which functions make a subexpression impure for the plan ("unsupported-effects"): `$now`, `$millis`, `$random`, `$eval`, `$error`, `$assert`?
- Whether a block `($a := ...; { "x": $a })` is selectable or reported as "dynamic-shape".
- What is returned when the whole result is a function value (a lambda or partial application), which is none of the listed output kinds.
- Whether the object constructed by the transform operator `~> | ... |` over a `map[string]any` input is an `*Object`, and in what order.
- Whether `=` and `in` and `$distinct` deep-compare a `map[string]any` against an `*Object` with the same members as equal.
- Whether `WithBindings` can bind an undefined value, and whether a binding value outside the admitted set goes through `Access`.
- Which error code a value with `Access.Kind == KindUnknown` raises when merely carried versus when used.

## On the concept

The regex analogy is the part I trust least, precisely because I use regex. Regex is the canonical example of a notation that fractured by host: PCRE, RE2, JavaScript, POSIX, and every practitioner has a scar from a pattern that worked in one and silently meant something else in another. Holding that up as the model for JSONata is holding up the failure mode. The analogy also misstates why regex fragmentation is tolerated: regexes live inside one program and never leave it. JSONata transforms in OpenBindings are the opposite. They are published in a document that the Go SDK and the TS SDK both evaluate, and this project's own orientation says those SDKs must be behaviorally identical at the boundary. A class whose README says "no parity between members" and "where the models differ, the members differ, and that is correct" has declared that `id + 1` on an ID above 2^53 may legitimately give two different answers to the same document. For the class that is a principle; for a document author it is a portability bug with a philosophy attached. The "portable core" section is an honest admission that the safe subset is small, and it is an observation, not a guarantee.

The value-model split, on the other hand, is what JSONata always should have had. The language never needed to inherit IEEE doubles, unordered-until-V8 objects, and no byte type; those are JavaScript's accidents, and every port has copied them out of deference. A Go member that carries int64 exactly, keeps constructed objects ordered, and refuses overflow is strictly better for the payloads this project handles, and no faithful port can deliver it. But the README overreaches from "the host defines what a value is" to "the host decides whenever the documentation is silent," and the JSONata documentation is silent about nearly everything a practitioner relies on: the grammar `$number` accepts, how `$round` finds a midpoint, how floats print, how strings order, which regex features exist, what `index` counts. In each of those places I can predict jsonata-js and I cannot predict Go's standard library without reading it. Worse, membership rule 6 says a member is verified against the reference suite and may depart only where the value model cannot represent a reference value, which means that in practice the reference implementation, not the documentation, is the authority for everything except value representation. The README should say that out loud instead of the reverse.

So: would I rather have a faithful port? Not quite. I want a port that is faithful to jsonata-js in everything it does and unfaithful only in what its values are: exact integers, ordered objects, real bytes, refusal on overflow. That is a narrower and more defensible claim than "native per host", it keeps one expression meaning one thing across the Go and TS members wherever the values are representable in both, and it lets the declared-divergence ledger be short and entirely about numbers. Ship the value model; drop the regex framing; make the reference the tiebreaker; and specify the literal typing before anything else, because until that sentence exists nobody can tell whether `orders[id = 9007199254740993]` is the library's proudest feature or its quietest bug.
