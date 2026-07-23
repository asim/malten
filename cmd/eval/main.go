// Command eval runs the built-in evaluation suite and prints a report. It uses
// the deterministic stub backend by default; set MALTEN_LLM=claude (with
// ANTHROPIC_API_KEY) to evaluate the real model against the same bar.
//
//	go run ./cmd/eval
//	MALTEN_LLM=claude go run ./cmd/eval
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/asim/malten/internal/eval"
	"github.com/asim/malten/internal/llm"
)

func main() {
	var model llm.LLM = llm.NewStub()
	if os.Getenv("MALTEN_LLM") == "claude" {
		model = llm.NewClaude(os.Getenv("MALTEN_MODEL"))
	}

	rep, err := eval.Run(context.Background(), model, eval.Scenarios())
	if err != nil {
		fmt.Fprintln(os.Stderr, "eval error:", err)
		os.Exit(1)
	}
	fmt.Print(rep.String())
	if !rep.OK() {
		os.Exit(1)
	}
}
