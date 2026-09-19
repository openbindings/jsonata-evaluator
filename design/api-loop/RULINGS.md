# Ruling queue

Decisions the loop may not make. Each carries the strongest argument on each
side and a recommendation. The stub keeps its current answer until a ruling
lands. A ruling still open when the loop stops becomes a GitHub issue on this
repository.

## 1. Output object type

**Current:** carried objects return as the caller's `map[string]any`;
constructed objects return as `*Object`. The switch is now documented.

**For uniform `map[string]any` with opt-in order (idiom):** the documentation
does not make key order normative (the reference *behaves* as JS objects do);
by the README's own rule, order is the host's, and Go's object value is an
unordered map. A heterogeneous result type is the one outcome wrong under
either reading; `encoding/json` never does this to a caller.

**For uniform `*Object` (pl, practitioner):** `$keys`, `$each`, grouping, and
the transform operator all observe order in the reference and every user
relies on it; sorted map order is a declared divergence either way; one type
means one code path.

**For the documented split (current):** exact carriage by identity is a
stated commitment and the cheapest possible path for the common rename;
converting every input map to `*Object` costs an allocation per object on the
way in and breaks identity.

**Recommendation:** keep the split. Carriage by identity is worth more than
type uniformity, and encoding handles both.

## 2. `Select` path: `...string` or compile-resolved `Field`

**For `Field` handles (perf):** `Select(ctx, path ...string)` with memoization
escapes the path slice, builds a key, inserts into a locked map: three to
four allocations and a lock before any evaluation, exceeding the cost of
evaluating a rename-only field. `Field(path...) (Field, bool)` resolved at
compile removes all of it and tells the caller at compile time whether the
field is selectable.

**For `...string` (current):** simplest possible call site; no handle type to
hold; the allocation argument assumes a locked memo, which ruling 3 decides.

**Recommendation:** `Field` handles. The performance case is concrete and the
compile-time selectability check is a real feature.

## 3. `Evaluation` concurrency

**For concurrency-safe (current):** an Evaluation handed to several
goroutines selecting different fields shares work automatically.

**For single-goroutine by rule (perf, integrator):** the 99% case is one
goroutine selecting a few fields; a mutex on every Select is paid by all for
the benefit of few; `Expression` being immutable is the concurrency contract
that matters, and `bufio.Reader` is the precedent.

**Recommendation:** single-goroutine by rule, with the sentence "create one
Evaluation per goroutine."

## 4. Regex dialect

**For Go's `regexp` as a declared divergence (perf):** linear-time by
construction, so a hostile pattern cannot make evaluation slow; standard
library; no dependency. The documentation names no dialect.

**For the reference's dialect via a backtracking engine (practitioner):**
lookahead, lookbehind, and backreferences appear in real `$match`/`$replace`
expressions against messy vendor strings; every expression written against
the JavaScript reference or Step Functions will assume them; refusing them at
Compile means those expressions do not port.

**Interaction with ruling 6:** if the reference decides operations where the
documentation is silent, the dialect is JavaScript's unless declared
otherwise. The three-category ledger allows declaring RE2 as a form-(c)
divergence with the linear-time guarantee as the reason.

**Recommendation:** Go's `regexp`, declared, with `CodeUnsupportedRegex` at
Compile naming the construct. The safety property is worth the portability
cost, and the corpus of real transforms uses no regex at all.

## 5. Ship `Access` implementations

**For (integrator):** the interface has nine methods and two dozen semantic
decisions (typed nil, presence, enums, well-known types, key naming); leaving
them to each integrator guarantees no two services agree on what
`$.createdAt` is. The doc names protobuf as the motivating case and ships
nothing for it.

**Against:** scope; protobuf and reflection each pull a dependency and a
policy surface into a package that is otherwise dependency-free.

**Recommendation:** ship them as separate subpackages (`access/proto`,
`access/reflect`) so the core stays dependency-free and the policy is
written once.

## 6. The class README

Six changes proposed by the panel; all frozen because they are the stance.

**6a. Narrow the split.** The value model governs what a value is and how
primitives combine; where the documentation is silent on *what is done*,
the reference suite governs unless a declared divergence says otherwise.
Evidence from the panel: `$number(".5")` (`strconv` accepts, reference
rejects); `$round(1.015, 2)` (`1.02` in the reference, `1.01` on a binary
host); `$string` of floats; string ordering; `$match.index` units. In each
case a practitioner can predict the reference and nobody can predict Go's
standard library without reading it. (pl, practitioner, idiom)

