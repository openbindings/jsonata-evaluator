package jsonata_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/openbindings/jsonata-evaluator/go/jsonata"
)

// Compile once, evaluate many. The nil Limits means DefaultLimits.
func ExampleCompile() {
	expr, err := jsonata.Compile(`{ "id": user_id, "name": display_name }`, nil)
	if err != nil {
		panic(err)
	}
	fmt.Println(expr.String())
}

// Unmarshal keeps every integer exact; encoding/json into any would round
// the ID to 9007199254740992 before the evaluator saw it.
func ExampleUnmarshal() {
	in, err := jsonata.Unmarshal([]byte(`{"user_id": 9007199254740993, "display_name": "ada"}`))
	if err != nil {
		panic(err)
	}
	_ = in
}

// The three outcomes: error, absent, present (a present nil is null).
func ExampleExpression_Eval() {
	expr := jsonata.MustCompile(`{ "id": user_id, "name": display_name }`, nil)
	in, _ := jsonata.Unmarshal([]byte(`{"user_id": 9007199254740993, "display_name": "ada"}`))

	out, present, err := expr.Eval(context.Background(), in, nil)
	switch {
	case err != nil:
		panic(err)
	case !present:
		fmt.Println("absent")
	default:
		b, _ := jsonata.Marshal(out, nil)
		fmt.Println(string(b))
	}
}

// Select one field without computing the others; Complete reuses it.
func ExampleExpression_Prepare() {
	expr := jsonata.MustCompile(`{ "id": user_id, "summary": $string($) }`, nil)
	in, _ := jsonata.Unmarshal([]byte(`{"user_id": 1, "display_name": "ada"}`))

	ev, err := expr.Prepare(in, nil)
	if err != nil {
		panic(err)
	}
	defer ev.Close()

	if !ev.Plan().Selective() {
		fmt.Println("whole evaluation:", ev.Plan().Reason)
	}
	id, present, err := ev.Select(context.Background(), "id")
	_, _, _ = id, present, err
}

// Errors are *Error, matched by sentinel or by class.
func ExampleError() {
	_, _, err := jsonata.Eval(context.Background(), `$sum([1..100000000])`, nil, nil)
	switch {
	case errors.Is(err, jsonata.ErrBudget):
		fmt.Println("too expensive")
	case err != nil:
		var e *jsonata.Error
		if errors.As(err, &e) && e.Code.Class() == jsonata.ClassRaised {
			fmt.Println("the expression refused:", e.Value)
		}
	}
}
