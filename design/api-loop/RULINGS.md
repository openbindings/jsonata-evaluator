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