**6b. Agreement or refusal**, replacing "no parity between members." For any
expression and input, two members return equal values or at least one
refuses. Keeps Go's exactness, forces a weaker host to refuse `$$.id + 1`
above 2^53 rather than round, makes cross-host disagreement impossible rather
than "correct." (pl; endorsed by practitioner and integrator)

**6c. Widen rule 8** to any arithmetic or conversion whose result is not
exact in the member's stated model. (pl, idiom)

**6d. Three-category divergence ledger:** (a) value model cannot represent;
(b) fixture contradicts the documentation, passage cited at the pinned
commit; (c) documentation silent and the value model chose. Category (c)
shared across members. (pl)

**6e. Promote the portable core to a profile** with a stated numeric domain
(integers exact to ±2^53, binary64 otherwise) that every member computes
identically. (pl, practitioner)

**6f. Drop "as the language requires"** from the ordered-object claim; the
reference orders integer-like keys numerically before insertion order, so
`*Object` is itself a declared divergence or must match. (pl, idiom)

**Recommendation:** adopt 6a through 6d and 6f now; they make the stance
coherent without changing its substance. Defer 6e until the profile has a
consumer in the SDKs.

## Additions from the iteration-1 panel (2026-09-18)

**Ruling 4, regex dialect, escalated.** All five reviewers of iteration 1
say an unstated dialect is the worst possible state: a practitioner cannot
know whether `(?<=\$)\d+` compiles, and a security reviewer cannot know
whether the engine is linear-time. Three of five (security, PL, integrator)
endorse Go's `regexp` as a declared divergence with a stable compile-time
code, the security reviewer noting it makes this member's regex story
*stronger* than the reference's because RE2 is the safe engine. The
practitioner wants the JavaScript dialect for portability of existing
expressions. The stub still says nothing about the dialect. Recommendation
unchanged: RE2, declared, with `CodeUnsupportedRegex` at Compile.

**Ruling 6a evidence, `$round`.** The pinned documentation states
ties-to-even and gives only non-tie examples (`$round(123.456, 2)`), so it
is silent on whether the value rounded is the binary float64 or its shortest
decimal. The reference shifts the decimal string, so `$round(2.675, 2)` is
`2.68` there. Under "Go's arithmetic" the stub now says `2.67` and states
why. This is the cleanest single example of what ruling 6a decides: the
practitioner can predict the reference and cannot predict the host.

**Ruling 6e evidence, the portable core.** The practitioner, for the second
panel running, shows that `$keys` and `$string` are not portable over a
`map[string]any` input (sorted here, document order in the reference,
integer-like keys first in the reference), so the README's claim that the
portable core yields identical results "without exception" is false unless
the caller supplies ordered objects. Either narrow the claim or drop those
two functions from the list.

**Ruling 5, `Access` implementations.** Two reviewers (idiom, integrator)
again ask for a shipped reflect-based struct Access following `encoding/json`
tag rules, on the ground that a Go programmer's first call will be over a
struct. Still frozen. Note that the iteration-2 `Access` redesign (one
`Resolve` method plus optional view interfaces) makes such an implementation
about a third the size it would have been.

## Additions from the iteration-2 panel (2026-09-18)

