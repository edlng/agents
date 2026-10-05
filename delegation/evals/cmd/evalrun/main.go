// Command evalrun measures every workflow, the adversarial reviewer, the final
// review writer, and the coordinator against named criteria, and writes
// delegation/evals/<workflow>/results.json. Model calls go through the Claude
// CLI (evaluation use, exempt from the runtime rule) or the API with --api.
//
//	go run ./delegation/evals/cmd/evalrun --trials 3 [--only code-review,coordinator] [--jobs 3]
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/edlng/agents/litmus-eval/delegation/evals/cliprovider"
	"github.com/edlng/agents/litmus-eval/delegation/internal/audit"
	"github.com/edlng/agents/litmus-eval/delegation/internal/catalog"
	"github.com/edlng/agents/litmus-eval/delegation/internal/coordinator"
	"github.com/edlng/agents/litmus-eval/delegation/internal/dispatch"
	"github.com/edlng/agents/litmus-eval/delegation/internal/provider"
	"github.com/edlng/agents/litmus-eval/delegation/internal/provider/anthropic"
	"github.com/edlng/agents/litmus-eval/delegation/internal/task"
)

const (
	workflowsDir   = "delegation/workflows"
	coordinatorDir = "delegation/coordinator"
	fixturesDir    = "delegation/fixtures"
	runsRoot       = "delegation/runs/evals"
)

// Trial is one execution of a case.
type Trial struct {
	CorrelationID string          `json:"correlation_id"`
	Pass          map[string]bool `json:"pass"`
	Notes         []string        `json:"notes,omitempty"`
	CostUSD       float64         `json:"cost_usd"`
	Error         string          `json:"error,omitempty"`
}

func (t *Trial) note(format string, args ...any) {
	t.Notes = append(t.Notes, fmt.Sprintf(format, args...))
}

type Case struct {
	ID       string
	Workflow string // results file this case belongs to
	Fixture  string
	Criteria []string
	Purpose  string
	Run      func(ctx context.Context, e *Env, t *Trial)
}

// Env is one trial's isolated harness state.
type Env struct {
	Provider provider.Provider
	Catalog  *catalog.Catalog
	Task     *task.Task
	Log      *audit.Log
	Runner   *dispatch.Runner
}

type CaseResult struct {
	ID       string   `json:"id"`
	Fixture  string   `json:"fixture"`
	Purpose  string   `json:"purpose"`
	Criteria []string `json:"criteria"`
	Trials   []Trial  `json:"trials"`
}

type Tally struct {
	Passed int     `json:"passed"`
	Total  int     `json:"total"`
	Rate   float64 `json:"rate"`
}

type Results struct {
	SchemaVersion string           `json:"schema_version"`
	Workflow      string           `json:"workflow"`
	Agent         string           `json:"agent"`
	Model         string           `json:"model"`
	Provider      string           `json:"provider"`
	PromptSHA256  string           `json:"prompt_sha256"`
	GeneratedAt   string           `json:"generated_at"`
	TrialsPerCase int              `json:"trials_per_case"`
	Criteria      map[string]Tally `json:"criteria"`
	Cases         []CaseResult     `json:"cases"`
	TotalCostUSD  float64          `json:"total_cost_usd"`
}

func main() {
	trials := flag.Int("trials", 1, "trials per case")
	only := flag.String("only", "", "comma-separated workflows to run (default all)")
	jobs := flag.Int("jobs", 3, "cases run in parallel")
	useAPI := flag.Bool("api", false, "use the Anthropic API instead of the Claude CLI")
	budget := flag.Float64("budget", 1.0, "budget per trial in USD")
	write := flag.Bool("write", true, "write results.json files")
	flag.Parse()

	var p provider.Provider = cliprovider.CLI{MaxBudgetUSD: *budget}
	providerName := "claude-cli (evals/ exemption)"
	if *useAPI {
		p, providerName = anthropic.New(), "anthropic-api"
	}
	cat, err := catalog.Load(workflowsDir)
	must(err)
	selected := map[string]bool{}
	for _, w := range strings.Split(*only, ",") {
		if w = strings.TrimSpace(w); w != "" {
			selected[w] = true
		}
	}
	var cases []Case
	for _, c := range allCases() {
		if len(selected) == 0 || selected[c.Workflow] {
			cases = append(cases, c)
		}
	}

	results := make([]CaseResult, len(cases))
	sem := make(chan struct{}, *jobs)
	var wg sync.WaitGroup
	for i, c := range cases {
		wg.Add(1)
		go func(i int, c Case) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			cr := CaseResult{ID: c.ID, Fixture: c.Fixture, Purpose: c.Purpose, Criteria: c.Criteria}
			for n := 0; n < *trials; n++ {
				cr.Trials = append(cr.Trials, runTrial(c, p, cat, *budget))
				t := cr.Trials[len(cr.Trials)-1]
				fmt.Printf("%-34s trial %d %s $%.4f %s\n", c.ID, n+1, passString(t, c.Criteria), t.CostUSD, t.Error)
				if strings.Contains(passString(t, c.Criteria), "FAIL") {
					for _, note := range t.Notes {
						if len(note) > 400 {
							note = note[:400] + "..."
						}
						fmt.Printf("    note: %s\n", note)
					}
				}
			}
			results[i] = cr
		}(i, c)
	}
	wg.Wait()

	byWorkflow := map[string][]CaseResult{}
	for i, c := range cases {
		byWorkflow[c.Workflow] = append(byWorkflow[c.Workflow], results[i])
	}
	var names []string
	for w := range byWorkflow {
		names = append(names, w)
	}
	sort.Strings(names)
	for _, w := range names {
		r := summarize(w, byWorkflow[w], cat, providerName, *trials)
		fmt.Printf("\n%s ($%.4f)\n", w, r.TotalCostUSD)
		for _, crit := range sortedKeys(r.Criteria) {
			t := r.Criteria[crit]
			fmt.Printf("  %-36s %d/%d\n", crit, t.Passed, t.Total)
		}
		if *write {
			b, _ := json.MarshalIndent(r, "", "  ")
			path := filepath.Join("delegation/evals", w, "results.json")
			must(os.MkdirAll(filepath.Dir(path), 0o755))
			must(os.WriteFile(path, append(b, '\n'), 0o644))
		}
	}
}

