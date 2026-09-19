# Iteration 2 cold read: pl and spec skeptic

> Given only the class README and go/jsonata at 4cb2f5d; told not to read design/. Verbatim.

## Grades

| Criterion | Grade | One-line justification |
| --- | --- | --- |
| Idiomatic fit for Go | B+ | `(value, present, err)`, `iter.Seq`, `MustCompile`, sentinels with `Is`: all recognizably Go; `Close` on a non-resource and `Limits` fused into `Compile` are the only stretches. |
| Ergonomics of the common path | B | `Eval(ctx, src, input, nil)` is one line; the tax is that every consumer of "an object" writes a two-arm switch, and every consumer of "a number" a three-arm one. |
| Correctness and footgun risk | C+ | The mixed int/float refusal rule as written refuses `0.1 + 0.2`; `[]byte` ordering depends on `Env`; `$keys` order depends on how the caller decoded; `Select` is not a guard; results may contain views. |
| Performance headroom the API permits | A- | By-reference admission, lazy views, `Reads()`, field-granular selection with shared work, budgets charged before allocation. This is the strongest part of the design. |
| Concept soundness | C | Honest and better than porting, but the authority model contradicts itself (rule 6 versus the thesis, rule 3 versus rule 6, rule 9 versus `Select`), "value model" absorbs whatever the documentation is vague about, and nested authorities (regex, JSON.stringify, XPath pictures) are not addressed. |
| Overall | B- | A good evaluator design carrying a specification theory that does not hold its own weight. |

## Attacking the concept

### 1. Documentation as authority

**Steelman.** docs.jsonata.org is a tutorial with a function reference. It has no grammar beyond an informal one, no evaluation semantics, and no definition of the three things that make JSONata JSONata: sequence flattening, singleton unwrapping, and undefined propagation. Those are defined by `evaluateStep` and `createSequence` in jsonata-js and nowhere else. Choosing "the documentation" as authority for a language whose documentation does not contain its semantics is choosing a text that cannot answer the questions that matter, and then answering them yourself.

The README's fallback is two-level: documentation, then value model. But most silences are not value questions. Regex dialect. `$keys` order. Signature-string coercion and context-argument substitution. `$sort` stability. Whether `{"a": 1}` over `[]` is `{}` or `{"a": 1}`. The parent operator. Tail calls. For all of these the README has no rule, and membership rule 1 quietly supplies one: "verified against the reference test suite." The suite is the reference implementation's behavior, snapshotted. So the actual authority order is documentation, then the reference (via fixtures), then the host, and the README states the opposite ordering in its headline.

Worse, rule 6 forbids what the thesis requires. The thesis says the documentation wins over the reference. Rule 6 says a departure from the reference suite is permitted *only* where the value model cannot represent a reference value. `$string(0.1 + 0.2)`: the documentation says JSON.stringify (which yields `0.30000000000000004`); I am fairly confident the reference applies `toPrecision(15)` and yields `"0.3"`. The package doc follows the documentation. float64 can represent `0.3` fine, so rule 6 forbids this divergence. Either the thesis or rule 6 is wrong, and it is rule 6.

And incorporation by reference: the documentation defines `$string` by JSON.stringify (ECMA-262), regex literals by JavaScript regex (ECMA-262 §22.2), `$formatNumber` and `$formatInteger` by XPath F&O pictures, `$fromMillis` by XSLT date pictures. "Documentation as authority" says nothing about whether the incorporated authority is owed in full, in part, or at all. Go's `regexp` cannot implement ECMAScript regex (no lookaround, no backreferences, code points not UTF-16 units). The package doc's entire treatment of this is "a regex literal that does not compile fails at Compile." That is a *notation* divergence, not a value one, and rule 6 does not permit it.

**Lands.** Fully. The stance is coherent only if you add a third authority (the suite) and admit it, and add a rule for nested authorities.

### 2. Host-native value model

**Steelman.** The transform's job is to move values between systems. The value model is where payload meaning lives. If the host defines what a number is, `$.id + 1` has different results in the Go member (exact int64) and any plausible TS member (binary64, silently 9007199254740992). The README embraces this: "where the models differ, the members differ, and that is correct." But an OpenBindings interface document is a *contract*, and the CLAUDE.md orientation says the SDKs must produce "the same portable values" at the boundary. A transform whose result depends on which SDK ran it is not part of a contract; it is a dialect.

