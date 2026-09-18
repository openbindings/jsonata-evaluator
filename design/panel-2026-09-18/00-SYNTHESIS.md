# Panel synthesis, 2026-09-18

Five cold reads of the class README and the API stub, each with a distinct
lens, none given the design rationale or the open questions already raised.
The five reports are beside this file, verbatim. This file is what they
agree on, where they split, and what follows. Arguments, not a verdict.

## Grades

| | Go idiom | Performance | Integrator | PL skeptic | Practitioner |
| --- | --- | --- | --- | --- | --- |
| Idiomatic fit for Go | B- | B+ | B | B+ | B+ |
| Ergonomics, common path | B | B | B- | B | B |
| Correctness and footguns | C+ | B- | C | C- | C- |
| Performance headroom | B | B- | B+ | B+ | B+ |
| Concept soundness | B | B+ | B+ | C | C |
| Overall | B- | B | B- | C+ | B- |

Two rows carry the story. Correctness is the weakest row for every reviewer.
Concept splits cleanly: the three engineers give it B to B+, the two
semanticists give it C, and the split is over one sentence.

## What five independent readers converged on

**1. Key order of a `map[string]any` input is undefined, and it breaks
determinism.** Four of five found this cold. `$keys`, `$each`, `$merge`,
`$sift`, and the transform operator over a Go map are nondeterministic run to
run, which violates rule 9 and every reasonable expectation. Every reviewer who
raised it proposed the same fix: sorted key order for maps, insertion order
for `*Object`, stated in the package doc.

**2. The numeric model is underspecified at exactly the points the library
exists for.** All five. What Go type is the literal `1`? What is
`9007199254740993` as a literal? If literals parse to `float64`, as every
port's parser does, then `orders[id = 9007199254740993]` compares
`9007199254740992.0` against `int64(9007199254740993)` and silently matches
nothing. The practitioner's phrasing: "this one line decides whether the
library delivers its headline promise." Also unspecified: promotion widths,
mixed signedness, `float32`, `json.Number` classification, non-finite inputs.

**3. Rule 8 contradicts the arithmetic paragraph.** Three of five. "Refuses
rather than approximates" and "any float64 operand yields float64" cannot both
hold for `*big.Int + float64` or `int64 + float64` above 2^53. One has to give.

**4. The authority stance is a hybrid described as a monarchy.** The PL
skeptic hardest, the practitioner and the idiom reviewer independently.
Rule 1 gates on the reference suite; rule 6 permits departure only for
unrepresentable values. So wherever the documentation is silent or contradicts
a fixture, the member must match the fixture, and "documentation as authority"
is overruled by its own gate. Rule 6's single category cannot even express the
situation the README calls fundamental. The proposed repair: say what it is.
Documentation where it speaks and agrees with the suite; the suite where the
documentation is silent; a ledger with three categories (unrepresentable,
fixture contradicts documentation with the passage cited, documentation silent
and the value model chose) and the third category shared across members.

**5. "No parity between members" is wrong for this use case.** Four of five.
The PL skeptic: a transform's meaning becomes "a function of (expression,
input, evaluator)," which for a document that ships to unknown clients is an
abdication. The practitioner: "a portability bug with a philosophy attached."
The integrator wants a boundary promise, not a value-model philosophy. The
idiom reviewer: right for carriage, wrong for portability, and the README
treats them as one virtue.

**6. Budgets are unstated and incomplete.** Three of five; two would block a
release on it. No defaults for depth, recursion, or output nodes, so a reader
must assume unbounded, and an unbounded recursion in Go is a fatal stack
overflow, not a recoverable error. No work budget at all, so
`$count([1..1e8])` is a one-node result with an 800 MB intermediate that no
existing option and no context deadline bounds.

**7. Output types are heterogeneous.** Four of five. `$.user` returns the
caller's `map[string]any`; `{ "user": $.user }` returns `*Object`. Every
consumer writes a two-arm type switch. The idiom reviewer's position is the
strictest: either constructed objects are `map[string]any` by default with an
opt-in for order, or `*Object` is used uniformly on input and output.

**8. The import path ends in the keyword `go`.** Two of five, and objectively
right. `github.com/openbindings/jsonata-evaluator/go` forces an alias at every
import site forever. The package needs a subdirectory: `go/jsonata/`.

**9. `Access` is too large and cannot fail.** Three of five. Nine methods is a
hand-rolled `reflect.Value`; `Number(v any) any` "exactly" has nowhere to go
when exactness is impossible.

**10. `Close() error` has nothing to error.** Three of five.

## Where they split

**The concept, at one sentence.** The engineers accept "the value model wins
where the documentation is silent" and grade the concept well. The
semanticists accept the *value model* (exact integers, ordered objects, real
bytes, refusal on overflow) enthusiastically, and reject extending "the host
decides" from *what a value is* to *what is done* where the documentation
happens to be silent. The practitioner's list of what the documentation is
silent on: the grammar `$number` accepts (`".5"`, `"+5"`, `"0x1F"`,
`"Infinity"`, which `strconv` accepts and the reference rejects), how `$round`
finds a midpoint (`$round(1.015, 2)` is `1.02` in the reference and `1.01`
under a binary host), how floats print, how strings order, which regex
features exist, what `$match.index` counts. In each case a practitioner can
predict the reference and cannot predict Go's standard library.

