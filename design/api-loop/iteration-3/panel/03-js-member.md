# Iteration-3 panel: the JavaScript member's author

Cold read of `b501e30`. Rotating lens: the engineer who will write the JavaScript/TypeScript member from the same README.

## Grades

| Criterion | Grade | One-line justification |
| --- | --- | --- |
| Idiomatic fit for Go | B+ | `ctx`, `errors.Is`, `iter.Seq`, `Close`, `MustCompile` all read native; the `(value, present, err)` triple on four methods and the `map`/`*Object` two-arm switch are the price of a host with no `undefined` and no ordered map, and the doc is honest about it. |
| Ergonomics of the common path | B | Compile once, `Unmarshal`, `Eval`, `Marshal` is fine; but the common OpenBindings path returns `int64 | *big.Int | float64` and `map | *Object`, so every consumer writes type switches the reference's users never wrote. |
| Correctness and footgun risk | C+ | The numeric section claims "never of the representation" and then makes mixed arithmetic depend on whether a 1 was spelled `1` or `1.0`; the Env carries a base64 default the README says the evaluator must never choose; `Unmarshal` built on `encoding/json` would silently coerce `\ud800` to U+FFFD against rule 8. |
| Performance headroom the API permits | A- | By-reference admission, lazy per-value checks, compile-time regex, one budget per Evaluation, `Reads()` for fetch-only-what-you-need, and shared work across selections: nothing here forces a copy or a serialization. |
| Concept soundness | B- | Host-native values and documentation-as-authority are the right stance for OpenBindings; but the regex analogy predicts dialects (every regex engine is one), and the Go doc has already had to invent rulings (round basis, mixed rule, member order, regex dialect) that no "host" answers, then labeled them as Go's. |
| Overall | B- | A well-built first member whose doc reads as the class definition; a second member cannot be started from the README alone without either copying Go's inventions by hand or diverging in exactly the places OpenBindings authors touch. |

## Class-level versus Go-specific

Classification of each semantic commitment in `jsonata.go` from the seat of someone about to write `@openbindings/jsonata` (or whatever it ends up called) from the same README.

