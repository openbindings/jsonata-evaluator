# Declared divergences: Go member

Where this package's behavior departs from the reference implementation's
observable behavior at the pinned commit, and why. The authority column
names what decided the entry: **documentation** (the pinned documentation
says otherwise), **interpretation** (the documentation speaks but admits
more than one reading, and this package chose one), **value model** (the
documentation is silent and this package's value model decides), or
**engine** (a refusal this package adds under an E code). Entries will be
pinned to the fixtures they depart from once the shared suite lands in
`suite/`; a departure no fixture exercises gets a fixture this member adds.

| Behavior | Reference | This package | Authority |
| --- | --- | --- | --- |
| `$string` of a float, and `&` with a float | 15 significant digits: `$string(0.1 + 0.2)` is `"0.3"`, `$string(1234567890123456789)` is `"1234567890123456800"` | `JSON.stringify`'s rendering: `"0.30000000000000004"`; integers exact; notation thresholds at 1e-6 and 1e21 as ECMAScript specifies | documentation, which specifies `JSON.stringify` |
| `$round` on a decimal midpoint | rounds the decimal spelling: `$round(2.675, 2)` is `2.68` | rounds the binary float64: `2.67` | interpretation: the documentation gives half-to-even and only non-tie decimal examples; whether a tie is judged on the decimal spelling or the binary value is a reading of text that speaks, not a silence, and the float64 is identical on every host, so this is an algorithm choice; pending a ruling on the numeric model |
| Integers above 2^53 | binary64 throughout: `9007199254740993 = 9007199254740992` is true | exact: false; `$string` yields the digits | value model |
| `$power` with integer operands beyond float64 | `$power(2, 64)` is `18446744073709552000`; `$power(10, 400)` is an error (D3061) | exact `18446744073709551616`; exact within `MaxIntegerBits` | value model |
| Mixed integer and decimal arithmetic beyond 2^53 | rounds silently | error E1008 (`CodeInexact`) | engine; refuse rather than approximate |
| `$keys` and member order with integer-like keys | integer-like keys first, numerically: `["2","10","b","a"]` | insertion order for `*Object`, sorted byte order for maps (every key, not only integer-like ones) | value model: the documentation is silent; the reference's order is ECMAScript's own-property order |
| `$split(s, "")` and `$match.index` on astral characters | UTF-16 code units | code points | documentation, which says characters |
| String ordering (`<`, `$sort`, `^()`, `$min`/`$max` of strings) with characters beyond U+FFFF | UTF-16 code unit order: `"😀"` sorts before `"～"` | code point order: `"～"` sorts before `"😀"` | value model, consistent with characters as code points |
| Duplicate member names in decoded input | `JSON.parse` keeps the last | `Unmarshal` refuses (E1006) | engine; a binding decision this member's decoder makes rather than a language one |
| The range operator's size cap | D2014 above 1e6 elements | the general budget: E1002 under `MaxWork` or `MaxOutputNodes` | engine |
| A function-valued result | returned to the caller as a function object | error E1001 | engine |
| Regular-expression dialect | JavaScript | pending ruling | value model: the documentation is silent |