**The regex analogy.** The idiom reviewer: sound. The integrator: "a set of
guarantees I already know how to operate." The performance engineer: apt for
dispatch, wrong for bounding, because RE2 is linear by construction and JSONata
is not. The PL skeptic: decorative for the semantic claim, load-bearing only
for "no intermediate tree," and the wrong precedent from its own domain, since
where regexes were embedded in cross-system contracts the response was to pin a
dialect (OpenAPI's `pattern` is nominally ECMA-262) rather than let each host
decide. The practitioner: "holding up the failure mode."

**The regex dialect specifically.** This is where the split has a concrete
cost. Under "host decides what is done," the dialect is Go's `regexp`, which
is a linear-time safety win (the performance engineer) and a real loss for
every `$match` written against messy vendor strings with lookaround (the
practitioner). Under "reference decides where the documentation is silent," the
dialect is JavaScript's, which requires a backtracking engine in Go and brings
its pathological cases. The PL skeptic's three-category ledger reconciles it:
declare the dialect as a form-(c) divergence ("documentation silent, host chose
RE2 for its linear-time guarantee"), shared across the project's members.

## The single best idea to come out of the panel

**Agreement or refusal**, from the PL skeptic, replacing "no parity between
members." For any expression and input, two members either return equal
values or at least one refuses. This keeps the Go member's exactness, forces a
weaker host to fail loudly on `$$.id + 1` above 2^53 rather than round quietly,
and makes cross-host disagreement impossible rather than "correct." The
README's rule 8 is, in the reviewer's words, "two words away from saying this
already." It delivers the project's original goal, the same results from both
SDKs, without requiring identical value models, and it is the one change that
turns the practitioner's C into something they said they would use.

## What follows for the class README

Proposed, for ruling. Each is a wording change; together they change the
stance from "host wins where silent" to "host defines values, reference
decides operations where the documentation is silent, and members agree or
refuse."

1. Narrow the split: the value model governs what a value is and how
   primitives combine; where the documentation is silent on *what is done*,
   the reference suite governs unless a declared divergence says otherwise.
2. Replace "No parity between members" with "Members agree or refuse."
3. Widen rule 8 to any arithmetic or conversion whose result is not exact in
   the member's stated model.
4. Give rule 6 three categories and make the third shared across members.
5. Promote the portable core from an observation to a profile with a stated
   numeric domain (integers exact to ±2^53, binary64 otherwise), which every
   member computes identically. The full language remains available beyond it.
6. Drop "as the language requires" from the ordered-object claim; the
   reference orders integer-like keys numerically before insertion order, so
   `*Object` is itself a declared divergence or must match.

## What follows for the API

Changes with no plausible objection, which I would make without a ruling:

- Sorted key order for `map[string]any` inputs; stated.
- A "# Numbers" section in the package doc: literal typing (integral literals
  are `int64`, `uint64` when only that fits, `*big.Int` otherwise; others
  `float64`), three result types, promotion at admission, `json.Number`
  classification, non-finite inputs refused, and the value function every
  observing operation is a function of.
- Stated finite defaults for every bound, plus `WithMaxWork` counting nodes
  evaluated and elements materialized. Budgets accepted at `Compile` so they
  travel with the untrusted artifact.
- `Close()` with no error, or no `Close` at all if nothing is pooled.
- Package at `go/jsonata/`.
- `Access.Number(v any) (any, error)`; `Keys` replaced by a `Range` callback.
- `Object`: `All() iter.Seq2`, useful zero value, `UnmarshalJSON`, sized
  constructor.
- `String()` in place of `Source()`, satisfying `fmt.Stringer` as `regexp` does.
- `Plan.Reason` as typed constants; `Mode` constants prefixed so `Complete`
  the constant and `Complete` the method are not the same identifier.
- Options resolved once via `(*Expression).With(opts...)` so per-request
  `Access` and `BytesEncoding` stop allocating.
- `$now`/`$millis` snapshotted at `Prepare` so they are selectable; block
  preludes `($x := ...; { ... })` selectable with the prelude shared;
  duplicate constructor keys an error under `Select`; rule 9 stated as "when
  `Complete` would succeed."
- `text.Evaluate` returns `([]byte, bool, error)` and accepts a compiled
  expression.
- An exported constant naming the documentation commit the member targets.

Changes needing a ruling:

- Output object type: uniform `map[string]any` with opt-in order, uniform
  `*Object`, or the current split with the switch documented.
- `Select` path as `...string` (constructor keys only) or compile-resolved
  `Field` handles (the performance engineer's proposal, which also removes the
  per-call allocation and lock).
- `Evaluation` concurrency-safe or single-goroutine by rule.
- Regex dialect: RE2 as a declared divergence, or a backtracking engine for
  the reference's dialect.
- Ship `Access` implementations for protobuf and reflection in subpackages.

## Where the design was wrong

Two things I built into the README and the API, and the panel was right to
hit them.

The extension of "host decides" from values to operations. I argued that the
documentation's silence on the regex dialect made it the host's, and by the
same logic `$number`'s grammar and `$round`'s midpoint would be Go's. The
practitioner's list shows that the documentation is silent on most of what a
practitioner relies on, and that "the host's" in practice means "whatever
`strconv` does," which nobody can predict without reading it. The value model
was the right idea; letting it annex operations was not.

The regex analogy as an argument. It was useful for one narrow claim, no
intermediate tree and native per host, and I let it carry the semantic claim
too. Two reviewers pointed out that regex is the domain where per-host
dialects were tried and had to be pinned back the moment patterns crossed
system boundaries, which is exactly this situation. Keep the implementation
lesson; stop citing the analogy as evidence that divergence is fine.
