# Grades from the iteration-2 panel

Five cold reads of the stub at `4cb2f5d`. Rotating lens: technical writer
(judging `go doc -all` output). Rows the loop controls are the first four.

| | idiom | integrator | writer | pl | practitioner | median |
| --- | --- | --- | --- | --- | --- | --- |
| Idiomatic fit for Go | B | B+ | A- | B+ | A- | B+ |
| Ergonomics, common path | B- | B- | B- | B | B | B- |
| Correctness and footguns | B- | C+ | C+ | C+ | C+ | C+ |
| Performance headroom | A- | A- | A- | A- | A- | A- |
| Concept soundness | B+ | B+ | A- | C | B | B+ |
| Overall | B+ | B | B | B- | B | B |

Movement (medians, iteration 1 → 2): Go fit B+ → B+, ergonomics B → B-,
correctness C+ → C+, performance A- → A-, concept B → B+, overall B → B.

Floor on loop-controlled rows: C+ (correctness, four reviewers). Grade gate
not met. Apply-bin findings: many, a majority of them defects introduced by
iteration 2's own new text (the float-mixing rule, the `uint` rule, the
`-0` attribution, `ReasonArrayInput`, `Class()`), plus doc structure. Loop
continues to iteration 3.

Ergonomics dipped because the writer counted seven guessed lines in a
twenty-line first program; iteration 3 adds the first-call block and
examples that answer them.