**Ruling 4, regex dialect: the loop is blocked on this item.** Third panel
running in which every reviewer flags the unstated dialect. This round the
practitioner writes the sentence they want ("Go's regexp, RE2 syntax, i and
m flags, lookaround and backreferences fail at Compile") and the PL skeptic
proposes a portable regex subset (RE2 ∩ ECMAScript over code points) for
the class. No reviewer in three panels has argued for leaving it unstated.
Recommendation unchanged. The stub still says nothing; `DIVERGENCES.md`
records it as pending.

**Ruling 1, output object type, sharpened.** The idiom reviewer argues from
the README's own text: rule 2 says host-native members evaluate over "the
host's ordinary values," Go's ordinary object is `map[string]any`, and rule
6 already licenses sorted enumeration as a declared divergence, so `*Object`
is "the JavaScript value model re-imported through a library type." The
counter (carriage by identity, decoded document order via `Unmarshal`)
stands, and every other reviewer accepts `*Object` while disliking the
two-arm switch. Recommendation unchanged: keep the split, documented.

**Ruling 7 (new), `Close`.** Panel 1 (idiom, perf, integrator) wanted `Close`
kept without an error return; panel 2's idiom reviewer wants it deleted
because Go has no borrows and `regexp`/`template` hold state without one.
The security reviewer wanted an explicit end to the borrow window. The stub
keeps `Close()`, idempotent, defining the end of sharing. Recommendation:
keep; the memoized-sharing window needs a definite end and `Close` is the
only honest way to give it one.

**Ruling 6a and 6d evidence, verified this round against jsonata-js 2.1.1.**
The reference renders floats at 15 significant digits (`$string(0.1 + 0.2)`
is `"0.3"`) where the documentation specifies `JSON.stringify`; it rounds
`$round(2.675, 2)` to `2.68` on the decimal spelling where the documentation
is silent; it hoists integer-like keys where the documentation is silent.
Under the current README, the first is documentation-over-reference (the
stub follows the docs), and rule 6 as written does not permit it, which is
exactly the defect 6d fixes. Both reviewers who raised it note that
`$string` and `&` on floats will be the first divergence any author notices.

## Additions from the iteration-3 panel (2026-09-18)

**Ruling 4, regex dialect: fourth panel, still blocked.** All five
reviewers again. New arguments: the JavaScript member's author notes that
honoring "characters are code points" forces the `u` flag, under which
ECMAScript's own engine is stricter than the reference (`/\-/u` is a syntax
error), so even the JavaScript member cannot be dialect-identical to the
reference; the PL skeptic names RE2's linear-time guarantee as a security
property a backtracking port cannot give and asks that the ruling say so;
the practitioner writes the sentence for the third time. Recommendation
unchanged: RE2, declared, with `CodeUnsupportedRegex` at Compile, and the
class defining a portable subset (RE2 ∩ ECMAScript over code points).

**Ruling 6, the class README: escalated.** Four of five reviewers (idiom,
integrator, js, pl) independently reach the same finding from different
directions: the Go doc's numeric tower, code-point strings, full case
mapping, member order, and refusal rules are decisions no host makes, the
Go member made them well, and the README then attributes them to "the
host", which licenses a second member to differ in exactly the places
OpenBindings authors stand (`$string` of a number, `$keys`, arithmetic near
2^53, regex). The idiom reviewer's form of it: Go's `regexp` implements a
*written* syntax specification, RE2, and ships one; the analogy argues for
a written class value model, not against it. The JavaScript author's form:
"without the profile I can promise a faithful ECMAScript-flavored dialect
of the documentation, which is a second thing, not a second member."

6e (the profile) is therefore escalated from "defer until a consumer" to
"required before a second member is started." New sub-items:

**6g. A precedence order over authorities** (pl): normative prose, then
examples, then an authority the documentation incorporates by reference
(`JSON.stringify`, XPath picture strings, regex syntax) read as specifying
the algorithm over the abstract value with the incorporating host's value
model replaced by the member's, then the value model, then the reference's
behavior. Without the substitution rule, incorporating `JSON.stringify` for
digits while rejecting its number model is unprincipled.

**6h. Rewrite rule 6** (pl, practitioner, js): a divergence is a departure
from the reference's *observable behavior*, not from the suite; three
classes (documentation says otherwise; documentation silent and the value
model decides; reference implements what the documentation does not
describe); the suite is the detection floor; a member adds a fixture for a
departure no fixture exercises and runs differential tests against the
reference. The ledger was re-tagged with these classes this iteration as
evidence; four of its rows violate rule 6 as written.

**6i. Rule 7** (pl, js): "closed except for the clock, entropy, and a
caller-supplied resolver that presents values, never functions." The API
has had that door since iteration 2 and the rule denies it.

**6j. Rule 9** (pl): "unobservable in success"; `Select` is not a guard,
so selection is observable when `Complete` would fail.

**6k. Member order** (js, ties to 6f and ruling 1): either the class pins
ECMAScript's own-property order (array-index keys ascending, then
insertion), which the documentation is written against and `$string`
inherits through `JSON.stringify`, and `Object.Set` hoists; or the class
declares order host-owned and OpenBindings authors are told `$keys` order
is not portable. The JavaScript host cannot give plain insertion order
without `Map`, which no caller holds.

**6l. Class-owned tables** (js): the E codes and their classes; the
position unit for the suite (code points; byte `Offset` a Go extra); the
`Limits` fields and what one node and one unit of work are, or budget
refusals are undeclarable divergences; `Now` at millisecond resolution;
the properties a member's decoder must have if it ships one (exact
integers, duplicate-name policy, unpaired-surrogate policy); the `Reason`
vocabulary and the `Reads()` opacity rules, because the SDK parity rule
makes them observable.