The counter is that JSON text already loses `9007199254740993` in `JSON.parse` before any transform runs, so the divergence is the binding's, not the evaluator's. True, and it makes host-native *honest*. But honesty about an implementation is not a language semantics. Look at where "the value model" is actually invoked in the package doc:

- `$uppercase("ß")` is `"ß"` because Go's `strings.ToUpper` uses simple case mapping. Go can produce `"SS"` (`x/text/cases`). This is not "what the host offers"; it is what the host's cheapest function offers.
- `$round(2.675, 2)` is `2.67` because "the value is the binary float64." I am fairly confident the reference rounds on the shortest decimal representation and yields `2.68`. The shortest round-trip decimal is *also* a pure function of the float64 value. Choosing the binary midpoint is a choice, not a consequence of the host.
- `$keys` on a `map[string]any` is sorted, because Go maps are unordered. The reference is insertion order (with ECMAScript's integer-like-key hoisting, which I would not implement either). Fine, but the same JSON document decoded by the caller into a map versus an `*Object` now yields different `$keys`, `$string`, `$each`, and `$merge` results from the *same member*.

"The host defines what a number is" is principled exactly once in this doc: the int64/big.Int/float64 model, which is a real, richer model with real rules. Everywhere else the phrase is doing the work of "we picked the convenient thing." And the README does not verify its own premise: I would want to see the pinned docs' data-types page before accepting "the documentation never says what a number is." If it says numbers are IEEE 754 doubles, the whole integer model is a declared divergence from the documentation, and the README's own priority order forbids it.

**Lands.** As a language stance, yes: values are half of JSONata's semantics and ceding them makes portability an accident. As an implementation stance it is defensible and better than pretending Go has JS numbers. The fix is a language-level floor above the members, not abandoning native values.

### 3. The regex analogy

**Steelman.** Regex engines share a notation and diverge per host; users cope; Go's `regexp` is the celebrated case of not porting PCRE. So JSONata engines can do the same.

Attack: (a) The regex core is *formal* (regular languages) and the practical dialects have *normative* specs (POSIX, ECMA-262, RE2's syntax page). "The notation is shared" has a referent there. JSONata has none. (b) Go's RE2 choice is principled by a theorem: linear-time matching in exchange for dropping non-regular features. It is a semantic boundary with a theory behind it. There is no analogous theory behind "int64 then big.Int." (c) The decisive asymmetry: a regex in a Go program is written *for* Go's `regexp` by the same author who runs it. A transform in an interface document is written for "JSONata" and run by whichever SDK the consumer happens to have. The regex analog of that is shipping a pattern in a config file consumed by both JS and Go, which is precisely the situation regex users get burned by and the reason they retreat to a portable subset. The README's "portable core" section is that retreat, presented as an observation. (d) In regex the value model is trivially the string; the entire interesting semantics belongs to the notation. In JSONata, arithmetic, comparison, ordering, and stringification are a large fraction of what transforms do. (e) The analogy is *self-defeating* on regex itself: JSONata embeds a regex notation, and the Go member's regex dialect is RE2, not ECMAScript. So the analogy predicts the problem, not the solution.

**Lands.** The analogy is load-bearing for the organizational claim (write natively, do not port) and decorative for the semantic one (host-defined primitives are fine). Keep it for the first; stop leaning on it for the second.

### 4. Declared divergence as sufficient discipline

**Steelman.** Every serious implementation of an under-specified notation keeps a divergence list. Pinning to fixtures so a suite bump invalidates them is a good forcing function.

Attack: (a) It is fixture-shaped, so it can only see what the suite exercises. A divergence from the language that no fixture hits cannot even be *recorded* in the sanctioned format. Rule 6 makes the divergence ledger a diff against a sample, not a statement about the language. (b) Declaring is not ruling. For most entries the documentation is silent, so each entry will read "we do X, reference does Y, documentation silent" with no resolution. A ledger of unresolved diffs is not authority. (c) It is member-facing, not author-facing. A transform author needs the *intersection* of every member's ledger. Nobody publishes that; the "portable core" is explicitly "not a rule." (d) Rule 6's permission criterion is inverted for the cases the Go member actually has. Every divergence I found above is one where Go is *richer* than the reference (exact integers, full Unicode strings, RE2's Unicode model) or where the choice is free (round, uppercase, key order). Rule 6 only permits divergence where Go is *poorer*. Rule 3 forbids being poorer. Read together, rules 3 and 6 forbid nearly every divergence the package doc declares. (e) Tail calls: the reference has TCO and (verify at the pinned commit) I believe the suite has a deep tail-recursion fixture. `MaxRecursion` 1024 without TCO fails it. Budget refusal is allowed by rule 8, but a fixture failure is a rule-6 divergence, and not a permitted one.

**Lands.** Declaration is necessary and not sufficient. Sufficient is: a corrected permission criterion, a required ruling per entry, and fixtures for the *declared* behavior so the divergence is tested rather than merely recorded.

### 5. Choosing JSONata at all

**Steelman.** For document-supplied transforms that must run identically on unknown hosts with the service unaware, you want a total, decidable, specifiable, portable language. JSONata is none of these: no spec, single reference whose author is the spec, JS value model, recursion and `$eval` (hence budgets), and it incorporates three foreign specifications. Every paragraph of philosophy in the README exists because of that vacuum. CEL has a formal spec, conformance tests, multiple implementations, and a *specified* per-type numeric model with declared overflow errors, which is exactly what this package is inventing by hand. JMESPath has a spec, grammar, and compliance suite. Both are less expressive, and JMESPath has no arithmetic.

Counter: JSONata's authoring ergonomics for JSON shaping are genuinely better than either, the transforms observed so far use a small subset, and switching costs are high.

**Lands partially.** The right response is not to switch but to stop pretending JSONata as a whole is the language. The language OpenBindings actually needs is the portable core, specified, with the rest of JSONata available at "documented behavior, may diverge" fidelity. That is what the README already describes, in the wrong register (observation instead of rule).

## What a PL person would want instead

Ranked by value per effort:

1. **A normative portable profile, owned by OpenBindings Core or `suite/`, above the members.** The listed portable core becomes a rule with its own fixtures derived from the documentation, independent of the reference suite. Members must pass it *identically*. Low-to-moderate effort; the list exists. This is the single thing that makes "transforms in interface documents" a contract.
2. **A language-level numeric floor.** Something like: "numbers denote values of the JSON number grammar; integer results in [-2^53, 2^53] are exact in every member; outside that range behavior is member-declared; mixed integer/float combination is refused when the integer is not exactly representable as binary64." One paragraph. Removes the largest cross-member hazard.
3. **Fix rule 6's criterion and require a ruling per divergence.** Wording below.
4. **An incorporated-authorities table.** For regex, `$string`/JSON.stringify, `$formatNumber`, `$formatInteger`, `$fromMillis`/`$toMillis`: which external spec, which subset the class owes, which subset is portable. The regex row defines a portable regex subset (RE2 ∩ ECMAScript, over code points). Low effort, high value.
5. **An operational semantics note for the evaluation core** (sequence, singleton, `[]`, undefined propagation, grouping over an array input) with fixtures. This is what the documentation does not contain and the suite only implies. Medium-high effort; put it in `suite/` as the "language-neutral runner description" it already promises.
6. **A differential fuzz harness** against jsonata-js at the pinned commit over generated inputs on the portable core, to *find* undeclared divergences rather than wait for fixtures. Medium effort.
7. **A formal grammar** (PEG) for the syntax. Medium effort, medium value; parsers agree more than evaluators do.

## Semantic commitments in the API

**Absence: `(value, present, err)`.** Sound and the right shape. Complete. Consistent across `Eval`, `Select`, `Complete`, `EvalJSON`. Leaves undefined: nothing material.

**Nil slice is empty array, nil map is empty object, nil pointer is null.** Sound, and the stated reason (predicates over an unset collection) is correct, since the reference returns `[]` for a path landing on an empty array rather than undefined. Consistent with carriage (a nil map returned by `$` is the same nil map).

**Map key order is sorted; `*Object` is insertion order; constructed objects are `*Object`.** Internally consistent. Not consistent with the portable-core claim: `$keys`, `$merge`, `$sift`, `$string` are in the core, and their output order now depends on whether the caller decoded into a map or an `*Object`. Leaves undefined: whether `$merge([x])` and `$sift` with an all-true predicate construct or carry.

**Numbers.** The three-representation model with value-based comparison is sound and the best part of the doc. Three defects:
- "the operation is an error (D1001) when its exact result is not representable as float64" refuses `0.1 + 0.2` (the exact sum of two binary64 values is not a binary64) and `1/3`. The README says `0.1 + 0.2` is host addition. The intended rule is presumably "an integer operand that is not exactly representable as float64 is refused when mixed with float64." Say that.
- `9007199254740993 / 2` is claimed to succeed. The exact quotient is 2^52 + 0.5, which is not a float64 (ULP at 2^52 is 1). So it either silently rounds, contradicting rule 8 and the treatment of `9007199254740993 + 0.5`, or it must refuse. Pick one rule for entering the float domain and apply it to division too.
- `-0 rendered as 0, which is what encoding/json produces`: I am fairly confident `encoding/json` renders negative-zero float64 as `-0`. The behavior is right (JSON.stringify gives `0`); the stated host reason is wrong.
Leaves undefined: which stdlib functions participate in the integer domain (`$power(2, 100)` exact big.Int or float?), what `$sqrt` does with an inexact int64, what `$number("1e400")` does, whether `MaxRecursion` counts tail calls, and whether a malformed `json.Number` is really a language cast error (D3030, `ClassEvaluation`) rather than an admission error (`E1001`, `ClassEngine`). That last one breaks the taxonomy: a caller cannot distinguish "my input was bad" from "the expression cast badly."

**Strings are valid UTF-8 code point sequences; invalid UTF-8 refused.** Sound as a model, but note it is the *package's* model, not the host's: a Go string is bytes. Leaves undefined: when the refusal fires. If on admission, `Eval` scans the entire input including parts never read, and `Prepare`'s "admitted lazily" contradicts it. If on use, does a pure copy count as use? Rule 4 says a copied value arrives unchanged; refusing a copied invalid string contradicts it.

**`[]byte` is a string whose content is its encoding; compared and ordered by that encoding.** Equality is sound within one `Env`. Ordering is not: base64's alphabet is not ASCII-monotone, so `$sort` over bytes is a permutation of byte order, and switching `Env.BytesEncoding` to the URL alphabet changes the order of the same values. That violates rule 5's "by value across every representation" in spirit. Leaves undefined: what `$base64decode` of a `[]byte` returns when the decoded content is not UTF-8.

**Object equality is deep, order-insensitive, map-vs-`*Object` insensitive.** Sound, matches the reference's deep equality. Consistent with the ordering commitment (`$string(a) = $string(b)` may be false while `a = b` is true; JS has the same).

**Selective evaluation: sibling failure does not propagate under `Select`.** The doc is honest about it, and "when Complete would succeed, Select returns the same field" is the correct one-directional guarantee. But README rule 9 says a selected field is identical to the same field of a complete evaluation, unconditionally. `{ "a": $error("x"), "b": 1 }`: `Select("b")` is 1, `Complete` fails. The API violates the class rule. Leaves undefined: whether a lambda defined in the prelude and called in a field is planned through (or is `ReasonUnsupportedCall`), whether `Access` is assumed pure for planning, and what `Select("items", "id")` does when the deepest selected field is an array (map over elements, as `Reads` does, or absent).

**`ReasonArrayInput`: "the constructor maps over it and the result is an array."** I am fairly confident this is wrong about JSONata. The object constructor over an array input is a group-by: with literal keys the result is one object whose values are evaluated over the grouped sequence (and singleton-unwrapped), and over `[]` the result is `{}`. Falling back to `ModeComplete` is the right call; the stated reason is not the language's behavior.

**`Reads()`: "sound when known."** Good commitment, correctly conservative. "a binding makes known false" is more conservative than necessary but sound. Leaves undefined: `$` inside predicates, context-passing calls like `$.a.$uppercase()`.

**`Access`: "consulted once per foreign value," "called concurrently."** "Once per value" requires memoization by identity, and a foreign value that is a non-pointer struct or a slice has no usable identity (map key on a non-comparable type panics). Commit to "once per read" or "once per pointer identity." Also: "A value that Access resolved is returned as the resolved value" means a result can contain an `ObjectView` implementation, so the caller's "two-arm switch" is really map, `*Object`, view, and untouched foreign. Consider returning the originating foreign value rather than its view.

**Timestamp fixed per `Eval`/`Prepare`; `$random`, `$eval` impure; closed environment; UTC only.** Sound, consistent with the reference's per-evaluate timestamp, and better than the reference on zones.

**Error `Code` with `Class()` "derived from its form."** `ClassRaised` covers `$error` and `$assert`, which are D codes in the language, so `Class` cannot be purely form-derived. Minor wording. Sentinels via `Is` on `Code`: sound.

**Limits fused into `Compile`.** Semantically fine (limits change refusal, not meaning). Note that runtime bounds (`MaxWork`, `MaxOutputNodes`) being compile-time means one shared `*Expression` cannot be evaluated under a per-call budget; the cache-key rationale is real but the coupling is a choice.

## What I would change

1. **Rewrite membership rule 6 and add a rule for nested authorities.**
   > 6. Divergences are declared, never silent. A departure from the reference suite is permitted only where the reference behavior is an artifact of its host (its number, string, regex, or key-order model) and not of the documentation. Each declaration names the authority it follows instead (the documentation, or the member's stated value model), records the reference behavior, and is pinned to the fixture it departs from. Where the documentation is silent and the question is not one of values, the reference suite is the authority.
   > 6a. Where the documentation incorporates another specification by reference (regex syntax, JSON.stringify, XPath pictures), the member states which subset of that specification it implements.

2. **State one rule for entering the float domain, and fix the two wrong examples.** Package doc, "Numbers":
   > Any float64 operand yields float64. An integer operand that is not exactly representable as float64 is refused (D1001) wherever it would be converted, including inexact integer division, so 9007199254740993 / 2 and 9007199254740993 + 0.5 both fail, while 9007199254740992 / 2 and 0.1 + 0.2 yield the host's float64 result.
   And list which standard-library functions participate in the exact-integer domain (`$sum`, `$abs`, `$floor`, `$ceil`, `$round` at precision 0, `$power` with a non-negative integer exponent, `$formatBase`).

3. **Reconcile rule 9 with `Select`.** Either:
   > 9. Selective evaluation, if offered, is unobservable when the complete evaluation succeeds: the field obtained selectively is identical to the same field of the complete result.
   or make `Select` return the sibling's error. I would take the wording change and add to `Select`'s doc: "Select is not a guard; a transform whose failure must be observed uses Complete."

4. **Make `[]byte` ordering `Env`-independent.**
   > A []byte equals another []byte with the same content, and equals a string equal to its encoded form. Ordering between []byte values is by content; ordering a []byte against a string is an error (T2010), as ordering a number against a string is.
   This keeps the sort comparator total within a kind and refuses the cross-kind case JSONata already refuses for numbers.

5. **Name the regex dialect and put a portable regex subset in the core.** Package doc, "Strings and bytes":
   > Regex literals are Go RE2 syntax over code points. Lookaround, backreferences, and other ECMAScript features RE2 lacks fail at Compile; `\s` and case-insensitive matching follow RE2.
   README portable core: add "regex literals limited to the RE2/ECMAScript intersection." And drop `$string` on non-integers from the "agree without exception" claim until the `toPrecision(15)` question is verified and declared.

Also, cheaply: `Access` "once per read"; results never contain views; `D3030` for malformed `json.Number` becomes `E1001`; `ReasonArrayInput` reworded to "the constructor groups over the input array"; clarify when invalid UTF-8 is refused; state whether `MaxRecursion` counts tail calls.

## On the concept

The *implementation* stance is right and the package doc is, for the most part, a careful piece of work. Native values by reference, refuse rather than approximate, budgets charged before allocation, lazy views for foreign types, static `Reads`, field-granular selection with a stated one-directional guarantee, a closed environment with a named boundary: nobody porting jsonata-js gets any of that, and each of those choices is a real semantic improvement over the reference. If the question were "is this a sound way to *build* a JSONata evaluator in Go," the answer is yes, with the defects listed above fixed.

The *authority* stance is the mistake, and it is a well-argued one. "Documentation as authority" for a language whose documentation is a tutorial makes the reference suite the real authority while the README says the reverse, and its own rule 6 then forbids most of the divergences the package actually declares. "The host defines values" is principled exactly where the package has done the work (the integer model) and is used as a shield everywhere else (round, uppercase, key order), which is where an author writing a transform for an unknown host gets hurt. And the regex analogy, taken seriously, describes the failure mode of shipping notation across hosts rather than justifying it.

What is missing is a layer, not a reversal: a normative profile owned above the members, containing the portable core as a rule, a numeric floor, a key-order rule, an incorporated-authorities table with a portable regex subset, and fixtures for all of it that do not come from the reference. With that layer, "members differ where their value models differ" becomes a true and harmless statement about the *rest* of the language, the divergence ledger becomes a list of rulings instead of a diff, and OpenBindings transforms become a contract at the one boundary where the README currently, and explicitly, declines to make them one.
