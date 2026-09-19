# Declared divergences: Go member

Where this package's behavior departs from the reference implementation's
test suite at the pinned commit, and why. Each entry names the authority
followed. Entries will be pinned to the fixtures they depart from once the
shared suite lands in `suite/`.

| Behavior | Reference | This package | Authority followed |
| --- | --- | --- | --- |
| `$string` of a float, and `&` with a float | 15 significant digits: `$string(0.1 + 0.2)` is `"0.3"`, `$string(1234567890123456789)` is `"1234567890123456800"` | shortest decimal that round-trips: `"0.30000000000000004"`; integers exact | the documentation, which specifies `JSON.stringify` |
| `$round` on a decimal midpoint | rounds the decimal spelling: `$round(2.675, 2)` is `2.68` | rounds the binary float64: `2.67` | the documentation is silent on the basis; the value model (Go's float64) decides |
| Integers above 2^53 | binary64 throughout: `9007199254740993 = 9007199254740992` is true | exact: false; `$string` yields the digits | the value model; the reference's behavior is its host's |
| Mixed integer and float arithmetic beyond 2^53 | rounds silently | error D1001 | the value model, refuse rather than approximate |
| `$keys` and member order with integer-like keys | integer-like keys first, numerically: `["2","10","b","a"]` | insertion order for `*Object`, sorted for maps | the documentation is silent; the reference's order is a JavaScript artifact |
| `$split(s, "")` and `$match.index` on astral characters | UTF-16 code units | code points | the documentation says characters |
| `$uppercase("straße")` | `"STRASSE"` | `"STRASSE"` | no divergence; recorded because an earlier draft chose simple mapping |
| Regular-expression dialect | JavaScript | pending ruling | the documentation is silent |
