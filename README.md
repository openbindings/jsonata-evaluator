# JSONata evaluators

Implementations of the JSONata language, native to each host, over the
host's own values. This repository is the OpenBindings evaluator class: the
definition of what such an implementation is, the shared conformance suite
every member runs, and the members the project maintains.

## The idea

A regular expression engine implements a notation natively over the host's
strings. Nobody ports PCRE to Go; Go writes `regexp`. What a character is, how
case folds, how matching is performed: those are the host's. What `.` and `*`
and a capture group mean: those are the notation's. Every host has its own
engine, each declares what it does not support, and the notation is shared.

A JSONata evaluator is the same kind of thing for JSON values. The JSONata
documentation defines what an expression does. The host defines what the
values are and how primitives combine. An evaluator is where the two meet,
and it is written for its host the way a regex engine is, not ported from
anyone else's program.

This is not how JSONata has been implemented elsewhere. Every existing port
takes the reference implementation as the authority and reproduces it,
including its value model and its departures from its own documentation.
This class takes the documentation as the authority and the host as the
value model. The difference is observable in results.

## The split

- **The documentation governs what is done.** Path navigation, sequence
  flattening and singleton unwrapping, object construction and grouping,
  predicates, control flow, undefined versus null, the closed environment,
  and the meaning of every standard-library function. These are owed in full.
- **The value model governs what values are and how primitives combine.**
  What a number is, how two of them add, how long a string is, what bytes are,
  how values compare and order. For a host-native member these are the host's,
  at the fidelity the host provides.

Where the two meet, the documentation wins where it speaks and the value
model wins where the documentation is silent. `1/2` is `0.5` because the
documentation says so. `0.1 + 0.2` is whatever the host's floating-point
addition yields, because the documentation never says what a number is.

The authority is the documentation pinned by the OpenBindings specification
(currently JSONata 2.1). Where the reference implementation's behavior differs
from that documentation, the documentation defines the language.

## Membership

A library is a member of this class when all of the following hold. Nothing
else is required.

1. **It interprets the pinned documentation**, verified against the reference
   test suite for that version, run through its public API.
2. **It evaluates over a stated value model.** The project's members are
   host-native: the host's ordinary values, by reference, with no
   serialization to JSON text or to an intermediate tree. A member with a
   different model states it rather than inheriting one by accident.
3. **The value model defines values and primitive operations.** A host-native
   member owes no more than its host offers and must not offer less.
4. **Carriage is exact.** A value an expression only selects, copies, or
   rearranges arrives unchanged: same numeric value, same string content, same
   bytes.
5. **Comparison is by value.** Equality, ordering, membership, sort keys, and
   `$type` are decided by mathematical or textual value across every
   representation the model admits, never by host type.
6. **Divergences are declared, never silent.** A departure from the reference
   suite is permitted only where the value model cannot represent a reference
   value, and each is recorded with its reason and pinned to the fixture it
   departs from, so a fixture change invalidates the declaration.
7. **The environment is closed.** No expression can reach host state, and the
   public surface offers no way to extend the language with host functions.
8. **It refuses rather than approximates.** Overflow, non-finite results,
   exhausted budgets, and unrepresentable values are failures, never silently
   rounded, saturated, or coerced.
9. **Selective evaluation, if offered, is unobservable.** A field obtained by
   evaluating only part of an expression is identical to the same field of a
   complete evaluation.

## What membership does not require

- **No arithmetic model beyond the stated one.** Whether integers are exact to
  64 bits or arbitrary precision, whether decimals are binary or exact, is the
  value model's business.
- **No parity between members.** Two members agree wherever their value
  models agree. Where the models differ, the members differ, and that is
  correct. The project's own members aligning with each other is a quality
  commitment of the project, not a property of the class.
- **No selective evaluation.** It is a performance feature the project's
  members provide, constrained only by rule 9.
- **No particular API.** Each member exposes what is idiomatic for its host.

## The portable core

An observation, not a rule. Transforms written for OpenBindings interfaces
have so far used a small subset of the language: object and array literals,
variable bindings, conditionals, comparisons, `$lookup`, `$exists`, `$keys`,
`$count`, `$merge`, `$sift`, `$string`, `$type`, and the root reference `$$`.
That subset is exactly the part of the language where the documentation and
the reference agree without exception. Authors who stay inside it get identical
results from every member. Authors who go outside it get documented behavior,
which may include a declared divergence.

## Relationship to OpenBindings

OpenBindings Core pins the transform language by documentation and binds an
evaluating tool only to that language's contract. It requires nothing about
values, runtimes, or how the language is implemented, and it privileges no
implementation. This class is one implementation stance, the one the
project's own evaluators take. An implementation with a different stance can
be fully conformant to Core; it is simply not a member here.

Members sit behind the OpenBindings SDKs' `TransformEvaluator` seam. What
reaches an evaluator has already crossed a binding: the governing binding
specification has decoded the wire into the operation value domain and, for
bytes, has chosen the boundary encoding. A member carries bytes as the host's
byte type, presents them as the binding's boundary string only when an
expression uses them as a string, and never chooses the encoding itself.

## Layout

```
README.md    this definition
suite/       the reference fixtures at their pinned upstream commit, the
             declared-divergence format, and a language-neutral runner
             description
go/          the Go member
```

Each member directory owns its README, its release process, and its ledger of
declared divergences, and versions independently. Go members tag by
subdirectory (`go/vX.Y.Z`).

## Members

| Host | Directory | Status |
| --- | --- | --- |
| Go | `go/` | design |

## License

Apache 2.0. See [LICENSE](LICENSE).
