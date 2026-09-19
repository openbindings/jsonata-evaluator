# Iteration-6 panel: technical writer

Cold read of `81e3693`. Rotating lens: technical writer judging the rendered documentation.

## Grades

| Criterion | Grade | One-line justification |
|---|---|---|
| Idiomatic fit for Go | B | Limits-as-receiver, `(value, present, err)`, `MustCompile`, `errors.Is` sentinels all read as Go; but the doc explains them by analogy (`net.Dialer`, `regexp`) more than by statement, and the overview is a 375-line essay where Go readers expect a page. |
| Ergonomics of the common path | B- | `EvalJSON` is the one-call path for exactly the caller this library exists for, and it has no example, no mention in the quick start, and sits sixth among `Expression`'s methods. |
| Correctness and footgun risk | C+ | Three footguns the text creates but never warns about at the call site: `Eval(ctx, jsonBytes, nil)` compiles and treats the payload as a string; the quick start's `jsonata.Marshal` runs under default bounds, not the expression's; and the ledger says two "settled" behaviors in the overview are pending. |
| Performance headroom the API permits | B+ | Carriage by identity, views, selective evaluation, and per-byte work accounting are all documented well enough that a reader can predict cost; the doc undersells that `Reads` and `Fields` exist for pre-fetch planning. |
| Concept soundness (JSONata-as-regex, documentation-as-authority, host-native values) | B | The regex analogy teaches in two sentences and then the value-model text spends 150 lines discovering that "documentation is silent about numbers" needs an arithmetic policy the documentation never gave you; the README's membership rule 6 forbids most of the ledger's own entries. |
| Overall | B- | The material for an A is here; it is in the wrong order, defines its terms after using them, states a dozen promises twice, and the examples do not run. |

## The first program

The caller I modeled is the stated one: decoded API payload in, transform out, one injected binding, large IDs exact.

```go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/openbindings/jsonata-evaluator/go/jsonata"
)

var expr = jsonata.MustCompile(`{ "id": user_id, "name": display_name, "tenant": $tenant }`)

func main() {
	body, _ := os.ReadFile(os.Args[1])
	env := &jsonata.Env{Bindings: map[string]any{"tenant": "acme"}}
	out, present, err := expr.EvalJSON(context.Background(), body, env)
	if err != nil {
		fmt.Fprintln(os.Stderr, err) // GUESS: what Error() prints; "not stable across versions" is the only description
		os.Exit(1)
	}
	if !present { // GUESS: can an object constructor ever yield absent? Nothing says; Select's doc hints a field can be absent, the constructor's fate is unstated
		return
	}
	os.Stdout.Write(out) // GUESS: trailing newline? Inferred across two "as X would" hops (EvalJSON -> EvalMarshal -> Marshal)
	fmt.Println()        // GUESS: is display_name omitted or present-as-null when the input lacks it? JSONata docs say omitted; this doc never says
	v, _, _ := expr.Eval(context.Background(), body, env) // GUESS: compiles, vets, and is wrong; body is []byte so $ is a byte *string*. No call-site warning
	id, _ := jsonata.Member(v, "id")
	n, ok := jsonata.Int64(id)
	fmt.Println(n, ok) // GUESS: what fmt prints for a *Object if I print v; Object has no String and the doc does not say
}
```

Six guesses in twenty lines. Four are about the shape of output text and absence, which is the kind of thing a first-hour user needs answered by an example with an `// Output:` line. One (the `[]byte` input) is a real bug a reader will ship.

## Reading order and structure

How it reads now. The first paragraph is doing four jobs: purpose, authority, arithmetic model, and a panic/fatal-error safety contract with a subordinate clause about concurrent map writes. A reader arriving to evaluate a transform is handed "goroutine stack overflow" before they are handed "here is how you call it." Then a status line, a quick start, a one-sentence aside on Limits phrased as a `net.Dialer` analogy, and then thirteen `#` sections in this order: values in, values out, errors, numbers, strings, objects, ownership, selective, bounds, environment, conformance. Numbers alone is ninety rendered lines and comes before the reader knows what `Env` is.

The order is roughly "what you must know to use it" for the first three sections and then "the value model, exhaustively" for the next three, then back to caller concerns. The value model reference is the longest block and the least actionable for the first hour. It should be last, and it should be shorter.