| Commitment in the Go doc | Class | What the JS member would do, and why |
| --- | --- | --- |
| Documentation at `DocumentationCommit` defines what an expression does; `Version = "2.1"` | class | Same constants, same commit. |
| Undefined versus null distinguished; `Eval` returns `(value, present, err)` | class (the distinction); host (the shape) | JS has `undefined` natively: `evaluate()` returns `undefined` for absent, `null` for null. No tuple. Cleaner than Go, and it is the reference's own convention. |
| Inputs are host values by reference; no serialization; admission per value on first read; `Prepare` checks nothing | class | Same. Admitted: `null`, `boolean`, `number`, `bigint`, `string`, `Uint8Array` (and subclasses such as `Buffer`), `Array` (including subclasses), plain objects (prototype `Object.prototype` or `null`, own enumerable string keys only, via `Object.hasOwn`). Everything else (`Map`, `Date`, `Set`, `Proxy` is undetectable, class instances, other TypedArrays, functions, symbols) is foreign. |
| Nil slice is `[]`, nil map is `{}`, nil pointer is null | host | No JS analog. The JS analogs the README does not mention: a property whose value is `undefined`, an array hole, and a `Map`. I would treat `undefined` property values and holes as absent (matching `JSON.stringify`, which drops them, and matching JSONata's own undefined), and `Map` as foreign. The README should say whether "absent inside an input" is even a concept a member may have; Go cannot have it. |
| Invalid UTF-8 string is `E1001` | class in spirit; host in form | JS strings can be ill-formed (lone surrogates). The analog is refusing a string for which `isWellFormed()` is false. Costs an O(n) scan on first read, as Go's `utf8.ValidString` does. The README must say ill-formed strings are refused, not "invalid UTF-8", or JS will not know to do this. |
| Foreign values readable only through `Resolver`; allowlist, not reflection; called once per read; charged as work | class | Same, with a synchronous `resolve(value)`. Note for rule 7: in JS, reading an own property of a plain object can execute host code (accessor properties). Checking descriptors on every read is too expensive; I would document that an accessor is the caller's Resolver-equivalent and the caller opened that door. Go has no such door. The README should state that rule 7 means "the evaluator provides no door," not "no door exists." |
| Carriage returns the same value, same type, same identity; constructed containers are new | class | Same. `===` identity for carried objects and arrays; never `structuredClone`. |
| Result that is a function is `E1001` | class | Same. |
| `*Error` with `Code`, `Class`, `Offset`(bytes), `Line`, `Column`(bytes), `Token`, `Value`; `errors.Is` sentinels; `ctx.Err()` for cancellation | class (codes, classes, message policy); host (position unit, matching idiom) | `class JsonataError extends Error` with `code`, `position`, `line`, `column`, `token`, `value`. Position in UTF-16 units would be the reference's convention; code points would match the "characters" stance. Byte offsets are meaningless in JS. The suite cannot compare positions until the class pins one unit; I would pin code points and let Go add byte `Offset` as a host extra. Cancellation: the JS idiom is `AbortSignal`, and throwing `signal.reason`. |
| Language codes S/T/D as the documentation defines | class | Same. |
| Engine codes `E1001`..`E1005` and their meanings | unclear | The Go doc says "engine code for refusals the language leaves to the implementation," which reads as member-owned. The OpenBindings SDKs need identical refusal classification on both sides, so I would use the same five codes. The README should own the E table so I am not copying Go's numbers by convention. |
| `Message` never includes input-derived content; `Value` carries the offending value; `Token` at most 64 chars | class | Same; this is a security property, not a Go property. |
| Integers exact at any size; results `int64`, `float64`, or `*big.Int`; overflow beyond `MaxIntegerBits` is `D1001` | host in wording, class in substance | JS: `number` for integers of magnitude at most 2^53 - 1, `bigint` beyond, normalized after every integer operation (a `bigint` result that is safe becomes a `number`; a `number` integer result that is not safe is recomputed in `bigint`). This gives one canonical representation per integer value, which is stronger than Go's `int64(1)` versus `float64(1.0)`. Rule 3 ("must not offer less than the host offers") forces this; the host offers BigInt. Every caller now receives `number | bigint`, and `JSON.stringify` throws on `bigint`; that is the JS price of the class. |
| A numeric literal `1` is `int64`, `1.0`, `1e2`, `0.5` are `float64`; `json.Number("1.0")` is `float64` | host, and it leaks | JS cannot distinguish `1` from `1.0`; they are the same `number`. This is the one place Go's representation is observable: see the next row. |
| Integer operand with `float64` operand must convert exactly or `D1001`; `9007199254740993 + 0.5` fails | unclear, and internally inconsistent | The doc says every operation "is a function of that value, never of the representation," yet `json.Number("1") + 9007199254740993` succeeds (int + int, exact) while `json.Number("1.0") + 9007199254740993` fails (float + int, inexact conversion), for identical numeric values. JS can only implement a value-based rule: an operand whose value is integral participates as an integer. Under that rule `1.0 + 9007199254740993` is exact in JS and an error in Go. No JS member can match Go here, because Go's rule is keyed on spelling. The README must pick: (a) fully by value (every integral-valued double is an integer; `1e300 + 1` is a 301-digit exact integer in both hosts), or (b) by value with a magnitude cap (integral and at most 2^53 counts as integer; larger doubles are decimals). Both are implementable identically in Go and JS. Go's current rule is implementable only in Go. |
| Integer `/` integer: exact when exact, else exact quotient rounded once to `float64` | class in substance | Same in JS, but there is no `big.Rat`; I write correctly rounded bigint/bigint to double by hand. Both members must state the same algorithm ("exact rational, rounded once, ties to even") or they will double-round differently. |
| `%` is Go's `%` for integers and `math.Mod` for floats | host, and the hosts agree | JS `%` on both `number` and `bigint` is truncated with the dividend's sign, same as Go. Agrees without effort. |
| Division or remainder by zero and non-finite results are `D1001`; non-finite input is `D1001` | class | Same. `NaN` and `Infinity` are reachable from JS callers (not from `JSON.parse`); refuse on first read. |
| `$sum`, `$abs`, `$floor`, `$ceil`, `$round` at precision 0, `$power` with non-negative integer exponent stay integer; `$sqrt`, `$average`, other `$power` yield float | class | Same. But `$power(2, 0.5)` and every non-integer `$power` go through the host's `pow`, and V8's and Go's `math.Pow` are not both correctly rounded across all inputs. The hosts do not agree here and the README does not say what to do when they do not. The suite needs a tolerance or the class needs to say "correctly rounded" (neither host provides it, so both would implement it). |
| `$sum` over floats | unclear | Sequential left-to-right binary64 addition agrees between hosts if both are naive; Kahan or pairwise would not. Unstated in both README and Go doc. Must be pinned. |
| `$round` half to even on the binary value; `$round(2.675, 2)` is `2.67` | class in substance, wrongly attributed to the value model | The ledger says "the value model (Go's float64) decides." JS has the same binary64, so JS must match Go, but `Math.round` is half-up and there is no host primitive for "half-even at decimal precision p on the exact binary value." I would implement it via exact decimal expansion with `bigint`. The algorithm must be pinned in the class ("exact rational value of the double, rounded half to even at p decimal digits, converted once to the nearest double"), or two members with the same float type will disagree on ties like `$round(0.125, 2)`. |
| `$string` of a float: "shortest decimal that round-trips," `-0` as `0`, per `JSON.stringify` | class, underspecified in the Go doc | JS gets this from the host for free: `Number::toString` is the documentation's algorithm. Go does not: `strconv.FormatFloat(f, 'g', -1, 64)` renders `1e6` as `"1e+06"` and `1e20` as `"1e+20"`, while `JSON.stringify` gives `"1000000"` and `"100000000000000000000"`, and switches to exponent form only at 1e21 and below 1e-6, spelling `"1e-7"` not `"1e-07"`. "Shortest round-trip" is a digit property, not a spelling. The Go doc must say "the ECMAScript Number::toString spelling." |
| `$string` of an object or array: `JSON.stringify` rules, no HTML escaping, prettify two spaces | class, underspecified | JS: the host algorithm, but with a custom walker because `JSON.stringify` throws on `bigint` (`JSON.rawJSON` in Node 21+ solves it) and honors `toJSON` (must not, on plain objects with a function-valued `toJSON`, which is foreign anyway). Go: `encoding/json` escapes U+2028 and U+2029 unconditionally and `JSON.stringify` does not; `JSON.stringify` escapes lone surrogates as `\udXXX`. The class needs an escape table. |
| Equality, ordering, membership by value across representations; never rounds through float64 when both exact | class | Same; in JS, `number` versus `bigint` comparison with `<` and `==` is already exact by value (`1n == 1` is true, `9007199254740993n > 9007199254740992` is true). Free. |
| Characters are code points: `$length("😀")` is 1, `$substring`, `$match.index` in code points | class | Must be implemented against the host's UTF-16 strings: `Array.from`, `codePointAt`, index translation for every regex match. O(n) per operation where the reference is O(1). Accepted. |
| String ordering by code point | unclear | The README lists "how values compare and order" under the value model, meaning the host's. JS `<` on strings orders by UTF-16 code unit, which differs from code point order for astral versus U+E000..U+FFFF. The host's answer differs from Go's. If the README means it, the members differ on `$sort` of strings containing emoji and private-use characters. I would follow Go and pin code points at class level. |
| Full Unicode case mapping: `$uppercase("straße")` is `"STRASSE"` | class in the Go doc; host per the README's regex analogy | JS gets it from the host (`toUpperCase` uses SpecialCasing). Go's `strings.ToUpper` yields `"STRAßE"`; the Go doc's promise needs `x/text/cases`. So Go exceeded its host to match Unicode, which is the right call, but the README's analogy explicitly hands case folding to the host. Pick one. |
| `$replace`: `$N`, `$$`, out-of-range group is empty; Go's `${name}` not recognized | class | Same rule; the JS member must additionally not recognize `` $` ``, `$'`, `$&`, `$<name>`, which `String.prototype.replace` would. The exact `$NN` disambiguation (two digits versus one digit plus literal) must be in the suite, not inferred from either host. |
| Regex literals compiled at `Compile` | class | Same; `new RegExp` throws at compile. |
| Regex dialect | unclear; the ledger says pending | The JS host's answer is `RegExp`. To honor "characters are code points" the JS member must compile with the `u` flag, otherwise `.` matches half a surrogate pair. `u` mode is stricter (`/\-/u` and `/{/u` are syntax errors), so the JS member would reject some patterns the reference accepts. Go's RE2 has no backreferences or lookaround and different `\s`, `\b`, and case-insensitive semantics. The class must define a portable subset and require each member to declare its dialect and its out-of-subset behavior. |
| `[]byte` is a string for `$type`, `$length`, string functions; encoded lazily through `BytesEncoding`; two byte values compare by bytes; bytes against string ordering is `T2010` | class (the README's "Relationship to OpenBindings" says so) | Same over `Uint8Array`. Equality of two `Uint8Array` by content, not identity. |
| `BytesEncoding` nil means `base64.StdEncoding` | contradicts the README | The README: a member "never chooses the encoding itself." A default is a choice. I would make the JS default "no encoding: using bytes as a string is `E1001`." If the class wants a default it must name it in host-neutral terms (RFC 4648 section 4, with padding), and then both members share it. |
| `map[string]any` observed in sorted byte order; `*Object` in insertion order; integer-like keys not hoisted; declared | host, and the hosts disagree | The ES specification orders own property keys with canonical array-index keys first, ascending numerically, then strings in insertion order, for every ordinary object regardless of prototype. That is the "JavaScript artifact" Go declines to reproduce, and it is the JS host's value model. A `Map` would give true insertion order but `JSON.parse` does not produce `Map`, `JSON.stringify` renders it as `{}`, and no TS caller wants it. So the JS member's `$keys({"2":1,"10":2,"b":3,"a":4})` is `["2","10","b","a"]` and Go's is `["2","10","b","a"]` only for `*Object` built in that order, `["10","2","a","b"]` for a map. Members differ; the README's "hosts differ, members differ" clause licenses it, and OpenBindings authors get different `$keys` and `$string` output. I would rather the class pin ES property order (the documentation is written against it and already pins `JSON.stringify`) and have Go's `Object.Set` hoist canonical array-index keys. Cheap in Go. |
| Two objects equal regardless of member order or representation | class | Same. |
| `Unmarshal`: numbers exact, `*Object` in document order, duplicate names are an error | unclear whether the decoder is in the class at all | The README says members evaluate the host's values with no serialization; a decoder is a convenience. If the class wants `parse()` to exist with these properties, JS pays: `JSON.parse` collapses duplicates before any reviver runs, so a from-scratch parser is required (5 to 20 times slower than V8's). Exactness alone is achievable with `JSON.parse` plus reviver source access (Node 21+). Lone-surrogate escapes: `JSON.parse` keeps them; `encoding/json` replaces them with U+FFFD, which is a silent coercion under rule 8 if Go's `Unmarshal` is built on it. |
| `Marshal`: `*Object` in order, map sorted, `json.Number` as its token, `*big.Int` as digits, bytes through encoding, no HTML escaping | host | `stringify()`: `bigint` as digits, `Uint8Array` through the encoding, no `toJSON`. `json.Number("1.0")` carried and re-emitted as `1.0` is a Go-only textual nicety; JS emits `1`. Same value. |
| `NewEncoder` over `io.Writer` | host | No analog; dropped. |
| `Materialize` | class | Same. |
| `Expression` immutable, concurrent-safe; input and Env borrowed; concurrent map mutation is fatal in Go; results may alias input and bindings | host | Single-threaded, so "borrowed" means "do not mutate during a synchronous call or while an `Evaluation` is open." Aliasing is the same. Re-entrancy (a Resolver calling back into the same `Evaluation`) is the JS hazard Go does not have; document it as undefined. |
| No goroutines, no process-wide state | class in spirit | No timers, no global caches, no `eval`/`Function`. |
| Selective evaluation: `Prepare`, `Select`, `Complete`, `Close`, `Plan`, `Reason`; unobservable (rule 9) | class (rule 9 and the `Reason` vocabulary if SDKs log it); host (shape) | Same shape with `[Symbol.dispose]` for `using`. Whether a given field is selectable may differ between members (planner qualification is member-level); the value never may. |
| Prelude evaluated once and shared; sibling `$error` is not a guard; `Select` is not a guard | class | Same semantics; this is about the language, not the host. |
| `$now`/`$millis` fixed at `Eval` or `Prepare`; `Env.Now` overrides; `$random` and `$eval` impure | class | `now?: () => number` in epoch milliseconds. Go has nanoseconds, `Date` has milliseconds, and the documented `$now()` format has three fractional digits, so the class should pin millisecond resolution. `Math.random` cannot be seeded, so no fixture can cover `$random` on either side. |
| `Reads()` static analysis with the listed opacity rules | unclear | The OpenBindings SDKs are required to be behaviorally identical, so if the Go SDK fetches only `Reads()` members, the TS SDK must too, which makes the soundness rules class-level. The Go doc presents them as this package's. |
| `Limits`: seven bounds, finite defaults, part of the compiled expression, `E1002` before the allocation | class (existence, `E1002`, which bounds); host (`MaxBytes` accounting) | Same fields. `MaxExpressionBytes`: bytes of what, UTF-8? In JS the natural unit is UTF-16 units; pin UTF-8 bytes. `MaxBytes`: JS cannot observe allocation, so it is an estimate (2 bytes per code unit, and so on). `MaxWork` and `MaxOutputNodes` are only comparable across members if "a node" and "a unit of work" are defined by the class; otherwise the same transform can exceed the budget in one member and not the other, which is observable. |
| Bounds count work, not time; cancellation from `ctx`, checked between operations and at intervals inside long ones | host | A synchronous JS evaluator cannot observe an `AbortSignal` that fires on a timer, because the timer never runs. Either a `deadline` (absolute ms) option checked at the same points, or an `evaluateAsync` that yields to the event loop every N work units and honors the signal. Go's `ctx` has no equal; the README should say cancellation is host-idiomatic. |
| Engine panic recovered into `E1005`; a panic inside Resolver or BytesEncoding propagates | host | JS: catch `RangeError` (stack overflow) from a defect and rethrow as `E1005`; errors thrown by a caller's resolver propagate. V8 heap exhaustion is fatal and uncatchable, so "no expression can terminate the process" holds in JS only as well as `MaxBytes` estimation holds. Go has the same caveat for its runtime OOM; neither doc says so. |
| Tail calls eliminated; do not count against `MaxRecursion`; default 1024 | class | JS engines do not eliminate tail calls; I trampoline. A naive recursive interpreter spends several JS frames per JSONata call level, and V8's default stack is roughly 10k small frames, so 1024 language-level recursion is near the edge. Either an explicit evaluation stack or a smaller effective bound; the class should say the default is a class number and the member may implement it however. |
| Timezone: UTC unless the documented offset argument or parsed text supplies one; local zone never consulted | class | Same. `Date` parsing of `"2017-05-15T10:00"` uses local time; the JS member must never call `new Date(string)` or `Date.parse`. |
| Bindings are values only; `$`-prefixed names are `E1003`; the map is borrowed | class | Same; rejects function values, which the reference accepts. |
| `$eval` available; compiles and evaluates within the same budget | class | Same. |
| `$type` by value across representations | class | Same; `typeof 1n` is `"bigint"` internally and `"number"` to the expression. |
| Sort stability | unclear | Not mentioned in either document. Go `sort.SliceStable` and ES `Array.prototype.sort` (stable since ES2019) agree if both choose stable. Pin it. |
| `$number(string)` grammar | unclear | Go's `strconv` and JS's `Number()` disagree on leading and trailing whitespace, `0x`, `Infinity`, empty string (JS gives 0). The documented grammar must be in the suite. |

## What the README must add for a second member to exist

Ranked by how much of my implementation I cannot start without it.

1. **A numeric profile between the documentation and the host, stated in value terms.** "Members whose hosts provide binary64 and exact integers evaluate under the class numeric profile: a number is an integer when its value is integral [and, optionally, of magnitude at most 2^53; otherwise it is a decimal]; integer with integer is exact; integer with decimal converts the integer to binary64 and fails with D1001 when that is inexact; integer division that is not exact yields the exact rational quotient rounded once to the nearest binary64; `$sum` and `$average` add left to right in binary64; `$round` rounds the exact rational value of the operand half to even at the requested decimal precision and converts once; `$string` of a decimal uses the ECMAScript Number::toString spelling; `$sqrt` is correctly rounded; `$power` with a non-integer exponent is the host's and members may differ in the last unit." Without this, Go's mixed rule is unimplementable in JS and the two members' `$round` and `$string` differ although their float types are the same.

2. **A ruling on object member order.** Either "member order is the ECMAScript ordinary-object property order (canonical array-index keys ascending, then insertion order), which the documentation is written against and `$string` already inherits through `JSON.stringify`," or "member order is host-owned; OpenBindings authors must not depend on `$keys` order." One sentence. Right now the Go ledger calls the ES order "a JavaScript artifact," and for me it is the host.

3. **A string profile.** "Characters are Unicode code points; an ill-formed string (invalid UTF-8, unpaired surrogate) is refused; strings order by code point; case mapping is full default Unicode case mapping without locale; no normalization is ever applied; error positions are in code points." Currently "how long a string is" and "how values compare and order" are handed to the host, and the Go doc then pins them anyway.

4. **A regular-expression stance.** "Each member uses its host's engine; the class defines the portable subset [grammar] and the `$replace` replacement mini-language [`$N`, `$NN` rule, `$$`]; a member declares its dialect, what it rejects at `Compile`, and its behavior for `.`, `\s`, `\b`, and case-insensitive matching outside the subset." Without it I do not know whether to compile with the `u` flag, and that changes which expressions compile.

5. **Bytes.** "Each member names its byte type (Go `[]byte`, JS `Uint8Array`); there is no default encoding; using bytes as a string without a supplied encoding is a refusal." Or name the default in RFC terms. The current Go default contradicts the README.

6. **A class-owned error table.** The E codes, their classes, the message policy, and the position unit. Otherwise the TS SDK and the Go SDK map refusals by convention, not by contract.

7. **Which `Limits` are class-level, with the same defaults, and what one unit of work and one output node are.** Otherwise budget refusals are observable divergences with no ledger entry possible.

8. **A statement that decoding and encoding are not part of the class,** and, if a member ships them, the three properties they must have (exact integers, duplicate-name policy, unpaired-surrogate policy).

9. **The selective-evaluation vocabulary (`Reason` names) and the `Reads()` opacity rules as class definitions,** because the OpenBindings SDK parity rule makes them observable.

10. **A correction to "The portable core."** `$string` of a decoded float such as `0.30000000000000004` is `"0.3"` in the reference and `"0.30000000000000004"` under the documentation; `$keys` and `$merge` observe member order. The listed subset is not "exactly the part where documentation and reference agree without exception."

## The suite and ledger two members need

**Fixtures.** Two corpora under `suite/`, both with stable ids and content hashes:

- The reference suite at its pinned commit, unchanged, in its own format (`expr`, `data`/`dataset`, `bindings`, `timestamp`, `result`/`undefinedResult`/`error.code`).
- A class corpus for what the reference does not test and where its expectations are its host's: integers beyond 2^53 in every arithmetic operator and every numeric function; `$string` spelling at 1e21, 1e-7, 1e6, -0, and 15-versus-17-digit values; `$round` at exact binary ties and at decimal midpoints; `$sum` order sensitivity; code points versus code units in `$length`, `$substring`, `$split`, `$match.index`, `$pad`; ill-formed strings; string ordering across the astral boundary; case mapping of ß, İ, ﬁ; member order with integer-like keys; `$string` escape table (U+2028, control characters, lone surrogates, `<`); bytes in every string function and in equality; `$replace` group references including `$10` with fewer than ten groups; regex portable subset; each `Limits` bound at the boundary; error codes for every refusal in the E table; timestamps with and without offsets.

**Expected-value notation.** JSON is not enough. The runner description must define: big integers as digit strings the runner parses exactly; `-0` distinguishable; bytes as a tagged form (`{"$bytes": "<hex>"}`); absent versus null; errors as code plus optional code-point position; object comparison order-insensitive by default with `orderSensitive: true` where order is the subject; numeric comparison exact by default with `ulps: N` for fixtures the class admits as host-dependent (`$power`); and a `profile` tag per fixture naming which class property it exercises (`numbers.mixed`, `strings.order`, `objects.integerKeyOrder`, `regex.dialect`, `budget.work`).

**The ledger.** Machine-readable, per member, with two axes, not one. Against the reference: `{fixture, fixtureHash, expected, authority: "documentation" | "value-model", reason}`, so a fixture change invalidates the entry (rule 6). Against siblings: a member does not enumerate every sibling disagreement; instead the class publishes `properties.json` listing the host-dependent properties from the profile tags, and each member's ledger declares its value for each (`objects.integerKeyOrder: "es-ordinary"` versus `"insertion"`, `regex.dialect: "ecmascript-u"` versus `"re2"`, `strings.order: "codepoint"`). The runner derives, per fixture, whether two members are expected to agree from their declared properties, and reports four buckets: agree with reference; agree with each other against the reference (a class divergence, must be in both ledgers with the same authority); disagree with each other on a declared property (a host difference, permitted); disagree with each other on no declared property (a defect in one member). The last bucket is the only thing that makes "the project's members align" checkable rather than aspirational.

**Runner description.** Language-neutral: how each fixture's data becomes host values (which is where `parse()` properties get tested), how bindings and `timestamp` are supplied (`now`), which limits to compile under, how results are normalized into the notation, and the rule that a member runs the suite through its public API only.

## The TypeScript mirror

```ts
export const VERSION = "2.1";
export const DOCUMENTATION_COMMIT = "5d1473277e0022d8580e00f891b12080eb3edd74";

export type Value =
  | null | boolean | number | bigint | string | Uint8Array
  | readonly Value[] | { readonly [key: string]: Value };
// `undefined` is never a Value; it is absence.

export interface Limits {
  maxExpressionBytes?: number; maxDepth?: number; maxRecursion?: number;
  maxOutputNodes?: number; maxWork?: number; maxBytes?: number; maxIntegerBits?: number;
}
export function defaultLimits(): Required<Limits>;

export interface Env {
  bindings?: Readonly<Record<string, unknown>>;
  resolver?: Resolver;
  bytesEncoding?: (bytes: Uint8Array) => string;   // absent: bytes-as-string is E1001
  now?: () => number;                               // epoch milliseconds
}
export interface EvalOptions { signal?: AbortSignal; deadline?: number }

export function compile(source: string, limits?: Limits): Expression;
export function evaluate(source: string, input: unknown, env?: Env, opts?: EvalOptions): unknown;
export function parse(text: string): Value;                       // exact integers, rejects duplicates
export function stringify(value: unknown, env?: Env): string;
export function materialize(value: unknown, resolver: Resolver, limits?: Limits): Value;

export class Expression {
  readonly source: string;
  readonly limits: Required<Limits>;
  reads(): { paths: string[][]; known: boolean };
  evaluate(input: unknown, env?: Env, opts?: EvalOptions): unknown;          // undefined = absent
  evaluateAsync(input: unknown, env?: Env, opts?: EvalOptions): Promise<unknown>; // yields; honors signal
  evaluateJSON(text: string, env?: Env, opts?: EvalOptions): string | undefined;
  prepare(input: unknown, env?: Env): Evaluation;
  toString(): string;
}

export class Evaluation {
  select(path: readonly string[], opts?: EvalOptions): unknown;
  complete(opts?: EvalOptions): unknown;
  readonly plan: Plan;
  close(): void;
  [Symbol.dispose](): void;
}
export interface Plan { readonly reason: Reason; readonly selective: boolean; fields(): Iterable<string> }
export type Reason = "none" | "dynamicShape" | "impure" | "unsupportedCall" | "arrayInput" | "planBudget";

export interface Resolver { resolve(value: unknown): unknown }   // synchronous
export interface ObjectView { get(key: string): { value: unknown } | undefined; entries(): Iterable<[string, unknown]> }
export interface ArrayView { readonly length: number; at(index: number): unknown }

export type Code = `S${string}` | `T${string}` | `D${string}` | `E${string}`;
export type ErrorClass = "unknown" | "syntax" | "type" | "evaluation" | "raised" | "engine";
export function classOf(code: Code): ErrorClass;
export const CODE_UNSUPPORTED_VALUE = "E1001", CODE_BUDGET = "E1002", CODE_BINDING = "E1003",
             CODE_CLOSED = "E1004", CODE_INTERNAL = "E1005";

export class JsonataError extends Error {
  readonly code: Code;
  readonly class: ErrorClass;
  readonly position: number;   // code points, or -1
  readonly line: number; readonly column: number;
  readonly token?: string;
  readonly value?: unknown;
}
```

Where the mirror breaks:

1. **`(value, present, err)` has no mirror and needs none.** `undefined` is the host's absent. The break is the other direction: `{a: undefined}` and array holes exist in JS inputs and have no Go analog; the class has to say what they are.
2. **`*Object` versus `map` has no mirror.** There is one object type, and its key order is the ES order with integer-like keys hoisted. I cannot give insertion order for those keys without `Map`, and `Map` is not what any caller holds or wants. Either the class pins ES order or the members diverge on `$keys`.
3. **`int64 | *big.Int | float64` mirrors as `number | bigint`,** with the safe-integer boundary playing `int64`'s role. But `float64(1.0)` versus `int64(1)` has no mirror at all; Go's mixed-arithmetic rule cannot be expressed.
4. **`ctx` has no synchronous mirror.** `AbortSignal` never fires inside a synchronous call. `deadline` is the honest option; `evaluateAsync` is the idiomatic one and doubles the API. The Go `Resolver(ctx, v)` becoming a synchronous `resolve(v)` forecloses async resolvers, which is the right call for a value-model door.
5. **`errors.Is` sentinels mirror as `instanceof JsonataError && e.code === …`.** Fine. Byte `Offset` and byte `Column` have no meaning; positions must be pinned in code points.
6. **`NewEncoder`/`io.Writer` and `MustCompile` have no mirror.** Dropped.
7. **`Now func() time.Time` mirrors as milliseconds only.** Nanoseconds do not exist; the class must not depend on sub-millisecond time.
8. **Regex: `RegExp` with `u` versus RE2.** Same notation, two dialects; some patterns compile in one member only, and `\s`, `\b`, `.`, and `i` semantics differ inside the shared grammar.
9. **`BytesEncoding` interface satisfied by `*base64.Encoding`** mirrors as a function; there is no host base64 object to satisfy it (Node `Buffer`, browser `btoa`, or the new `Uint8Array.prototype.toBase64`). The default must be "none."
10. **`MaxBytes` cannot be measured,** only estimated; `MaxRecursion` 1024 is near V8's native stack unless the interpreter is non-recursive.
11. **Panic recovery mirrors as catching `RangeError`,** but heap exhaustion is uncatchable in V8, so "no expression can terminate the process" is a `MaxBytes` promise in both hosts and neither doc says so.
12. **`parse()` cannot be `JSON.parse`** if duplicate names must be rejected; the reference implementation's users get V8's parser and mine get a slower one. If the class says decoders are not its business, I ship `parse()` as reviver-plus-source on Node 21+ and document that duplicates take the last value, as the host does.
13. **Concurrency prose has no mirror,** but re-entrancy does, and it needs a sentence.

## What I would change in the Go doc

1. **Open with the class and the profile, not with Go's types.** Replace "This package's value model, built from Go's own types, defines what values are and how they combine" with: "This package is the Go member of the OpenBindings evaluator class. It evaluates under the class numeric profile (binary64 decimals and exact integers) and string profile (code points, full Unicode case mapping), using int64, *big.Int, float64, and string as their representations. Where this document says 'this package' it means this member; where it says 'the class' a JavaScript member is bound to the same behavior."

2. **Replace the mixed-arithmetic rule with a value-based one and delete the contradiction.** Replace "An integer operand combined with a float64 operand must convert to float64 exactly, and is an error (D1001) otherwise" with: "An operand whose value is integral participates in arithmetic as an integer whatever its representation, so json.Number("1.0") + 9007199254740993 is exact. An operand whose value is not integral is a decimal; an integer combined with a decimal converts to float64 and is an error (D1001) when that conversion is inexact, so 9007199254740993 + 0.5 fails while 0.1 + 0.2 yields the float64 sum." (Or the capped variant, if the class chooses it.) Then "never of the representation" becomes true, and a JS member can implement it verbatim.

3. **Pin `$string` and `$round` to algorithms, not to adjectives.** Replace "a float64 as the shortest decimal that round-trips" with: "a float64 in the ECMAScript Number::toString spelling that the documentation specifies through JSON.stringify: shortest round-trip digits, plain notation from 1e-6 up to but excluding 1e21, exponent notation with a sign and no zero padding otherwise, so $string(1e6) is "1000000" and $string(1e-7) is "1e-7"; strconv's 'g' format is not this." Replace "$round rounds half to even on the binary float64 value" with: "$round rounds the exact rational value of the float64 half to even at the requested decimal precision and converts once to float64; the class pins this algorithm because every member with binary64 must agree on ties."

4. **Reframe member order as the class's choice or as a declared host property, not as a Go fact.** Either: "The class fixes member order as the ECMAScript ordinary-object order the documentation is written against: canonical array-index keys ascending, then insertion order. *Object implements it; a map[string]any, which has no order, is observed with array-index keys ascending and the rest in sorted byte order." Or, if the class declines: "The class leaves member order to the host (declared property objects.memberOrder); this member's value is insertion order for *Object and sorted byte order for maps, and the JavaScript member's value is the ECMAScript order. Transforms that depend on $keys order are not portable between members."

5. **Remove the bytes default and re-scope the E codes and positions.** Replace "Nil means base64.StdEncoding" with "Nil means none: an expression that uses a []byte as a string fails (CodeUnsupportedValue), because the class requires that the binding, not the evaluator, choose the encoding." Replace "or an engine code for refusals the language leaves to the implementation" with "or a class-defined engine code (E1001 through E1005, shared by every member)." Add to `Error`: "Position is the offset of the offending token in code points, the unit the class suite uses; Offset, Line, and Column are this member's byte-based positions in the go/token convention."

## On the concept

From the second implementer's seat, the class as written produces two dialects, and it does so in exactly the four places an OpenBindings author is most likely to stand: `$string` of a number, `$keys` of an object with numeric-string keys, arithmetic near 2^53, and any regex beyond `[a-z]+`. Not because host-native values are wrong (they are right, and the by-reference, no-serialization stance is the only one that makes a `TransformEvaluator` seam cheap enough to sit on every response), but because the README delegates to "the host" a set of questions that no host actually answers. Go has no opinion on whether `$round(0.125, 2)` is `0.12` or `0.13`, on whether `"2"` sorts before `"a"` in an object, on whether `1.0` is an integer, or on which regex dialect a JSON query language should carry. The Go member answered them, competently, and then wrote the answers in Go's vocabulary, and the ledger attributes them to "the value model." The JS host answers some of those questions differently (property order, regex, string ordering) and cannot express one of them at all (the `1` versus `1.0` distinction). The regex analogy the README leans on is honest about this: PCRE, RE2, and ECMAScript RegExp share a notation and are three dialects, and everyone who writes a portable pattern knows to stay in the intersection. That is a fine model for a language with a formal grammar and no shared data. It is a poor model for a transform language whose whole job is to produce the same portable value on both sides of an SDK boundary.

Two agreeing members are achievable, but only if the class grows a layer the README currently denies it needs: a profile between the documentation and the host, stated in value terms, that every member with binary64 and exact integers must implement identically. Most of it costs the JS member nothing, because the documentation is written against JavaScript and the profile would largely be "do what ECMAScript does" (`Number::toString`, property order, `toUpperCase`), which means the Go member is the one porting algorithms, which is exactly what the README says nobody does. The remaining host differences (regex dialect, `pow` in the last unit, the memory bound) are genuinely irreducible and should be declared as named properties in both ledgers so the runner can tell a permitted difference from a defect. What I can promise OpenBindings authors, once that profile exists: identical values for the portable core including `$string` and `$keys`; identical absent-versus-null; identical language and engine codes; identical exactness for every integer they will ever see in an API payload; and identical budget refusals if work is defined by the class. What I cannot promise, and would not let the README imply: identical regex behavior outside a published subset, identical last-ulp results from `$power`, or that a transform written and tested against jsonata-js gives the same `$string` output here, since the class has already chosen the documentation over the reference on that point and both members will diverge from it together. Without the profile, I can promise only that my member will be a faithful ECMAScript-flavored dialect of the documentation, which is a second thing, not a second member.
