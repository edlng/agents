// Command devstep launches workflow steps through the Claude CLI so prompts
// can be tuned without the API key. It uses the same dispatch path as the
// coordinator. Example:
//
//	go run ./delegation/evals/cmd/devstep --task delegation/fixtures/endpoint-allowlist \
//	  --steps code-review/security,spec-validation/tests --review
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/edlng/agents/litmus-eval/delegation/evals/cliprovider"
	"github.com/edlng/agents/litmus-eval/delegation/internal/audit"
	"github.com/edlng/agents/litmus-eval/delegation/internal/catalog"
	"github.com/edlng/agents/litmus-eval/delegation/internal/dispatch"
	"github.com/edlng/agents/litmus-eval/delegation/internal/task"
)

func main() {
	taskDir := flag.String("task", "delegation/fixtures/endpoint-allowlist", "task directory")
	steps := flag.String("steps", "code-review/security", "comma-separated workflow/step list, run in order")
	review := flag.Bool("review", false, "run the adversarial reviewer over the produced artifacts")
	runs := flag.String("runs", "delegation/runs", "audit trail root")
	budget := flag.Float64("budget", 1.00, "run budget in USD")
	flag.Parse()

	if err := run(*taskDir, *steps, *review, *runs, *budget); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(taskDir, steps string, review bool, runs string, budget float64) error {
	c, err := catalog.Load("delegation/workflows")
	if err != nil {
		return err
	}
	t, err := task.Load(taskDir)
	if err != nil {
		return err
	}
	log, err := audit.Create(runs, audit.NewID())
	if err != nil {
		return err
	}
	defer log.Close()
	limits := dispatch.DefaultLimits
	limits.BudgetUSD = budget
	r := dispatch.New(cliprovider.CLI{MaxBudgetUSD: budget}, log, c, t, limits)
	ctx := context.Background()
	fmt.Println("run:", log.Dir())
	for _, s := range strings.Split(steps, ",") {
		wf, step, ok := strings.Cut(strings.TrimSpace(s), "/")
		if !ok {
			return fmt.Errorf("step %q must be workflow/step", s)
		}
		print(r.LaunchStep(ctx, wf, step, nil, nil, ""))
	}
	if review {
		print(r.LaunchReview(ctx, r.ArtifactIDs(), ""))
	}
	fmt.Printf("spent: $%.4f\n", r.Spent())
	return nil
}

func print(v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}