**6m. The portable-core claim is false as written** (practitioner, third
panel; js): `$string`, `$keys`, `$merge`, and `$sift` are in the listed
subset and in the ledger. Narrow it ("over string, boolean, null, and
integer values, and objects without integer-like keys") or make it the
profile of 6e.

**Ruling 8 (new): the numeric model.** The two items flagged for veto in
iterations 2 and 3 have now been argued by three reviewers each and belong
in the queue rather than in a changelog footnote.

*8a. `$round` basis.* Current: half to even on the binary float64
(`$round(2.675, 2)` is `2.67`). For the decimal spelling (practitioner):
the reference does it, every user has calibrated to it, and the
documentation's examples are decimal. For pinning either as a class
algorithm (js): the float64 is bit-identical on every host, so this is not
a value-model consequence and the ledger's justification was wrong; two
binary64 members with different tie algorithms disagree on
`$round(0.125, 2)`. The ledger row now says "interpretation".
Recommendation: keep the binary basis (it is the value's rounding, and
decimal-spelling rounding is a rendering artifact), and pin it at class
level under 6e so every binary64 member matches.

*8b. Refuse or round once on an inexact mixed conversion.* Current:
`9007199254740993 + 0.5` is `CodeInexact`; `9007199254740993 / 2` is the
exact quotient rounded once. The doc now states the principle (operands
are never converted inexactly; a non-integral result of exact operands is
rounded once). For round-once everywhere (pl): both cases lose the same
information for the same reason, and `1/3` proves entering binary64 is
definitional, not approximation; refusal should be reserved for non-finite,
division by zero, and `MaxIntegerBits`. Recommendation: keep refusal; the
distinction between converting an operand and rounding a result is the
one that lets `$$.id + 0.5` fail loudly instead of corrupting the ID.

*8c. The 2^53 integrality cap*, applied this iteration: an integral float64
of magnitude at most 2^53 is an integer. Without a cap `1e300 + 1` is a
301-digit exact integer computed from a float; with it, `1e23` stays a
decimal. Flagged; no reviewer argued against a cap.

**Ruling 1 evidence.** The JavaScript author: JS has one object type and
its order is ECMAScript's; the `map`/`*Object` split has no mirror. If 6k
pins ECMAScript order, `*Object` becomes the type that implements it and
the ruling-1 argument for uniform `*Object` strengthens.

**Ruling 3 evidence.** The integrator's sketch selects two fields from
separate goroutines with `errgroup` and relies on the documented safety;
the idiom reviewer asked what a concurrent `Select` of the same field does
(now stated: it waits). One consumer for the current answer.

**Ruling 5 evidence.** The integrator's protobuf Resolver sketch runs to
roughly eighty lines with six marked guesses (JSON versus proto field
names, presence, enum rendering, well-known types, map keys, element
conversion context). Third panel asking for a shipped implementation.

## Additions from the iteration-4 panel (2026-09-18)

**Ruling 3, `Evaluation` concurrency: the performance engineer costs it.**
The memo itself is cheap (an atomic state word per field, a channel only on
contention), but "one budget across all selections" under concurrent
`Select` makes every budget decrement an atomic add in the inner loop, or
forces reservation windows whose failure point differs from sequential
selection. The stub now admits the second consequence. The reviewer's
recommendation is to drop the promise; the integrator's iteration-3 sketch
is the one consumer that used it. Recommendation unchanged:
single-goroutine by rule.

**Ruling 7, `Close`: a second idiom reviewer argues for deletion.** The
argument: `Close` releases nothing the collector would not, its borrow
contract cannot be enforced, and the doc undercuts it in the next
sentence. The counter stands: the memoized-sharing window needs a definite
end. Recommendation unchanged: keep.

**Ruling 8, the numeric model.**

*8a, `$round`.* The practitioner argues for the decimal spelling with a new
ground: the documentation's tie examples (`$round(11.5)` is `12`,
`$round(12.5)` is `12`, verified) show half-to-even on decimal ties that are
exactly representable, which decides nothing about inexact ties but shows
the documentation thinks in decimal digits; and a price with a trailing 5
is the case an invoice transform hits. The ledger row now says
"interpretation, pending" and records the exactly-representable point.
Recommendation unchanged: keep the binary basis and pin it at class level.

*8b, refuse or round once.* Now three reviewers across two panels (pl in
iteration 3; practitioner and idiom in iteration 4). The practitioner
shows the asymmetry a user will hit: `ts_ns / 1e9` succeeds and `ts_ns *
1e-9` fails for the same nanosecond timestamp, because `1e9` is integral
and `1e-9` is a decimal; the stub now states that consequence in one line.
The idiom reviewer adds that if refusal is a class rule (README rule 8),
`CodeInexact` should be a class-level obligation so that a member which
rounds silently is non-conformant rather than merely different; that is
ruling 6 material and recorded there. Recommendation unchanged: keep
refusal, with the stated principle (operands are never converted
inexactly; a non-integral result of exact operands is rounded once).