func runTrial(c Case, p provider.Provider, cat *catalog.Catalog, budget float64) (t Trial) {
	t.Pass = map[string]bool{}
	for _, crit := range c.Criteria {
		t.Pass[crit] = false
	}
	defer func() {
		if r := recover(); r != nil {
			t.Error = fmt.Sprint("panic: ", r)
		}
	}()
	ctx := context.Background()
	if c.Fixture == "" || c.Workflow == "coordinator" {
		c.Run(ctx, &Env{Provider: p, Catalog: cat}, &t)
		return t
	}
	tk, err := task.Load(filepath.Join(fixturesDir, c.Fixture))
	if err != nil {
		t.Error = err.Error()
		return t
	}
	log, err := audit.Create(filepath.Join(runsRoot, c.Workflow), audit.NewID()+"-"+c.ID)
	if err != nil {
		t.Error = err.Error()
		return t
	}
	t.CorrelationID = log.CorrelationID()
	limits := dispatch.DefaultLimits
	limits.BudgetUSD = budget
	e := &Env{Provider: p, Catalog: cat, Task: tk, Log: log, Runner: dispatch.New(p, log, cat, tk, limits)}
	c.Run(ctx, e, &t)
	t.CostUSD = e.Runner.Spent()
	if err := log.Close(); err != nil && t.Error == "" {
		t.Error = err.Error()
	}
	return t
}

func summarize(workflow string, cases []CaseResult, cat *catalog.Catalog, providerName string, trials int) Results {
	r := Results{
		SchemaVersion: "stage5.eval-results.v1", Workflow: workflow, Provider: providerName,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339), TrialsPerCase: trials,
		Criteria: map[string]Tally{}, Cases: cases,
	}
	var declared []string
	h := sha256.New()
	if workflow == "coordinator" {
		m, err := coordinator.LoadManifest(coordinatorDir)
		must(err)
		r.Agent, r.Model, declared = m.Agent, m.Model, m.Evaluation.Criteria
		h.Write([]byte(m.System))
	} else {
		w := cat.Workflows[workflow]
		r.Agent, r.Model, declared = w.Agent, w.Model, w.Evaluation.Criteria
		for _, s := range w.Steps {
			spec, err := w.Spec(s.ID)
			must(err)
			h.Write([]byte(spec.System))
		}
	}
	r.PromptSHA256 = hex.EncodeToString(h.Sum(nil))
	for _, crit := range declared {
		r.Criteria[crit] = Tally{}
	}
	for _, c := range cases {
		for _, t := range c.Trials {
			r.TotalCostUSD += t.CostUSD
			for _, crit := range c.Criteria {
				tl, ok := r.Criteria[crit]
				if !ok {
					panic(fmt.Sprintf("case %s measures undeclared criterion %s of %s", c.ID, crit, workflow))
				}
				tl.Total++
				if t.Pass[crit] {
					tl.Passed++
				}
				r.Criteria[crit] = tl
			}
		}
	}
	for crit, tl := range r.Criteria {
		if tl.Total > 0 {
			tl.Rate = float64(tl.Passed) / float64(tl.Total)
		}
		r.Criteria[crit] = tl
	}
	return r
}

func passString(t Trial, criteria []string) string {
	var parts []string
	for _, c := range criteria {
		mark := "FAIL"
		if t.Pass[c] {
			mark = "ok"
		}
		parts = append(parts, c+"="+mark)
	}
	return strings.Join(parts, " ")
}

func sortedKeys(m map[string]Tally) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
