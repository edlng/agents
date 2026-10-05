// Command clismoke runs the provider smoke test through the Claude CLI. It is
// a development tool; the graded runtime is delegation/cmd/delegate.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/edlng/agents/litmus-eval/delegation/evals/cliprovider"
	"github.com/edlng/agents/litmus-eval/delegation/internal/smoke"
)

func main() {
	model := flag.String("model", "claude-sonnet-5-5", "model ID")
	runs := flag.String("runs", "delegation/runs", "audit trail root")
	flag.Parse()
	sum, err := smoke.Run(context.Background(), cliprovider.CLI{MaxBudgetUSD: 0.25}, *model, *runs)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	b, _ := json.MarshalIndent(sum, "", "  ")
	fmt.Println(string(b))
}
