# Iteration 2 cold read: technical writer

> Given only the class README and go/jsonata at 4cb2f5d; told not to read design/. Verbatim.

## Grades

| Criterion | Grade | One-line justification |
|---|---|---|
| Idiomatic fit for Go | A- | Compile/MustCompile/String mirror `regexp`, `errors.Is`/`As` with sentinels, `iter.Seq`, ctx everywhere, Close on a borrowed handle; the `(value, present, err)` triple is unusual but the doc earns it. |
| Ergonomics of the common path | B- | Zero examples anywhere; nil `*Limits` and nil `*Env` are legal but you learn it three screens away from the call site; the map-vs-`*Object` two-arm switch is pushed onto every caller. |
| Correctness and footgun risk | C+ | The regex dialect is never named; nothing warns that `json.Unmarshal` into `any` destroys the large IDs this library exists to protect; encoding a native result with `encoding/json` silently ignores `Env.BytesEncoding` and HTML-escapes; a lambda result is undocumented; nil `*Object` is null while nil map is empty with no why. |
| Performance headroom the API permits | A- | Compile once, lazy admission, by-reference carriage, `Prepare`/`Select` sharing work, `Reads()` for fetch narrowing, view interfaces for foreign values; the doc conveys all of it, just not in an order that sells it. |
| Concept soundness | A- | The regex analogy is exactly right and stated in three sentences; the README overstates two of its own rules (7 and 9) in ways the package doc quietly corrects. |
| Overall | B | The content is precise and unusually honest; the structure is a specification that a reader has to reverse-engineer into instructions, and pkg.go.dev will show an Examples section that is empty. |

## The first-call test

Target: compile once, evaluate a rename over a decoded JSON body containing a large integer ID, distinguish absent from null from error, encode to JSON.

```go
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/openbindings/jsonata-evaluator/go/jsonata"
)

func main() {
	body := []byte(`{"user_id": 9007199254740993, "display_name": "ada"}`)

	expr, err := jsonata.Compile(`{ "id": user_id, "name": display_name }`, nil) // [1]
	if err != nil {
		log.Fatal(err)
	}

	input, err := jsonata.Unmarshal(body) // [2]
	if err != nil {
		log.Fatal(err)
	}

	out, present, err := expr.Eval(context.Background(), input, nil) // [3]
	if err != nil {
		var jerr *jsonata.Error // [4]
		if errors.As(err, &jerr) {
			log.Fatalf("%s at %d: %s", jerr.Code, jerr.Position, jerr.Message)
		}
		log.Fatal(err) // ctx.Err()
	}
	if !present { // [5]
		fmt.Println("no result")
		return
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false) // [6]
	if err := enc.Encode(out); err != nil { // [7]
		log.Fatal(err)
	}
}
```

Lines where the doc did not tell me what to write:

- **[1]** `nil` for limits. `Compile`'s doc says "under limits" and nothing about nil. The permission is in the `Limits` type doc ("a nil *Limits means all defaults"), which on pkg.go.dev is nine types further down. I guessed.
- **[2]** `jsonata.Unmarshal` instead of `json.Unmarshal`. The package doc never says why you would prefer it. The trap that matters for this library's stated purpose (large integer IDs) is that `json.Unmarshal` into `any` yields `float64` and rounds `9007199254740993` to `9007199254740992` before the evaluator sees it. Nothing in the doc says this. A reader who already uses `encoding/json` will keep using it and the number model will be silently defeated at the door. The doc also never says that `json.Decoder.UseNumber()` output (`map[string]any` with `json.Number`) is an equally good input.
- **[3]** `nil` for env. Same problem as [1]: legal per `Env`'s type doc, unstated at `Eval`.
- **[4]** `*jsonata.Error` as the `errors.As` target. `Error`'s doc says "Use errors.As" without saying the pointer type; `Is` is on `*Error` and the sentinels are `*Error`, so pointer it is, but I inferred it.
- **[5]** What to do on absent. `EvalJSON` says absent is "no bytes"; for the native path the doc defines absent (section five) and leaves the caller's response unspecified, which is correct, but a first-call reader needs one sentence of guidance ("an absent result usually means the input lacked the field; treat it as no output, not as an error").
- **[6]** `SetEscapeHTML(false)`. The doc says `$string` and `EvalJSON` do not HTML-escape, but never tells a caller encoding a native result that `encoding/json`'s default does, so a `<` in a name will round-trip differently through `Eval`+`json.Encoder` than through `EvalJSON`.
- **[7]** Whether `json.Encoder` is faithful at all. It is for `*Object` (has `MarshalJSON`) and `json.Number` (encoded as its token). It is **not** for `[]byte`: `encoding/json` always emits `base64.StdEncoding`, so a caller who set `Env.BytesEncoding = base64.RawURLEncoding` and then encodes natively gets a different string than `$string` or `EvalJSON` would produce. The doc's advice "or encodes the result, which handles both" is the sentence that sends the reader into this hole. There is no exported encoder for the native path.

