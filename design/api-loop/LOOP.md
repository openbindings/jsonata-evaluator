# The API loop

> Draft for review, 2026-09-18. A development loop for the Go member's public
> API surface: apply what a cold-read panel found that is objectively good,
> re-present to a fresh panel, repeat until the panel stops finding such
> things. Runs unattended between rulings.

## What is under iteration

The **API surface**: the stub package at `go/jsonata/` (every exported
signature and doc comment as it would ship, bodies unimplemented), plus the
`text/` convenience subpackage. It must pass `gofmt` and `go vet` at every
iteration. Nothing is implemented; only the contract is iterated.

**Not under iteration:** the class README. Panel findings that would change
the README's stance go to the ruling queue. The README changes only when a
ruling lands.

## One iteration

1. **Triage** every finding from the previous panel into exactly one bin.
2. **Apply** the Apply bin to the stub. Every added sentence is a decision,
   never a hedge. `gofmt`, `go vet`, and the `base64` compile-time assertion
   must pass.
3. **Record** `iteration-N/CHANGES.md`: each applied change with the finding
   and reviewer that motivated it; each ruling-queue addition with both sides;
   each rejection with a one-line reason.
4. **Panel**: five fresh cold reads. Identical prompt each iteration except
   the artifact. Reviewers are given the README and the stub only, are told
   not to read anything else in the checkout, and never see prior panels,
   the changelog, or the ruling queue. Reports are saved verbatim to
   `iteration-N/panel/`.
5. **Check the stopping conditions.** If none holds, go to 1.

## The triage test

A finding goes in the **Apply** bin only if all four hold:

- **(a) It is one of these kinds:** a contradiction between two statements;
  a behavior the API forces but does not state; an unbounded or unstated
  default on a resource; a Go idiom violation with standard-library precedent;
  a compile-level or naming defect (keyword path, colliding identifiers);
  a missing statement about ownership, aliasing, concurrency, or lifetime; or
  an error-surface gap a caller cannot work around.
- **(b) It does not change an OB-design ruling or a frozen item** (below).
- **(c) It is verifiable:** the fix compiles, a sentence that was absent now
  exists and commits to one behavior, or a contradiction is gone.
- **(d) It has grounds beyond one opinion:** two or more reviewers converged
  on it, or it has standard-library precedent, or it is a demonstrable defect
  (a case that gives a wrong or nondeterministic answer).

A finding goes in the **Rule** bin if it fails (b), or if reviewers disagree
on the fix and the disagreement is about values rather than facts. The queue
entry carries the strongest argument on each side and my recommendation.

Everything else goes in the **Reject** bin with a reason. A cold panel may
re-raise a rejected item; if it is re-raised by a different lens in a later
iteration, it is re-triaged rather than auto-rejected.

## Frozen pending ruling

These are OB-design or value questions. The loop may not decide them and the
stub keeps its current answer until a ruling lands:

1. Output object type: uniform `map[string]any` with opt-in order, uniform
   `*Object`, or the carried/constructed split with the switch documented.
2. `Select` by `...string` path or by compile-resolved `Field` handles.
3. `Evaluation` concurrency-safe or single-goroutine by rule.
4. Regex dialect: Go's `regexp` as a declared divergence, or the reference's
   dialect via a backtracking engine.
5. Whether to ship `Access` implementations (protobuf, reflection).
6. Every class-README change: the authority tiebreaker, agreement-or-refusal,
   the widened rule 8, the three-category ledger, the portable profile, the
   ordered-object claim.

Numeric specifics that follow from the standing ruling "Go's arithmetic,
refuse rather than approximate" are **not** frozen: literal typing, result
types, promotion at admission, `json.Number` classification, non-finite
handling, and the value function. They are applied, and flagged in
`CHANGES.md` so a veto is one line.

## Stopping conditions

The loop stops when **both** of the first two hold, or when either of the
last two does:

- **Converged on findings:** two consecutive panels produce no Apply-bin
  findings (only Rule-bin, Reject-bin, or previously applied items).
- **Converged on grades:** on every panel of those two, every reviewer grades
  every row the loop controls at B+ or better, and the median across reviewers
  on each of those rows is A- or better. The rows the loop controls are
  idiomatic fit, ergonomics of the common path, correctness and footguns, and
  performance headroom. Grades are a floor, never an average: an average hides
  a C.
- **Blocked:** findings have converged but grades have not, and the grades
  that fall short cite items in the ruling queue. The loop has done what it
  is allowed to do; the output is the ruling queue.
- **Capped:** six iterations, or roughly 3M tokens across panels.

The **concept soundness** row is recorded every iteration and is not a
stopping condition, because it is graded against the class README, which the
loop may not change. It is reported to the ruling queue as evidence.

## Panel composition

Four fixed lenses for comparability across iterations: Go idiom purist,
application integrator, PL and specification skeptic, JSONata practitioner.
One rotating lens for coverage. The panel that preceded the loop had a
performance engineer; the panels that review iteration 1 onward rotate:
security and sandbox reviewer (1), technical writer reading only `go doc`
output (2), the engineer who will write the JavaScript member from the same
README (3), performance engineer (4), then security again (5). Same rubric
every time.

## Records and commits

```
design/api-loop/
  LOOP.md                     this document
  RULINGS.md                  the accumulating ruling queue, with both sides
  iteration-1/
    CHANGES.md                applied, ruled, rejected, with reasons
    panel/                    five reports, verbatim
    grades.md                 the table
  iteration-2/ ...
```

