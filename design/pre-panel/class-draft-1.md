# JSONata evaluators

> **Draft for review, 2026-09-18.** Project guidance, not specification text.
> OpenBindings Core pins the transform language ([§5.5](../../spec/openbindings.md))
> and binds an evaluating tool only to that language's contract
> ([OBI-T-10](../../spec/openbindings.md)). Everything below is one
> implementation stance, the one the project's own evaluators take. An
> implementation that takes a different stance can be fully conformant.

## 1. What this defines

A **JSONata evaluator**, in this document's sense, is an implementation of
the JSONata language as the pinned documentation defines it, over a value
model the implementation states. It exists because Core pins the transform
language by *documentation* and says nothing about values or runtimes: the
specification pins the notation, each implementation supplies the values,
and this class of library is where the two meet.

The project's own evaluators are **host-native**: their stated value model is
the host language's own values, by reference. That is a choice the project
makes for its members, not a condition of the class.

The defining split:

- **JSONata's documentation governs what is done.** Path navigation, sequence
  flattening and singleton unwrapping, object construction and grouping,
  predicates, control flow, undefined versus null, the closed environment,
  and the meaning of every standard-library function. These are owed in
  full and are not the implementation's to reinterpret.
- **The stated value model governs what values are and how primitives
  combine.** What a number is, how two of them add, how long a string is,
  what bytes are, how values compare and order. For a host-native member
  these are the host's, never JavaScript's and never an emulation of any
  other host.

Where the two meet, the documentation wins where it speaks and the value
model wins where it is silent. `1/2` is `0.5` because the documentation says
so; `0.1 + 0.2` is whatever the stated model's floating-point addition yields
because the documentation never says what a number is.

## 2. Why this is the stance

Every JSONata port surveyed takes the opposite stance: the reference
implementation is the authority, the port emulates it, and differences are
defects to minimize. That stance imports JavaScript's value model into hosts
that have better ones, and it cannot be maintained: every such port carries a
divergence ledger anyway, and the reference itself fails its own
documentation in places (the transform operator's copy path rounds through
string formatting; corpus case LANG-22).

The stance here is the one CEL and JMESPath take for their languages:
specification-defined operations over host-native values, one implementation
per host, a shared conformance suite, host limits declared rather than hidden.
JSONata lacks a formal specification, so this document also has to name the
authority: **the pinned documentation, and where the reference implementation
disagrees with it, the reference is wrong.**

## 3. Membership floor

A library is a member of this class when all of the following hold. Nothing
else is required; everything else is the implementation's business.

1. **Interprets the pinned documentation.** Structure, control flow, sequence
   rules, and every standard-library function per the JSONata version Core
   pins, verified against the reference test suite for that version.
2. **Over a stated value model.** The implementation says what its values
   are and evaluates over them directly. The project's members evaluate over
   the host's native values, by reference, with no serialization to JSON text
   or to an intermediate tree; a member with a different model states that
   model instead of inheriting one by accident.
3. **The stated value model defines values and primitive operations.**
   Numeric representation, arithmetic, string measurement, byte handling, and
   ordering are the model's own. For a host-native member that is the host's
   environment, at the fidelity it provides, never more and never less.
4. **Exact carriage.** A value that an expression only selects, copies, or
   rearranges arrives unchanged: identical numeric value, identical string
   content, identical bytes.
5. **Comparison by value.** Equality, ordering, membership, sort keys, and
   `$type` are decided by mathematical or textual value across every numeric
   and string representation the host admits, never by host type.
6. **Divergences are declared, never silent.** A departure from the reference
   suite is permitted only where the host cannot represent a reference
   value, and each one is recorded with its reason and pinned to the fixture
   it departs from, so a fixture change invalidates the declaration.
7. **Closed environment.** No document-supplied expression can reach host
   state, and the public surface offers no way to extend the language with
   host functions. (§5.5 clause 5 makes this a Core requirement for any
   evaluating tool; it is restated here because the class exists to satisfy
   it by construction rather than by configuration.)
8. **Refuse rather than approximate.** Integer overflow, non-finite results,
   exhausted work budgets, and unrepresentable values are failures, never
   silently rounded, saturated, or coerced.
9. **Selective evaluation, if offered, is unobservable.** A field obtained by
   selective evaluation is identical to the same field of a complete
   evaluation of the same expression over the same input.

## 4. What the floor deliberately does not say

- **No arithmetic model beyond the stated one.** Whether integers are exact
  to 64 bits or arbitrary precision, whether decimals are binary or exact, is
  the value model's business. A host-native member does not owe more than its
  host offers and must not offer less.
- **No parity between members.** Two members agree wherever their value
  models agree. Where the models differ, the members differ, and that is
  correct. The project's own members aligning with each other is a quality
  commitment of the project, not a property of the class.
- **No selective evaluation requirement.** It is a performance feature, valued
  and expected in the project's members, and constrained only by rule 9.
- **No mandated public API.** Each host's member exposes what is idiomatic
  for that host.

## 5. The portable core

An informative observation, not a rule. The project's own generated
transforms use a small subset of the language: object and array literals,
variable bindings, conditionals, comparisons, `$lookup`, `$exists`, `$keys`,
`$count`, `$merge`, `$sift`, `$string`, `$type`, and the root reference `$$`.
They avoid implicit path mapping, sequence flattening, sorting, grouping,
wildcards, and the date and regex libraries.

That subset is exactly the part of the language where the documentation and
the reference agree without exception. Authors and generators who stay
inside it get identical results from every member of this class. Authors who
go outside it get documented behavior, which may include a declared host
divergence. Tools may report which of the two an expression is in; nothing
forbids the second.

## 6. Where values enter and leave

A member sits behind the SDKs' `TransformEvaluator` seam. What reaches it has
already crossed a binding: the governing binding specification has decoded
the wire into the operation value domain and, for bytes, has chosen the
boundary encoding per the bytes-boundary rules in the
[binding-specs catalog](../../spec/binding-specs/README.md) (follow the
artifact's declared encoding; Base64 in the gap). A member therefore carries
bytes as the host's byte type, presents them as the binding's boundary string
only when an expression treats them as a string, and never chooses the
encoding itself.

## 7. Non-goals

- Defining JSONata. The documentation does; this document names it as the
  authority and nothing more.
- Serializing results. A member returns host values; encoding them is the
  caller's.
- A second numerical policy. See §4.
- A registry of members. Anyone may implement this for any host under their
  own authority; the project's members are simply the ones the project
  maintains.