What I would ship, as section list:

1. **Opening paragraph** (four sentences). Evaluates JSONata 2.1 over Go values. The documentation at `DocumentationCommit` defines the language; Go's own types define the values, as `regexp` does for strings. Integers are exact, decimals are `float64`, and nothing is rounded silently. Every evaluation is bounded by `Limits` and cannot panic the caller.
2. **Status.** Keep as is.
3. **Quick start.** Lead with the one-call form: `expr.EvalJSON(ctx, body, env)`. Then the decomposed form (`Unmarshal`, `Eval`, `Marshal`) with the note that the decomposed `Marshal` must be `expr.Limits().Marshal` if the bounds matter. Put the three-outcome switch here, once.
4. **Results: present, absent, null.** The current first paragraph of "Getting values out." Five lines.
5. **Inputs.** The admitted list, as a list, not a sentence. The `[]byte`-is-a-string rule with the explicit warning that JSON text is not an input. `NoInput`. Foreign values in two sentences with a pointer to `Resolver`.
6. **Vocabulary.** Six terms defined once: admitted, foreign, view, carried, observed, order-observing. Every later section uses them and never re-explains them. (Today "carried" is used in "Getting values in" and defined in "Getting values out"; "foreign" is used two sentences before "Anything else is foreign"; "view" is used a dozen times before `ObjectView` appears.)
7. **Errors.** Current section, plus the `[]byte`-with-no-`BytesEncoding` case since it is the one a newcomer hits.
8. **Limits and cancellation.** Merge "Bounds and cancellation" into a short section; move the per-field detail to the `Limits` struct where it already lives.
9. **Env.** Bindings, clock, bytes, resolver. Merge the current "Environment" section in; drop the second statement of "no way to register functions" (it is on `Env.Bindings` too).
10. **Selective evaluation.** Half the current length; the prelude, guard, and plan rules belong on `Evaluation` and `Select`, where a reader who chose selection will look.
11. **Ownership and concurrency.** Current section, minus the sentences already on `Prepare` and `Close`.
12. **Value model: numbers.** Cut to the rules and the sharp edges: integer domain exact, decimal domain float64, mixed operands must convert exactly or fail, division yields a float rounded once, `$string` renders as JSON.stringify. Move every "the reference implementation does X instead" sentence to the ledger and link it.
13. **Value model: strings, bytes, regular expressions.** Same treatment.
14. **Value model: objects and member order.** Same.
15. **Conformance and divergences.** Keep.