Each iteration is one commit on the branch `design/api-loop` in this
repository: the stub changes, the records, nothing else, pushed to
`origin` after the commit (see Landing). The branch is the audit trail; a
wrong iteration is one revert.

## Cost

One panel is five agents at roughly 85K tokens each, about five minutes of
wall time. Six iterations is about 2.5M tokens. The first iteration also
includes moving the stub into `go/jsonata/` and applying the current Apply
bin, which is the largest change the loop will make.

## Iteration 1 triage, pre-populated from the 2026-09-18 panel

So the loop's judgment can be checked before it runs.

**Apply** (grounds in parentheses):

- Package at `go/jsonata/`; import no longer ends in the keyword `go`
  (compile-level; two reviewers).
- `# Numbers` section in the package doc: integral literals are `int64`,
  `uint64` when only that fits, `*big.Int` otherwise, all others `float64`;
  three result types; narrow integers and `float32` widen at admission;
  `json.Number` classifies once on first use, integral in range to `int64`,
  integral beyond to `*big.Int`, fractional or exponent to `float64`;
  non-finite inputs refused at admission; every observing operation is a
  function of the value, never the representation (five reviewers; follows
  from the standing ruling; flagged for veto).
- `*big.Int` or `int64` with a `float64` operand: refuse unless the integer
  is exactly representable as `float64` (resolves the rule 8 contradiction;
  three reviewers).
- `map[string]any` keys observed in sorted order; `*Object` in insertion
  order; stated (four reviewers; nondeterminism is a demonstrable defect).
- Stated finite defaults for every bound; `WithMaxWork` counting nodes
  evaluated and elements materialized; budgets accepted at `Compile` so they
  travel with the untrusted artifact; the sentence "no expression can
  terminate the process" (three reviewers, two would block on it).
- `Close()` with no error (three reviewers; `io.Closer` precedent is for
  resources that can fail).
- `Access.Number(v any) (any, error)`; `Keys` replaced by a `Range`
  callback (three reviewers).
- `Object`: `All() iter.Seq2[string, any]`, useful zero value,
  `UnmarshalJSON`, sized constructor (stdlib precedent: `maps.All`,
  `bytes.Buffer`).
- `String()` replacing `Source()` (`regexp.Regexp.String` precedent).
- `Plan.Reason` as typed constants; `Mode` constants prefixed so the
  constant `Complete` and the method `Complete` are distinct identifiers.
- `(*Expression).With(opts ...EvalOption) *Expression` so per-request
  options resolve once (two reviewers).
- `$now`/`$millis` snapshotted at `Prepare`; block preludes selectable with
  the prelude evaluated once; duplicate constructor keys an error under
  `Select`; rule 9 stated as "when `Complete` would succeed" (three
  reviewers; the current wording is vacuous when `Complete` fails).
- `text.Evaluate` returns `([]byte, bool, error)` and accepts a compiled
  `*Expression`; a `text.Decode` exposing the exact-number decoder (two
  reviewers).
- Exported `DocumentationCommit` constant.
- Context errors returned so `errors.Is(err, ctx.Err())` holds; the tuple's
  values on error stated as zero; ownership and aliasing sentences for
  returned values and for input mutation during evaluation; `WithBindings`
  captures by reference, stated.
- Overflow reported under the language's `D1001` rather than an engine code
  (practitioner; matches how existing handlers key; the idiom reviewer's
  separate-namespace preference yields to caller precedent).
- Regex: failures reported at `Compile`, `$match.index` counts characters
  consistent with `$substring` (two reviewers; dialect itself frozen).

**Rule:** the six frozen items above, each with both sides in `RULINGS.md`.

**Reject:** renaming `Prepare` (one reviewer, held loosely; `database/sql`
precedent is adequate); making `Object` an interface (no caller has asked);
a compile cache in the library (the performance engineer's own argument
against it is the stronger one).

## Landing

The loop exists to converge on one artifact and land it once. No work from
the loop may outlive it as a branch.

- **Integration ref:** `main`, as declared for this repository in
  `openbindings/project/repositories.json` (the same ref as `jsonata`,
  `openapi-client`, and `asyncapi-client`). The loop branches from `main`
  and lands on `main`.
- **One branch.** All iterations commit to `design/api-loop`. No per-iteration
  branches, no per-ruling branches, no experiment branches. A rejected
  direction is a revert on the same branch, recorded in that iteration's
  `CHANGES.md`.
- **Durability.** The branch is pushed to `origin` after every iteration's
  commit so that a local mishap cannot lose an iteration. A pushed design
  branch is not a release and creates no obligation; it is deleted on landing.
- **Rulings do not become branches.** A ruling still open when the loop
  stops is filed as a GitHub issue on this repository, referenced from
  `RULINGS.md`, and the stub keeps its current answer. Issues hold deferred
  decisions; branches never do.
- **Landing gate.** The loop has stopped (converged, blocked, or capped);
  `gofmt` and `go vet` are clean; every entry in `RULINGS.md` is either
  resolved in the stub or filed as an issue; the `design/` records for every
  iteration are committed.
- **Landing.** One pull request from `design/api-loop` to `main`, squash-merged
  with the branch deleted, per the project's PR workflow. The squash commit
  message summarizes the iterations and links the issues. After landing, the
  local branch is deleted and `main` is fast-forwarded, so the checkout has
  exactly one branch again.
- **Catalog.** The repository is declared in the project catalog before the
  loop lands, so the landing has a declared destination. Cohort tier stays
  `extended` until a member ships.
