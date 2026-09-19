# Ruling queue

Decisions the loop may not make. Each carries the strongest argument on each
side and a recommendation. The stub keeps its current answer until a ruling
lands. A ruling still open when the loop stops becomes a GitHub issue on this
repository.

The loop stopped on 2026-09-18 (see `FINAL.md`). Every ruling below is
filed as an issue, numbered to match:
[#1](https://github.com/openbindings/jsonata-evaluator/issues/1) object
type, [#2](https://github.com/openbindings/jsonata-evaluator/issues/2)
Select path, [#3](https://github.com/openbindings/jsonata-evaluator/issues/3)
Evaluation concurrency,
[#4](https://github.com/openbindings/jsonata-evaluator/issues/4) regex
dialect, [#5](https://github.com/openbindings/jsonata-evaluator/issues/5)
Resolver implementations,
[#6](https://github.com/openbindings/jsonata-evaluator/issues/6) the class
README, [#7](https://github.com/openbindings/jsonata-evaluator/issues/7)
Close, [#8](https://github.com/openbindings/jsonata-evaluator/issues/8) the
numeric model, [#9](https://github.com/openbindings/jsonata-evaluator/issues/9)
the typed exit,
[#10](https://github.com/openbindings/jsonata-evaluator/issues/10) the
decimal decode boundary,
[#11](https://github.com/openbindings/jsonata-evaluator/issues/11) decoder
stance, [#12](https://github.com/openbindings/jsonata-evaluator/issues/12)
the absence triple.

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

## Additions from the iteration-5 panel (2026-09-18)

**Ruling 4, regex dialect: sixth panel; the security reviewer calls it the
ReDoS decision.** A backtracking engine under a budget that charges one
unit per call is catastrophic backtracking by construction
(`$match($$.s, /^(a+)+$/)`); RE2 is linear but a large program over an
uncharged subject was still unbounded, which iteration 6 fixed by charging
subject length and program size. The reviewer also notes `$eval` hands the
pattern to the payload author, so refusal must happen at `$eval` too. Both
the practitioner and the PL skeptic now believe the documentation's
regular-expression page names JavaScript syntax; if so the ledger's
authority class is "incorporated", not "silent", and the ruling is forced
toward declaring what RE2 cannot do as engine refusals rather than toward
choosing a dialect. Verify the pinned page before ruling. Recommendation
unchanged: Go's `regexp`, declared, unsupported constructs refused at
Compile and at `$eval` with a stable S code.

**Ruling 7, `Close`.** A third idiom reviewer argues for deletion ("if there
is nothing behind it, do not ship it"); the security reviewer wants it kept
and blocking, because the alternative is a fatal map race when a caller
modifies its input while another goroutine's `Select` is still walking it.
Iteration 6 made `Close` block. Recommendation unchanged: keep.

**Ruling 8a, `$round`.** Two reviewers this panel (practitioner, pl) make
the same new argument: `$string(2.675)` renders `"2.675"` here, so the
package tells the user the value is 2.675 and then rounds it as if it were
below; the number the package shows and the number it rounds should agree,
and the shortest round-trip decimal is well-defined and host-independent.
That is the strongest argument yet for the decimal basis, and it removes
the ledger row. Recommendation revised: judge the tie on the shortest
round-trip decimal, and pin that algorithm at class level under 6e.

**Ruling 8b, refuse or round once.** The PL skeptic shows the doc's stated
principle did not justify the refusal: `0.5` is an exact value, so "an
operand is never converted inexactly" does not distinguish `9007199254740993
+ 0.5` (refused) from `9007199254740993 / 2` (rounded once to
`4503599627370496`, which is then an exact integer). Iteration 6 states the
two as two policies and names the reason for the refusal (an identifier
losing digits on entering the decimal domain). Four reviewers across three
panels now argue for one rule; none argues for the current pair as a
principle, only as a policy. Recommendation unchanged (keep refusal), with
the note that the alternative is now fully specified in the PL skeptic's
report: compute exactly, keep integral results exact, round non-integral
results once, reserve refusal for non-finite, division by zero, and
`MaxIntegerBits`.

**Ruling 9, the typed exit.** The integrator asks again for `Canonical`
(every object `*Object`, every array `[]any`, every number
`int64|float64|*big.Int`) and now also for `Project` (multi-select into an
`*Object`). `Clone` was applied this iteration for a different reason
(ownership) and is not a normalizer. Recommendation unchanged.

**Ruling 10 (new): the decimal decode boundary.** The idiom reviewer argues
that decoding `0.1000000000000000055511151231257827` to float64 at
`Unmarshal` breaks README rule 4 (exact carriage) for decimals the way
`encoding/json` breaks it for integers, and proposes carrying as
`json.Number` any decimal token whose shortest float64 rendering does not
reproduce it. The PL skeptic instead asks that the decode boundary be
stated as the value function (which iteration 6 did). The two positions:
representation-preserving decode (fidelity to the token; a per-read
classification cost on rare values; `Marshal` re-emits the token) versus
value-fixed decode (one number type for decimals; simpler laws; the token's
extra digits are lost by definition). Recommendation: keep value-fixed
decode; a decimal in this model is a float64, and the class's rule 4 is
about values, not tokens.

**Ruling 1 evidence.** Every reviewer this panel names the
`map`/`*Object` split as the largest ergonomic cost; the idiom reviewer
calls `Member` a bandage and the integrator counts roughly fifteen concrete
result types. No new argument for either uniform choice.

**Ruling 3 evidence.** The security reviewer wants the concurrent
`Evaluation` kept (an `Evaluation` shared across goroutines is a real
pattern in fan-out handlers) but with `Close` blocking; the performance
engineer in iteration 4 wanted it dropped. The loop-controlled text now
states both the waiting semantics and the budget caveat.

**Ruling 5 evidence.** Fifth panel; the integrator's protobuf Resolver is
again sixty lines with the `[]*pb.T`-is-foreign trap, and the reviewer
adds the observation that no int64 presentation is right on both the
expression side (`id + 1` needs an integer) and the JSON wire side
(`protojson` quotes int64 because JSON consumers round), which is exactly
why the project should ship the decision once.

**Ruling 6, the class README: evidence and the post-loop task.** The PL
skeptic (fifth panel, fifth C+) restates the authority ladder as 6n, shows
rule 6 as written disallows roughly ten of the ledger's fifteen rows, and
proposes the concrete rule-6 and rule-8 rewrites; the practitioner, idiom,
and integrator reviewers each independently say the "portable core" must
become a tested profile. The security reviewer notes the documentation
defines no security-relevant behavior at all, so every such rule is an
engine rule the class should own (6l). The first post-loop artifact both
PL reviewers ask for is a class-level semantic core under `suite/`: the
value domain, the value function, the three rounding boundaries, member
order as representation, and the authority precedence, with the reference
fixtures re-tagged (`documentation`, `reference-only`, `value-model`) and
the laws as property tests.

*Concept row across the loop.* Medians: B, B+, B, B+, B. The PL skeptic's
C+ is constant and always for the README; the security reviewer grades the
concept B+ and says host-native evaluation narrows the attack surface by
removing the second parse and widens it by reading caller structures by
reference, which iteration 6's depth and work charging addresses.

## Additions from the iteration-6 panel (2026-09-18), the loop's last

**Ruling 8c, the integrality boundary: three positions.** Iteration 4
capped integrality at 2^53 (an integral float64 above it was a decimal);
iteration 5 removed the cap after the PL skeptic proved it broke
substitutivity (`float64(9007199254740994) + 1` and the equal int64 gave
different results). The iteration-6 panel attacks the removal from three
seats. The PL skeptic: the value function is lexical (a `.` or `e` in the
token decides the value) while the doc swears values are independent of
provenance, and the result manufactures digits (`1e23 + 1` is
`99999999999999991611393`; `1e300 * 1e300` is a 601-digit exact integer
where the reference gives D1001); the proposal is to decode every numeric
token by its mathematical value, so `1e23` and `9007199254740993.0` are
the integers they spell. The practitioner: exponent spellings are
magnitudes, not counts; restore the cap so a float64 at or beyond 2^53 is
a decimal that renders as `JSON.stringify` does. The idiom reviewer: draw
the boundary at 2^53, where `CodeInexact` already draws it. The three
rules and the law each violates:

- *Cap at 2^53* (iteration 4): violates substitutivity (equal values,
  unequal results under `+ 1`).
- *No cap, integral float64 is its integer* (current): honors
  substitutivity; renders and computes binary artifacts as exact digits;
  overrides `JSON.stringify` for values inside its domain.
- *Decode by mathematical value* (pl, iteration 6): honors substitutivity
  and rule 4 at the token boundary; makes `9007199254740993.0 =
  9007199254740993` true; makes `1e23` a 24-digit exact integer, so
  `Unmarshal` of a payload full of exponent-spelled magnitudes produces
  `*big.Int` values, and `$string(1e300)` is 301 digits.

Recommendation: the practitioner's cap, restated so it does not break
substitutivity: an integral float64 of magnitude at most 2^53 is an
integer; above it, a float64 is a decimal and an int64 or `*big.Int`
compared with it is compared exactly but combined with it under the
decimal rule. Substitutivity then holds because no int64 above 2^53 is
equal to any float64 except an integral one, and that pair combines under
the same rule from either side once the decimal rule is uniform (8b).

**Ruling 8b.** The PL skeptic's grouping test: `(1e300 * 1e300) * 1.5` is
E1008, `1e300 * (1e300 * 1.5)` is D1001, and `1e300 * 1e300 * 1e300`
succeeds with a 900-digit integer: three outcomes for one product. The
practitioner and PL skeptic both ask for one rule; the idiom reviewer keeps
refusal in the keep list. Five reviewers across four panels for one rule;
recommendation: round once, with refusal reserved for non-finite,
division by zero, and `MaxIntegerBits`, unless Matt holds that the
identifier-digit-loss case must refuse, in which case `/` must refuse too.

**Ruling 8a.** The PL skeptic notes that under the package's own value
function `2.675` is not a tie (it denotes 2.67499999...), so half-to-even
has nothing to decide and the authority is the value model, not
interpretation; the practitioner repeats the financial-rounding argument
for the decimal spelling. Recommendation unchanged from iteration 5: judge
the tie on the shortest round-trip digits and pin it at class level.

**Ruling 4, regex dialect: sixth panel, all five reviewers.** The idiom
reviewer: the README's own analogy settles it, a Go member uses
`regexp/syntax` and refuses what it does not support, and leaving it
pending while asserting the analogy is the one place the design is
inconsistent with itself. The practitioner enumerates what would be refused
and what differs silently (`\s`, case folding). Recommendation unchanged.

**Ruling 6.** The PL skeptic's law audit (L1 through L16) is the most
complete statement of what the README must say; the writer independently
finds rule 6 forbids most of the ledger and the portable-core claim is
false on the ledger's first row. Both PL reviewers and the writer ask for
the value model as a language-neutral document under `suite/`. First
post-loop task.

**Ruling 1.** The idiom, integrator, and practitioner lenses each name the
object split as the largest ergonomic cost, sixth panel running.

**Ruling 2.** The integrator shows `Select(ctx, "id", "name")` reads as two
fields and means the path `id.name`; the idiom reviewer asks for a required
first key. Evidence for `Field` handles or for `Select(ctx, key)` plus
`SelectPath`.

**Ruling 3.** The idiom reviewer argues single-goroutine `Evaluation` from
the stdlib's per-input handles (`sql.Rows`, `json.Decoder`) and notes that
concurrent selections make `CodeBudget` unreproducible, which iteration 6
had to admit in the doc. Recommendation unchanged: single-goroutine.

**Ruling 7.** Third consecutive idiom reviewer for deleting `Close`; the
iteration-6 blocking `Close` without a ctx is the new objection
(`http.Server.Shutdown` takes one).

**Ruling 9.** The integrator asks for `Canonical`, `SelectFields`, and
32-bit rendering of a carried `float32`; the practitioner and integrator
ask for a per-expression divergence report or lint.

**Ruling 11 (new): decoder stance versus evaluator divergence.** The PL
skeptic observes that duplicate-name and unpaired-surrogate refusal are
policy, not malformation, and that the ledger itself calls them "a binding
decision this member's decoder makes"; by the README's layering they
belong to the binding. Options: keep them as `Unmarshal`'s declared stance
under E1006; make them decoder options; move them to a separate code.
Recommendation: keep the refusal, rename the ground in the ledger from
"engine" to "decoder stance", and state in `Unmarshal` that a binding
which needs last-wins decodes with `encoding/json` and `UseNumber`.

**Ruling 12 (new): the absence triple.** The idiom lens proposed replacing
`(value, present, err)` with an `Undefined` sentinel in three panels; the
PL skeptic and practitioner defended the triple in two. The argument for
the sentinel: the doc documents its own footgun (`v, _, err`), and a
sentinel fails loudly (`Marshal(Undefined)` errors). The argument against:
comma-ok is the map idiom, a sentinel `any` has no stdlib precedent, and
`NoInput` already exists for the input side only. Recommendation: keep the
triple.

**Concept row, final.** Medians across the six panels: B, B+, B, B+, B, B.
The PL skeptic graded C+ in every panel, always for the README; every
other lens graded B- or better and said the API is stronger than the README
that introduces it. The loop's evidence for ruling 6 is that this finding
was reached independently by eleven reviewers of four lenses.