Cut outright: the `net.Dial`/`net.Dialer.DialContext` sentence (the `Limits` doc already enumerates the pairs); "The design is written up at <repo root>" (it points at nothing specific); "A view that is pointer-shaped converts to an interface without allocating" (allocation trivia on an interface's doc); "Under concurrent selections the budget may be exhausted at a different point" (true, unactionable).

## Contradictions, duplications, and undefined terms

**Contradiction: settled in the overview, pending in the ledger.** Overview, Numbers: "Two policies govern the decimal domain, and they are two rather than one. An integer operand combined with a decimal operand must convert to float64 exactly, and is an error (CodeInexact) otherwise". Ledger, row "Mixed integer and decimal arithmetic beyond 2^53": "whether this refusal or a single rounding is the rule is pending". A caller reading the overview will write code against `ErrInexact`. Fix: either the ledger drops "pending" or the overview says, as it does for `$round`, that this is the current answer and not settled.

**Contradiction: regex dialect.** Overview: "Go's ${name} form is not recognized. Regex literals are compiled by Compile, so a malformed literal fails there". That is only true if Go's `regexp` is the engine. Ledger: "Regular-expression dialect | JavaScript | pending ruling". A transform author cannot know whether lookbehind, backreferences, or `\d` semantics work. Fix: the overview must say which syntax `/.../` accepts today and that it is current, not settled, or say nothing about `${name}` until it is settled.

**Contradiction: README rule 6 versus the ledger's taxonomy.** README: "A departure from the reference suite is permitted only where the value model cannot represent a reference value". Ledger authority column admits **documentation**, **interpretation**, and **engine** departures, none of which are "the value model cannot represent". `$split` on code points, `$round` midpoints, the range cap, and duplicate-key refusal all violate rule 6 as written. Fix rule 6: "A departure from the reference suite is permitted only where the documentation, an authority it incorporates, or the stated value model requires it, or where the member refuses rather than approximates; each is recorded with its authority and pinned to the fixture."

**Contradiction: the portable core includes `$string`.** README: the subset "is exactly the part of the language where the documentation and the reference agree without exception", and `$string` is in the list. The ledger's first two rows are `$string` divergences. Fix: qualify ("`$string` of strings, integers, and objects") or drop the "without exception" claim.

**Contradiction: unknown-position sentinels.** `Error.Offset`: "-1 when unknown". Same comment, next sentence: "Line and Column are 1-based, Column in bytes, or 0." Two sentinels for one condition. Fix: pick 0 for all three (matching the "or 0" wording) or -1 for all three and say why.

**Overclaim: "Every package-level function runs under DefaultLimits".** `Member`, `Int64`, `Float64`, `NewObject`, `DefaultLimits` do not. The `Limits` doc lists the eight that do. Fix: "Every package-level function that has a `Limits` method of the same name runs under DefaultLimits."

**Term collision: "class".** `Code` doc: "Class() == ClassEngine identifies a refusal another member of the class might not make." In this file `Class` is a Go type for error classes; "the class" here means the evaluator class from the README, which the Go reader has never heard of. Fix: "identifies a refusal that is this package's, not the language's; another JSONata evaluator may not make it."

**Term collision: "binding".** Overview, bytes: "because the class requires that the binding, not the evaluator, choose the encoding." `Env.Bindings` is a map of variables; this "binding" is an OpenBindings binding specification. Fix: "because the caller that decoded the payload, not this package, knows what the bytes were on the wire."

**Duplicated promises** (each stated twice or three times in different wording):
- Caller-code panics propagate: intro paragraph and "Bounds and cancellation." Keep the second.
- Input and Env must not be modified during evaluation: "Ownership," `Prepare`, `Close`. Keep it on `Prepare` and `Close` and one line in "Ownership."
- Cache on `Expression.Limits` not on the receiver: `Limits` doc and `Limits.Compile` doc. Keep the `Compile` one.
- Select is not a guard / sibling failure not observed under a selective plan: "Selective evaluation" and `Select`. Keep it on `Select`.
- Results may alias input and bindings: "Getting values out," "Ownership," `Env.Bindings`, `Close`. Keep "Ownership" and `Close`.
- `MaxWork` charges carried and caller-supplied values: overview "Bounds" and `Limits.MaxWork`. Keep the field.
- No way to register functions: "Environment" and `Env.Bindings`. Keep the field.
- A `json.Number` is classified on each read and charged per byte: "Getting values in" (for RawMessage), Numbers, `Limits.MaxWork`.

**Sentences doing two jobs** (split each):
- "A nil []T is the empty array, a nil map[string]T and a nil *Object are the empty object, because Go code routinely leaves a collection unset and $count(items) = 0 should hold for it rather than items being null; every other nil pointer, including a nil *big.Int, is null." Rule, rationale, and a second rule in one sentence.
- The `json.RawMessage` sentence in "Getting values in" has five clauses and two parentheticals; it is the rule, the cost, a consequence, the workaround, and an error case. Make it four sentences.
- "An integer operand combined with a decimal operand must convert to float64 exactly, and is an error (CodeInexact) otherwise, so 9007199254740993 + 0.5 fails while 0.1 + 0.2 yields the float64 sum, and x / 1e9 succeeds where x * 1e-9 fails for an x beyond 2^53 (1e9 is integral; 1e-9 is a decimal): the refusal is where an identifier would lose digits on entering the decimal domain." Rule, three examples, rationale.

**Undefined at first use:** carried/carriage, foreign, admitted, view, observe/observed, order-observing operation, prelude, plan, selective, "the transform operator" (JSONata `~> | |`; say so), "the reference implementation" (jsonata-js; name it once), "boundary encoding" (README only), "engine" (used as "this package" and as "the E-code source").

**Stated but not actionable:** "$power with a non-integer exponent is math.Pow and may differ from another host's in the last unit"; "Which value a tie is judged on is not settled"; "Under concurrent selections the budget may be exhausted at a different point"; "as net.Dial and net.Dialer.DialContext relate."

## Symbol docs that do not stand alone

A reader lands on these from a search or from the pkg.go.dev index.

- **`Expression.Eval`**: uses "carried" and "admitted set" undefined. Add: "input is a decoded Go value (see Unmarshal); a []byte input is a byte string, not JSON text. For JSON text use EvalJSON."
- **`Limits.Eval`**: same `[]byte` sentence.
- **`EvalJSON`**: "is EvalMarshal over Unmarshal(input)" is a definition by two other symbols. Add the one sentence a first-hour reader wants: "It decodes input as JSON, evaluates, and returns the result as JSON text with no trailing newline; present is false for an absent result."
- **`Prepare`**: "completes the plan" and "prelude" are undefined here. Add: "Selection evaluates only the field asked for when the expression is an object constructor with literal keys (Plan reports whether it can); a block's leading statements run once, on the first Select."
- **`Evaluation.Select`**: "constructor granularity", "the plan permits" undefined. Add a two-line example path: `Select(ctx, "user", "id")`.
- **`Materialize`**: "foreign value", "admitted values" undefined. Add: "A foreign value is one whose Go type this package does not read directly (see Resolver)."
- **`Marshal`**: uses "foreign" and refers to "env's BytesEncoding" before a reader knows what that is. Add: "A []byte value needs env.BytesEncoding to render; without one it is an error."
- **`Member`**: fine alone. **`Int64`/`Float64`**: "by the rule arithmetic applies to operands" is a reference to the overview. Add: "an integer converts exactly when its magnitude is at most 2^53 or it is otherwise exactly representable."
- **`Code`**: "another member of the class" (see term collision above).
- **`Class` constants**: `ClassEngine` is fine; `ClassSyntax` "Only $eval can raise one after a successful Compile" is good.
- **`Reason` constants**: `ReasonPlanBudget` "the planner's work bound" is nowhere defined; there is no `Limits` field for it. Say which bound, or say it is internal and fixed.
- **`Plan`**: "how selections will be served" says nothing. Add: "Selective means a Select evaluates only that field; otherwise every Select evaluates the whole expression once and looks the field up."
- **`NoInput`**: good.
- **`Object`**: good; the best doc in the file.
- **`Resolver`**: long but stands alone; move the "pointer-shaped view" allocation note out.
- **`BytesEncoding`**: add why it exists: "The evaluator does not choose how bytes read as text; the caller that decoded them does."
- **`Env`**: "the value-model side of an evaluation" is jargon. Add: "Env carries what an expression can see besides its input."
- **`Version`/`DocumentationCommit`**: fine.

## The ledger as documentation

Not yet readable by a transform author, and that is the reader who needs it most. Specific problems:

1. **The intro is one 170-word paragraph** that defines five authority labels before a reader knows why they would care about authority at all. A transform author's question is "if I write X, what do I get here and what do I get in jsonata-js?" The authority column answers a different question ("who decided") that matters to the project's reviewers.
2. **"the stub's current text"** is unreadable outside this session. A transform author does not know there is a stub.
3. **Go identifiers in the wording**: `CodeInexact`, `*Object`, `Unmarshal`, `MaxIntegerBits`. A transform author sees error codes and JSON, not Go types. Keep the code (E1008) first and the Go name in parentheses or in a footnote.
4. **Pending rows are buried among settled ones**, in a table that reads as settled. The three pending questions (round midpoint, mixed arithmetic, regex dialect) are exactly the ones an author would most want to avoid depending on.
5. **Rows mix language behavior with decoder behavior**: "Duplicate member names in decoded input" and "Ill-formed strings" are about `Unmarshal`, which a transform author never calls. Put them in a separate "Input decoding" group.
6. **The authority rationale in row 2 contradicts the README's split** ("documentation wins where it speaks"): row 1 says JSON.stringify is incorporated documentation; row 2 says the value model overrides it. Both may be right, but the rule that decides between them is nowhere stated, so the two rows together teach an author that the authority column is unpredictable.
7. **No stable anchor per row.** An error message, a fixture, or a code comment cannot cite a row. Give each a short heading that greps.

Shape I would ship:

- A four-line intro: what a divergence is, that each is pinned to a fixture, and that pending entries are marked and may change.
- **Open questions** first, three entries, each with: the expression, the two answers, which one this package gives today, and what an author should do to stay portable (e.g., "do not depend on the rounding of a tie; use `$formatNumber` when the spelling matters").
- Then settled entries grouped by area: **Numbers**, **Strings and characters**, **Objects and member order**, **Engine refusals**, **Input decoding**. Each entry: `### <expression or behavior>` as a heading, then three short lines: *jsonata-js*, *this package*, *why* (one sentence, plain language: "the documentation says characters, and a character is a code point here"). The authority label goes at the end of *why* in parentheses for the reviewers who want it.
- Move "Entries will be pinned to the fixtures ... once the shared suite lands" to the top as an honest status line, not a mid-paragraph aside.

## What I would do with one day

1. **Make the examples run.** Add `// Output:` to every example so `go test` verifies them and pkg.go.dev renders the result text. Add `ExampleExpression_EvalJSON` (the common path), `ExampleEnv` (a binding read by `$tenant`), and `ExampleMember` (reading `id` out of a `*Object` with `Int64`). Delete `ExampleUnmarshal` as it stands (`_ = in` shows nothing) or fold it into the EvalJSON example. This is the highest-value hour: it turns four of my six guesses into facts.
2. **Reorder the overview** to the fifteen-section list above and cut it by about forty percent: value model last, vocabulary section added, every "the reference implementation does X" sentence moved to the ledger with a link.
3. **Fix the two settled/pending contradictions** (CodeInexact, regex dialect) in whichever direction the maintainers choose, and rewrite README rule 6 so the ledger's own entries are permitted by it.
4. **Add the `[]byte`-input warning** to `Expression.Eval` and `Limits.Eval`, and change the quick start's `jsonata.Marshal(out, nil)` to `expr.Limits().Marshal(out, nil)` or to `EvalMarshal`, with one sentence saying why.
5. **De-duplicate** the eight twice-stated promises, keeping each on the symbol where a reader would look.
6. **Reshape DIVERGENCES.md** as described: open questions first, grouped, plain-language why, Go names demoted.
7. **Fix the small things**: Offset/Line/Column sentinel, the "every package-level function" overclaim, the two "class"/"binding" collisions, `ReasonPlanBudget`'s undefined bound.

## On the concept

The regex analogy is the best two sentences in the repository and it teaches. "Nobody ports PCRE to Go; Go writes `regexp`" gives a reader the whole stance in one image, and the split that follows (documentation governs what is done, the host governs what values are) is crisp enough to test any single behavior against. As a teaching device it works. Where it strains is the moment the prose admits that the documentation is silent about what a number is, and then spends the longest section of the package doc constructing an arithmetic policy (two domains, exact-or-refuse, division rounds once, integral floats are integers) that the documentation never gave it and Go's own types do not give it either. Go has no "integer that is a float64 when integral" type; that is a design the package made, and the regex analogy does not cover it. The prose should say so plainly: "the host supplies the primitives; this package supplies the rule for combining them, and here it is." Instead it presents the policy as if it fell out of the value model, and the ledger's authority column then has to invent "interpretation" and "engine" categories to account for the places where it did not.

The overclaims are in the README, not the Go doc. "Every existing port ... reproduces its departures from its own documentation" and "The difference is observable in results" are stances a reader would like a single example for on the first page; the doc has a dozen further down. "That subset is exactly the part of the language where the documentation and the reference agree without exception" is false by the ledger's own first row. Rule 6 promises a narrower ledger than the one shipped. The Go doc, by contrast, underclaims in one place that matters: it never says out loud that `EvalJSON` is the whole API for the caller this library was built for, and that `Reads` and `Fields` let a caller plan what to fetch before any input exists. Those two facts are the strongest argument that the host-native, selective design buys something a ported engine cannot, and they are buried in method docs.

Teachable as written: the concept, yes; the package, not yet. The package doc reads as a design record that was promoted to documentation without being re-cut for the reader who has an expression and a payload and wants a result in the next ten minutes. Everything that reader needs is present. It is in the wrong order, and it is surrounded by everything the maintainers needed to convince themselves.
