# Grades from the iteration-5 panel

Five cold reads of the stub at `7f8b4fe`. Rotating lens: security and
sandbox reviewer. Rows the loop controls are the first four.

| | idiom | integrator | security | pl | practitioner | median |
| --- | --- | --- | --- | --- | --- | --- |
| Idiomatic fit for Go | B | B+ | A- | B+ | A- | B+ |
| Ergonomics, common path | B- | B- | B+ | B | B+ | B |
| Correctness and footguns | B- | B- | B- | B- | B- | B- |
| Performance headroom | A- | A- | A- | A- | A | A- |
| Concept soundness | B- | B+ | B+ | C+ | B | B |
| Overall | B | B | B+ | B- | B+ | B |

Movement (medians, iteration 4 → 5): Go fit B+ → B+, ergonomics B → B,
correctness B- → B-, performance A- → A-, concept B+ → B, overall B+ → B.

Floor on loop-controlled rows: B- (correctness, all five reviewers). Grade
gate not met. Apply-bin findings: present. Iteration 6 is the loop's cap.

Correctness is B- from every seat for the fourth panel running, and the
reasons have stopped moving: the map-versus-`*Object` split (ruling 1), the
regex dialect (ruling 4), the refusal-versus-rounding question (ruling 8b),
and the typed exit (ruling 9) account for most of every reviewer's
deductions. The security lens found the loop's last structural class of
defect: the work budget charged one unit for operations whose cost scales
with an uncharged value, value walks were not depth-bounded, integer tokens
were unbounded before parsing, and `Marshal` had no bound at all. Those are
applied in iteration 6.