*8c, the 2^53 cap: removed this iteration.* The PL skeptic showed the cap
broke substitutivity (`float64(9007199254740994) + 1` rounded to
`...996` while the equal int64 gave `...995`, verified). Every integral
float64 is exactly some integer and fits `MaxIntegerBits`, so the cap was
not needed for exactness; the stub now treats any integral float64 as the
integer it denotes, at the cost that `1e23 + 1` is the 23-digit exact
integer and `$string(1e21)` is digits rather than `"1e+21"` (both in the
ledger). Flagged for veto; the alternative is to keep the cap and strike
"never of the representation" from the doc.

**Ruling 9 (new): the typed exit.** Three reviewers want a supported way
from a result into typed Go. The idiom reviewer wants a finite output
model: `Materialize` (or a `Canonical`) that yields only `nil`, `bool`,
`string`, `[]byte`, `int64`, `*big.Int`, `float64`, `[]any`, and `*Object`,
on the `encoding/json` precedent of promising what `Unmarshal` into `any`
yields, and argues carriage is a property of `Eval` while normalization is
an opt-in second step. The integrator wants `Int64`, `Float64`, `String`,
`Index`, and a `Decode(ctx, v, env, target)` with `json` tag semantics and
exact int64 fields, on the ground that the alternative every team writes
is a round trip through JSON text that re-introduces the 2^53 loss. The
practitioner wants the type of every result shape stated. `Int64` and
`Float64` were applied this iteration as helpers that prejudge neither
design. Recommendation: a `Canonical(v any) any` with the idiom reviewer's
contract, not a normalizing `Materialize` (identity return is worth
keeping), and `Decode` deferred until an SDK consumer needs it.

**Ruling 1 evidence.** The idiom reviewer counts the `map`/`*Object`
bifurcation as the largest ergonomics tax and calls `Member` a bandage;
the practitioner and PL skeptic say the same in different words. No new
argument for either uniform choice.

**Ruling 4, regex dialect: fifth panel.** The PL skeptic observes that the
stub has already ruled in all but name: "Go's `${name}` form is not
recognized" and regex compiled at `Compile` describe Go's `regexp`, and "a
pending row is not a declaration". The integrator says this ruling matters
more than any numeric one because the linear-time property is what makes
untrusted regex literals safe. The idiom reviewer predicts it will be the
first issue filed and that users will experience it as "Go's regex is
broken", exactly as with `regexp`. Recommendation unchanged.

**Ruling 5 evidence.** The integrator's protobuf Resolver runs to sixty
lines again, with the `[]*pb.Item`-is-foreign-as-a-whole trap caught only
on a second read. Fourth panel.

**Ruling 6, the class README: new sub-items.**

*6n. An authority ladder* (pl, second time): documentation text; an
authority the documentation incorporates by reference (`JSON.stringify`,
"regular expression", "characters"), applied over the member's value
model; a recorded interpretation where the text admits readings; the value
model where the documentation is silent about values; an engine refusal.
The ledger now carries an **incorporated** class as evidence.

*6o. Rule 3 is vacuous as written* (pl): "owes no more than its host offers
and must not offer less" is contradicted by `*big.Int` promotion (more than
Go's wraparound) and the absence of `complex128` (less). Replace with: the
member's value model is stated, and it is built from host types without an
intermediate tree.

*6p. Rule 1 versus rule 6 are circular* (pl): membership requires passing
the reference suite, which encodes departures from the documentation; the
two reconcile only through the ledger, which rule 6 restricts to
unrepresentable values.

*6q. Refusal as a class obligation* (idiom): if rule 8 is a class rule,
`CodeInexact` and the refusal on inexact conversion should be class-level,
so that a member which rounds silently is non-conformant.

*6m evidence.* The practitioner, for the fourth panel, shows the
portable-core claim is false for `$string` of any non-integral number and
for `$keys`/`$merge`/`$sift` with integer-like keys, and proposes the
narrowed sentence.

*Concept row.* Medians across the four panels of the loop: B, B+, B, B+.
The PL skeptic's grade is C+ in every panel and always for the same
reason: the README's two-tier authority story does not survive its own
ledger. Every other lens grades the concept B- or better and says the API
is stronger than the README that introduces it.
