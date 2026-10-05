// Command devrun runs a full review through the Claude CLI, using the same
// harness as delegate run. Use it to tune prompts without the API key.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/edlng/agents/litmus-eval/delegation/evals/cliprovider"
	"github.com/edlng/agents/litmus-eval/delegation/internal/dispatch"
	"github.com/edlng/agents/litmus-eval/delegation/internal/harness"
	"github.com/edlng/agents/litmus-eval/delegation/internal/provider"
)

func main() {
	taskDir := flag.String("task", "delegation/fixtures/endpoint-allowlist", "task directory")
	runs := flag.String("runs", "delegation/runs", "run directory root")
	budget := flag.Float64("budget", 2.00, "run budget in USD")
	coordModel := flag.String("coordinator-model", "", "override the coordinator model")
	workerModel := flag.String("worker-model", "", "override every sub-agent model")
	record := flag.String("record", "", "append every model exchange to this JSONL file")
	flag.Parse()
	limits := dispatch.DefaultLimits
	limits.BudgetUSD = *budget
	var p provider.Provider = cliprovider.CLI{MaxBudgetUSD: *budget}
	if *record != "" {
		rec, err := provider.NewRecorder(p, *record)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer rec.Close()
		p = rec
	}
	res, err := harness.Run(context.Background(), harness.Config{
		Provider: p, TaskDir: *taskDir, RunsRoot: *runs, Limits: limits,
		CoordinatorModel: *coordModel, WorkerModel: *workerModel,
	})
	b, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println(string(b))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
