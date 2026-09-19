# Declared divergences: Go member

Where this package's behavior departs from the reference implementation's
test suite, or from any other reference behavior this package has found,
and why. The authority column names what decided the entry: **documentation**
(the pinned documentation says otherwise), **incorporated** (an authority
the documentation names by reference, applied over this member's value
model), **interpretation** (the documentation speaks but admits more than
one reading, and this package chose one), **value model** (the documentation
is silent and this package's value model decides), or **engine** (a refusal
this package adds under an E code). Entries will be pinned to the fixtures
they depart from once the shared suite lands in `suite/`; a departure no
fixture exercises gets a fixture this member adds. An entry marked pending
records a question this package has not yet answered; the stub's current
text is not a declaration for it.

| Behavior | Reference | This package | Authority |
| --- | --- | --- | --- |
| `$string` of a non-integral float, and `&` with one | 15 significant digits: `$string(0.1 + 0.2)` is `"0.3"` | `JSON.stringify`'s rendering: `"0.30000000000000004"` | incorporated: the documentation specifies `JSON.stringify`, whose digit and notation rules this package applies over its own numbers |
| `$string` of an integer that is a float64 in the reference | `$string(1234567890123456789)` is `"1234567890123456800"`; `$string(1e21)` is `"1e+21"` | the exact digits: `"1234567890123456789"`; `"1000000000000000000000"` | value model: an integral value renders as an integer |
| `$round` on a decimal midpoint | rounds the decimal spelling: `$round(2.675, 2)` is `2.68` | rounds the binary float64: `2.67` | interpretation, pending: the documentation gives half-to-even, and its tie examples (`$round(11.5)` is `12`) are exactly representable, so they do not decide whether a tie is judged on the decimal spelling or the binary value; the float64 is identical on every host, so this is an algorithm choice, not a value-model consequence |
| Integers above 2^53 | binary64 throughout: `9007199254740993 = 9007199254740992` is true | exact: false; `$string` yields the digits | value model |
| An integral float64 beyond 2^53 in arithmetic | float arithmetic: `1e23 + 1` is `1e23` | the exact integer the float64 denotes: `99999999999999991611393` | value model: a value is an integer when it is integral, whatever its representation |
| `$power` with integer operands beyond float64 | `$power(2, 64)` is `18446744073709552000`; `$power(10, 400)` is an error (D3061) | exact `18446744073709551616`; exact within `MaxIntegerBits` | value model |
| Mixed integer and decimal arithmetic beyond 2^53 | rounds silently | error E1008 (`CodeInexact`) | engine; refuse rather than approximate |
| `$keys` and member order with integer-like keys | integer-like keys first, numerically: `["2","10","b","a"]` | insertion order for `*Object`, sorted byte order for maps (every key, not only integer-like ones) | value model: the documentation is silent; the reference's order is ECMAScript's own-property order |
| `$split(s, "")` and `$match.index` on astral characters | UTF-16 code units | code points | documentation, which says characters |
| String ordering (`<`, `$sort`, `^()`, `$min`/`$max` of strings) with characters beyond U+FFFF | UTF-16 code unit order: `"😀"` sorts before `"～"` | code point order: `"～"` sorts before `"😀"` | value model, consistent with characters as code points |
| Duplicate member names in decoded input | `JSON.parse` keeps the last | `Unmarshal` refuses (E1006) | engine; a binding decision this member's decoder makes rather than a language one |
| Ill-formed strings | `JSON.parse` accepts `"\ud800"` and JavaScript strings may hold unpaired surrogates | `Unmarshal` refuses the escape (E1006); a string that is not valid UTF-8 is refused when observed (E1001) | engine; refuse rather than coerce |
| The range operator's size cap | D2014 above 1e6 elements | the general budget: E1002 under `MaxWork` or `MaxOutputNodes` | engine |
| A function-valued result | returned to the caller as a function object | error E1001 | engine |
| Regular-expression dialect | JavaScript | pending ruling | value model: the documentation is silent |
