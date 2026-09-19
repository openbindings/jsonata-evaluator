# Grades from the iteration-4 panel

Five cold reads of the stub at `e57816b`. Rotating lens: performance
engineer. Rows the loop controls are the first four.

| | idiom | integrator | perf | pl | practitioner | median |
| --- | --- | --- | --- | --- | --- | --- |
| Idiomatic fit for Go | B+ | A- | B+ | B+ | B+ | B+ |
| Ergonomics, common path | B | B | A- | B | B | B |
| Correctness and footguns | B- | B- | B | B- | B- | B- |
| Performance headroom | A- | A- | B+ | B+ | A- | A- |
| Concept soundness | B+ | B+ | A- | C+ | B- | B+ |
| Overall | B+ | B+ | B+ | B- | B | B+ |

Movement (medians, iteration 3 → 4): Go fit B+ → B+, ergonomics B → B,
correctness B- → B-, performance A- → A-, concept B → B+, overall B → B+.

Floor on loop-controlled rows: B- (correctness, four reviewers). Grade gate
not met (the gate is a B+ floor and an A- median). Apply-bin findings:
present. Loop continues to iteration 5.

Correctness held at B- with a new defect class: this round's findings are
mostly consequences of iteration 4's own text (the 2^53 integrality cap
breaks substitutivity; the `/` worked example renders wrong; `Unmarshal`'s
doc contradicted the classification rule on `1.0`; "first read" promised a
cache a `[]byte` cannot hold) plus the standing footgun list every
integrator writes down. Overall rose because four of five reviewers now
grade the API B+ or better; the PL skeptic's B- is the concept row
leaking into the API rows, and that row is ruling-queue evidence.