Seven marked lines in a twenty-line program is the ergonomics grade.

## Outline as it should be

The current order is Values, Ownership, Numbers, Strings and bytes, Absence, Whole and selective evaluation, Bounds, Environment. Ownership is second, before the reader knows `Evaluation`, `Select`, `Complete`, or `Close` exist. Absence, which is needed for the first call, is fifth. The headline safety promise ("No expression, whatever its content, can terminate the process") is the first sentence of the seventh section. And pkg.go.dev sorts the type index alphabetically, so a reader who skips the prose lands on `Access` first and reaches `Expression` ninth of fifteen. The package doc is the only place the API can be walked in call order, and it does not walk it.

1. **Synopsis and first call.** What the package is in two sentences, then a six-line indented code block: `Compile`, `Eval`, `present`, `err`. Keep sentence one as is. Replace sentences two through four (the "evaluator class" paragraph) with a self-contained statement and a URL; see line edit 1. Move "No expression, whatever its content, can terminate the process" here from Bounds, since it is the promise an OpenBindings integrator is reading the page to find.
2. **Getting values in.** The admitted-type list (Values para 1), the nil rules with the missing why for nil `*Object`, a definition of "admitted" and "foreign", and the new sentence about `Unmarshal` versus `json.Unmarshal`. "Values are read by reference and are never copied on admission" stays here.
3. **Getting values out.** The whole Absence section moves here, followed by Values para 3 (carriage, map versus `*Object`, `Materialize`). The two-arm-switch advice becomes an Example function rather than a sentence.
4. **Errors.** New short section: `*Error`, `errors.As`, `errors.Is` against sentinels, `Code` and `Class`, where the language's codes are catalogued, and "ctx cancellation is returned as ctx.Err(), not as an Error" (moved from Bounds). Today errors are explained only in the `Error` type doc, so a reader of the package doc meets `CodeUnsupportedValue`, `D1001`, `D3030`, `T1006`, and `D1009` as bare tokens.
5. **Numbers.** As is, but lead with the memorable promise (see line edit 9), then admission widening, then results, then equality. Delete "the refusal rules below apply to results".
6. **Strings, bytes, and regular expressions.** As is, plus the regex dialect sentence (line edit 7). Take "A regex literal that does not compile fails at Compile" out of `Compile`'s doc and leave it here once.
7. **Objects and member order.** Values para 2 (map sorted, `*Object` insertion order, `Unmarshal` produces `*Object`) plus the object-equality sentence now at the end of Numbers ("Two objects are equal when they have the same members ... regardless of member order or of map versus *Object"), which is about objects, not numbers.
8. **Ownership and concurrency.** The Ownership section, now after the reader knows what an `Evaluation` is, merged with "The engine spawns no goroutines and holds no process-wide state" (from Bounds) and the `Expression`-is-safe-for-concurrent-use sentence (from `Compile`'s doc).
9. **Selective evaluation.** Whole and selective evaluation as is, with "prelude" defined, and a second indented code block showing `Prepare`, `Select`, `Close`.
10. **Bounds and cancellation.** Bounds minus the charging inventory (which moves into `Limits.MaxWork` and `Limits.MaxBytes`, where three quarters of it already lives) and minus the two sentences moved to sections 1 and 8.
11. **The environment.** Environment minus the duplicate Access-allowlist sentence (line edit 10).
12. **Conformance.** `Version`, `DocumentationCommit` with a clickable URL, where this member's declared divergences are recorded, and the one-line class definition with a link to the root README.

## Line edits

1. **"It is the Go member of the OpenBindings evaluator class (see the README at the repository root): the pinned JSONata documentation defines what an expression does; Go defines what the values are and how they combine."**
   Replace: "The JSONata documentation pinned by DocumentationCommit defines what an expression does. This package's value model, built from Go's own types, defines what values are and how they combine, the way regexp implements regular-expression notation over Go strings rather than porting another engine. The design is written up at https://github.com/openbindings/jsonata-evaluator#readme."
   Why: the module root is `go/`, so pkg.go.dev renders `go/README.md` (five lines, "Nothing here is usable yet"), not the root README the sentence points at. "Class" is undefined and reads as OOP. And "Go defines ... how they combine" is not what the Numbers section then says: Go's `int64 +` wraps, this package promotes to `*big.Int` or refuses; the package defines a model on top of Go, and should say so.

2. **"No JSON text is read or written on the evaluation path; Unmarshal and EvalJSON exist for callers that hold JSON bytes and add nothing the native path lacks."**
   Replace: "Inputs are Go values, not JSON text. A caller holding JSON bytes decodes them with Unmarshal, which keeps every integer exact; decoding with encoding/json into any turns 9007199254740993 into the float64 9007199254740992 before this package sees it. json.Decoder with UseNumber is also fine."
   Why: wrong altitude (a defensive claim about API status in the fourth sentence of the package) and it misses the one trap that defeats the library's purpose for its stated consumer.

3. **"A nil pointer is null. A nil []any is the empty array and a nil map[string]any is the empty object, so that an unset Go collection does not become null and change the meaning of a predicate over it."**
   Replace: "A nil []any is the empty array and a nil map[string]any is the empty object, because Go code routinely leaves a collection unset and $count(items) = 0 should hold for it rather than items being null. Every nil pointer, including a nil *Object and a nil *big.Int, is null."
   Why: as written, the second sentence appears to contradict the first for `*Object`. A reader will assume a bug. The why is stated but as an abstraction ("change the meaning of a predicate"); one concrete predicate is shorter and clearer.

4. **"Any other value is reached through the Env's Access; without one it is an error (CodeUnsupportedValue) when first used."**
   Replace: "These are the admitted values. Anything else is foreign: the expression can read it only through an Access supplied in Env, and without one it is an error (CodeUnsupportedValue) the first time the expression reads it. Admission is per value on first read; a value the expression never reads is never checked."
   Why: "admitted" and "foreign" are the two load-bearing terms of the whole document and neither is defined in the package doc (foreign is defined only in the `Access` type doc). The last sentence resolves an ambiguity the doc currently leaves open: `Prepare` says admission is lazy, `Eval` says nothing, and "refused" in Values and Numbers reads as an up-front pass.

5. **"Only a pure copy preserves the representation." / "a pure copy never encodes" / "A value the expression only selected, copied, or rearranged is returned as the value that was carried"**
   Replace: define once in Getting values out: "A value the expression only selects, copies, or rearranges is carried: it is returned as the same Go value, same type, same identity." Then use "carried" everywhere: "Only carriage preserves the representation"; "a carried []byte is never encoded".
   Why: three names for one rule (pure copy, carried, selected-copied-rearranged) across three sections; a reader cannot tell whether they are the same rule.

6. **"The float64 nearest to a decimal token is the model's representation of that decimal, not an approximation of a result; the refusal rules below apply to results."**
   Replace: "A decimal token is a float64; that is what a decimal is in this model, not a rounding of a result. Refusal applies to results, and is described next."
   Why: the sentence is doing a hedge's job (pre-empting "but you said you never round") in a spec's voice, and it forward-references rules the reader has not seen.

7. **"A regex literal that does not compile fails at Compile."** (Strings) and **"Regex literals are compiled here, so a regex that does not compile fails at Compile."** (Compile)
   Replace, in Strings only: "Regex literals use Go's regexp syntax (RE2: no backreferences, no lookaround; the i and m flags are honored) and are compiled by Compile, so a malformed literal fails there rather than at evaluation." Delete the sentence from `Compile`'s doc.
   Why: repetition, and the dialect is the single fact both target readers need most. A jsonata-js reader writes `/(?<=\$)\d+/` and gets a compile error with no doc explaining why; a `regexp` reader cannot confirm the syntax reference is `regexp/syntax`.

8. **"Every statement of a block prelude is evaluated before the first selection, its bindings are shared by every selection, and a failure in the prelude fails every selection."**
   Replace: "In a block whose final expression is an object constructor, the statements before it (the prelude) are evaluated once, before the first selection; their bindings are shared by every selection and a failure among them fails every selection."
   Why: "prelude" is this package's term, not JSONata's, and it is used before it is defined.

9. **"Every admitted numeric representation denotes one numeric value, and every operation that observes a number (arithmetic, comparison, ordering, membership, sort keys, $type, $string, object keys) is a function of that value, never of the representation."**
   Keep, but put this sentence in front of it: "Integers are exact at any size and floats are float64; a result that cannot be represented exactly is an error, never an approximation."
   Why: the existing sentence is correct and abstract. The promise a reader will remember and rely on ("exact integers, refuse rather than round") is currently reconstructable only from the fourth paragraph. It is also the README's rule 8, which the package doc never states as a sentence.

10. **"An expression can call the Access with any key, in any order, as many times as the work bound allows; Access is therefore the boundary of the closed environment and should be an allowlist over known types, never reflection over arbitrary structs."** (Environment)
    Replace: "The Access is the only door out of the closed environment; see Access for how to implement one."
    Why: near-verbatim duplicate of the `Access` type doc's second paragraph. State it once, at the type, where the implementer is looking.

11. **"It is called concurrently."** (Access)
    Replace: "It is called concurrently whenever the caller evaluates concurrently; the engine starts no goroutines of its own."
    Why: as written it contradicts "The engine spawns no goroutines" for any reader who has not connected the two sentences, which are 1,500 words apart.

12. **`Select`: "After Close, Select returns an error (CodeClosed)."** and **`Complete`: "After Close it returns an error (CodeClosed)."**
    Delete both; `Close`'s doc already says "further Select or Complete calls return an error (CodeClosed)".
    Why: stated three times. The same treatment applies to the timestamp fix, stated in `Prepare`, in Whole and selective evaluation, and in Environment; keep it in the package doc and in `Prepare`, drop it from Environment's parenthetical.

13. **"Compile parses and prepares an expression under limits."**
    Replace: "Compile parses and prepares an expression under limits; nil means DefaultLimits."
    Why: the call site is where the reader decides what to pass. Same edit for `Eval` and `Prepare`: "env may be nil".

14. **"A binding whose value is nil is null, not absent, so $x ?? d does not fall back for it while $x ?: d does; omit the binding to make $x absent."**
    Replace: "A binding whose value is nil is null, not absent. Unlike JavaScript's ??, JSONata's ?? falls back only for an absent operand (see the pinned documentation's operators page), so $x ?? d yields null while $x ?: d yields d; omit the binding to make $x absent."
    Why: a JS reader will read this as a bug because JS's `??` treats null as nullish. Name the contrast and cite the authority; that is the whole thesis of the library.

15. **"An absent result is present == false with no bytes and a nil error."** (EvalJSON)
    Replace: "An absent result is out == nil, present == false, and a nil error."
    Why: "no bytes" is ambiguous between nil and an empty slice, and callers write `if out == nil` or `len(out) == 0` on it.

Two more that would not fit the count but matter: `Reads()` says "a binding makes known false" with no reason, and a reader will assume a defect since `$x` reads no input member; say why (or narrow the rule). `ReasonUnsupportedCall` refers to "a function the planner has not qualified as selectable" without listing which functions are qualified, so a caller cannot predict the plan from the expression.

## What go doc loses

- **Every function body is `panic("unimplemented")`.** The API is a design skeleton. pkg.go.dev will render `go/README.md`, which does say "Status: design ... Nothing here is usable yet", so a reader who reads the module README is told; a reader who lands on the package page from a search is not. Until implementation lands, the package doc's first paragraph should carry the status line too.
- **There are no Example functions.** `go doc` never shows them, but pkg.go.dev puts an Examples section under every symbol that has one and an empty Examples index otherwise. The doc has no code at all, in any form. The minimum set: `ExampleCompile`, `ExampleExpression_Eval` (showing `present`), `ExampleExpression_Prepare` (Select, Close, Plan), `ExampleError` (errors.As and Class), `ExampleUnmarshal` (the big-integer point), and one for the map-versus-`*Object` switch.
- **The imports are only `context` and `iter`.** `json.Number`, `*big.Int`, and `base64.StdEncoding` appear throughout the prose but are not linked on pkg.go.dev because the package does not import those packages. Once the implementation imports them the links appear; until then the reader has to know which `Number` is meant.
- **`assert_test.go` carries compile-time proof** that the four `base64` encodings satisfy `BytesEncoding` and that `*Error` implements `Is`. The prose claims both; the reader has no way to know the claims are checked. That is fine, but the doc should then not hedge ("satisfy the interface unchanged" is already assertive; good).
- **`DocumentationCommit` is a bare SHA.** The source gives no URL either. A reader cannot get from `5d1473277e...` to a page without knowing the repository is `github.com/jsonata-js/jsonata` and that the docs live under `docs/`.
- **The Go-side README does not say where declared divergences live.** The root README promises each member "its ledger of declared divergences"; neither `go doc` nor `go/README.md` names the file. A pkg.go.dev reader who hits `$uppercase("ß")` returning `"ß"` cannot find out whether that is declared.
- Nothing else is lost: there are no unexported names with explanatory comments, and comment placement is correct throughout (every exported symbol has a doc that starts with its name).

## What a reader from jsonata-js or from regexp will misunderstand

**From jsonata-js:**

- *"`evaluate` returns `undefined` when there is no result, so I check `value == nil`."* They will conflate null and absent. Correcting sentence: the Absence section, moved to the Getting values out position, with `present` shown in the first code block.
- *"Objects keep insertion order, so `$keys` on my input is in the order I built it."* A `map[string]any` input gives sorted byte order. Correcting sentence: "Unlike jsonata-js, where an input object's key order is the JavaScript insertion order, a map[string]any has no order and is observed in sorted byte order; pass an *Object when input order matters."
- *"Numbers are doubles; `9007199254740993 = 9007199254740992` is true."* Here it is false and `9007199254740993 + 0.5` is an error rather than a rounded double. Correcting sentence: line edit 9's lead, plus "Where jsonata-js rounds to a double, this package carries the exact integer or refuses."
- *"`expr.assign('x', fn)` and `expr.registerFunction` exist."* They do not, and `Env.Bindings` accepts values only. Correcting sentence: at `Env.Bindings`, "Values only; there is no way to bind a function, by design (see The environment)." The current statement lives in the last section.
- *"Regex literals are JavaScript regexes; lookbehind and backreferences work."* They will fail at `Compile` with no explanation. Correcting sentence: line edit 7.
- *"`??` is JavaScript's nullish coalescing."* Correcting sentence: line edit 14.
- *"`$uppercase("ß")` is `"SS"`."* Correcting sentence exists ("Unicode simple case mapping"); it should add "unlike jsonata-js, which applies JavaScript's full mapping" and name it as a declared divergence.
- *"`$match` positions are string indexes."* JS gives UTF-16 code unit offsets; here code points. The doc says so; fine.
- *"If the expression evaluates to a lambda I get a function back."* The doc never says what `Eval` returns for a function-valued result. The output kinds exclude it; `$string` of one is T1006. State what `Eval` does with it.

**From regexp:**

- *"Bounded means linear time, like RE2."* The doc says "The bounds count work and bytes, not time" in the middle of a 200-word paragraph; move that sentence to the front of Bounds, and say in the synopsis that deadlines come from ctx.
- *"`MustCompile` takes one argument."* Two here; the second is nil for defaults. Correcting sentence: line edit 13.
- *"No match returns nil, so I test `value == nil`."* Same conflation as the JS reader, from the other direction. Same fix.
- *"`$replace` uses Go's `${name}` and `$1` expansion."* `$1` works, `${name}` does not, and `$$` is a literal dollar as in Go. The doc corrects this; it should also say whether `$0` is the whole match.
- *"An Expression is like a Regexp: stateless, no cleanup."* `Expression` is; `Evaluation` needs `Close`, which is not a `regexp` idea at all. The first code block should use `Eval`, and the selective example should show `defer v.Close()` so the borrow model is learned from code rather than from the Ownership section.
- *"The regex syntax inside JSONata literals is `regexp/syntax`."* They will assume it and be right, but the doc never confirms it. Line edit 7.

## On the concept

The README explains the idea to someone who knows regex and JSON and has heard of JSONata; it does not explain it to someone who has not. "The idea" is the best writing in either document: three sentences on `regexp`, three on JSONata, and the thesis lands. But the file opens with a definitional paragraph about "the OpenBindings evaluator class" before the idea, never says what JSONata is or links docs.jsonata.org, never says what OpenBindings is, and uses "class" in a taxonomic sense that a Go reader will parse as a type. "The difference is observable in results" is the sentence a skeptic will stop on, and the README offers no example of the reference departing from its own documentation; the `1/2` and `0.1 + 0.2` examples illustrate the documentation-versus-value-model split, not the documentation-versus-reference one. One concrete case, with the doc quote and the reference's output, would carry the whole section. The Membership list is a good implementer's contract but it is where a newcomer's attention will die; it belongs after a "What this means for you" paragraph, not before.

The package doc's account of the concept matches the README's in spirit and diverges from it in three places, and in all three the package doc is right. First, the README's rule 7 says "The environment is closed. No expression can reach host state." The package doc says "closed except for the clock, $random, and the Access and BytesEncoding the caller supplies," and then spends two paragraphs on the fact that `Access` is a door an expression can drive with any key. The package doc is honest; the README should say "closed except through what the caller places in the environment." Second, the README's rule 9 says a selectively obtained field "is identical to the same field of a complete evaluation," unconditionally. The package doc says "When Complete would succeed, Select returns the same field," and explains that a sibling field's `$error` is not a guard under selection. The package doc is right and the README's rule should carry the qualifier, because an OpenBindings integrator who relies on a transform's `$assert` will otherwise read rule 9 as a guarantee the library does not make. Third, the package summary says "Go defines what the values are and how they combine," and the README's rule 3 says a member "owes no more than its host offers." The Numbers section then describes int64-to-`*big.Int` promotion and exact-or-refuse semantics, which is considerably more than Go's `int64` arithmetic offers. The detailed model is the right one; the one-line summary in both documents should say that the member defines a value model built from the host's types, not that the host defines it.

One place the README is right and the package doc under-delivers: rule 8, "refuses rather than approximates," is the memorable promise of the whole design and the package doc never states it as a sentence. The Numbers section proves it example by example, and the reader is left to induce the rule. Say it once, up front, and the seven hundred words that follow become evidence instead of a specification the reader has to compile.
