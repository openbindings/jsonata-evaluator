# Grades from the iteration-1 panel

Five cold reads of the stub at `feb3743`. Rotating lens: security and sandbox
(replacing performance). Rows the loop controls are the first four.

| | idiom | integrator | security | pl | practitioner | median |
| --- | --- | --- | --- | --- | --- | --- |
| Idiomatic fit for Go | B+ | B+ | B | A- | A- | B+ |
| Ergonomics, common path | A- | B- | B | B | B+ | B |
| Correctness and footguns | B | C+ | C | C+ | B- | C+ |
| Performance headroom | A- | A- | B+ | A- | A- | A- |
| Concept soundness | B | B+ | B- | C | B | B |
| Overall | B+ | B | B- | B- | B+ | B |

Movement from the pre-panel stub (medians): Go fit B+ → B+, ergonomics
B → B, correctness C → C+, performance B+ → A-, concept B → B, overall
B- → B.

Floor on loop-controlled rows: C (security, correctness). Grade gate not
met. Apply-bin findings: many. Loop continues to iteration 2.

The dominant correctness findings this round are new, not carried over:
the bounds as written do not bound bytes (security), `Access` cannot report
failure so guards fail open (security, pl, idiom), memoized values are
described as both shared and caller-owned (four reviewers), and nil slices
as null flips `$count`-shaped predicates (security, integrator).
