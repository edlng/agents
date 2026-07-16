package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/edlng/agents/litmus-eval/litmus/internal/litmus"
)

const usage = `usage:
  litmus-eval list
  litmus-eval replay <agent> <case>
  litmus-eval probe <agent> <case> --budget <usd>
  litmus-eval batch <manifest> --budget <usd> [--include-replay-only] [--jobs <count>]
  litmus-eval grade <run-directory> --budget <usd> [--jobs 1]
  litmus-eval compare <baseline-run> <current-run>`

const defaultBatchJobs = 3
const defaultGradeJobs = 1

type command struct {
	Name              string
	Agent             string
	CaseID            string
	Manifest          string
	RunDirectory      string
	BaselineRun       string
	CurrentRun        string
	BudgetUSD         float64
	IncludeReplayOnly bool
	Jobs              int
}

type application struct {
	root     string
	runner   litmus.Runner
	now      func() time.Time
	revision func(string) string
}

type batchCompletion struct {
	index    int
	reserved float64
	result   litmus.CaseResult
	err      error
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "resolve root: %v\n", err)
		return 1
	}
	return application{
		root:     root,
		runner:   litmus.Runner{Root: root},
		now:      time.Now,
		revision: gitRevision,
	}.run(args, stdout, stderr)
}

func (app application) run(args []string, stdout, stderr io.Writer) int {
	parsed, err := parseArgs(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	switch parsed.Name {
	case "list":
		return app.runList(stdout, stderr)
	case "replay":
		return app.runReplay(parsed, stdout, stderr)
	case "probe":
		return app.runProbe(parsed, stdout, stderr)
	case "batch":
		return app.runBatch(parsed, stdout, stderr)
	case "grade":
		return app.runGrade(parsed, stdout, stderr)
	case "compare":
		return app.runCompare(parsed, stdout, stderr)
	default:
		fmt.Fprintln(stderr, usage)
		return 2
	}
}

func parseArgs(args []string) (command, error) {
	if len(args) == 0 {
		return command{}, fmt.Errorf("%s", usage)
	}

	switch args[0] {
	case "list":
		if len(args) != 1 {
			return command{}, usageError("list")
		}
		return command{Name: "list"}, nil
	case "replay":
		if len(args) != 3 {
			return command{}, usageError("replay <agent> <case>")
		}
		return command{Name: "replay", Agent: args[1], CaseID: args[2]}, nil
	case "probe":
		agent, caseID, budget, err := parseBudgetCaseArgs(args, "probe <agent> <case> --budget <usd>")
		if err != nil {
			return command{}, err
		}
		return command{Name: "probe", Agent: agent, CaseID: caseID, BudgetUSD: budget}, nil
	case "batch":
		if len(args) < 4 || args[2] != "--budget" {
			return command{}, usageError("batch <manifest> --budget <usd> [--include-replay-only] [--jobs <count>]")
		}
		budget, err := parseBudget(args[3])
		if err != nil {
			return command{}, err
		}
		parsed := command{
			Name:      "batch",
			Manifest:  args[1],
			BudgetUSD: budget,
			Jobs:      defaultBatchJobs,
		}
		jobsSet := false
		for index := 4; index < len(args); index++ {
			switch args[index] {
			case "--include-replay-only":
				if parsed.IncludeReplayOnly {
					return command{}, usageError("batch <manifest> --budget <usd> [--include-replay-only] [--jobs <count>]")
				}
				parsed.IncludeReplayOnly = true
			case "--jobs":
				if jobsSet || index+1 >= len(args) {
					return command{}, usageError("batch <manifest> --budget <usd> [--include-replay-only] [--jobs <count>]")
				}
				jobs, err := parseJobs(args[index+1])
				if err != nil {
					return command{}, err
				}
				parsed.Jobs = jobs
				jobsSet = true
				index++
			default:
				return command{}, usageError("batch <manifest> --budget <usd> [--include-replay-only] [--jobs <count>]")
			}
		}
		return parsed, nil
	case "grade":
		if len(args) < 4 || args[2] != "--budget" {
			return command{}, usageError("grade <run-directory> --budget <usd> [--jobs 1]")
		}
		budget, err := parseBudget(args[3])
		if err != nil {
			return command{}, err
		}
		parsed := command{
			Name:         "grade",
			RunDirectory: args[1],
			BudgetUSD:    budget,
			Jobs:         defaultGradeJobs,
		}
		jobsSet := false
		for index := 4; index < len(args); index++ {
			if args[index] != "--jobs" || jobsSet || index+1 >= len(args) {
				return command{}, usageError("grade <run-directory> --budget <usd> [--jobs 1]")
			}
			jobs, err := parseJobs(args[index+1])
			if err != nil {
				return command{}, err
			}
			if jobs != 1 {
				return command{}, fmt.Errorf("grade jobs must be 1 for cost-controlled grading")
			}
			parsed.Jobs = jobs
			jobsSet = true
			index++
		}
		return parsed, nil
	case "compare":
		if len(args) != 3 {
			return command{}, usageError("compare <baseline-run> <current-run>")
		}
		return command{Name: "compare", BaselineRun: args[1], CurrentRun: args[2]}, nil
	default:
		return command{}, fmt.Errorf("unknown command %q\n%s", args[0], usage)
	}
}

func parseBudgetCaseArgs(args []string, commandUsage string) (string, string, float64, error) {
	if len(args) != 5 || args[3] != "--budget" {
		return "", "", 0, usageError(commandUsage)
	}
	budget, err := parseBudget(args[4])
	if err != nil {
		return "", "", 0, err
	}
	return args[1], args[2], budget, nil
}

func parseBudget(value string) (float64, error) {
	budget, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(budget) || math.IsInf(budget, 0) || budget <= 0 {
		return 0, fmt.Errorf("budget must be a positive USD amount")
	}
	return budget, nil
}

func parseJobs(value string) (int, error) {
	jobs, err := strconv.Atoi(value)
	if err != nil || jobs <= 0 {
		return 0, fmt.Errorf("jobs must be a positive integer")
	}
	return jobs, nil
}

func usageError(arguments string) error {
	return fmt.Errorf("usage: litmus-eval %s", arguments)
}

func (app application) runList(stdout, stderr io.Writer) int {
	cases, err := listCases(app.root)
	if err != nil {
		fmt.Fprintf(stderr, "list cases: %v\n", err)
		return 1
	}
	for _, testCase := range cases {
		fmt.Fprintf(stdout, "%s/%s\n", testCase.Agent, testCase.ID)
	}
	return 0
}

func listCases(root string) ([]litmus.Case, error) {
	casesRoot := filepath.Join(root, "litmus", "cases")
	agents, err := os.ReadDir(casesRoot)
	if err != nil {
		return nil, err
	}

	var cases []litmus.Case
	for _, agent := range agents {
		if !agent.IsDir() || agent.Type()&os.ModeSymlink != 0 {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(casesRoot, agent.Name()))
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			id := strings.TrimSuffix(entry.Name(), ".json")
			testCase, err := litmus.LoadCase(root, agent.Name(), id)
			if err != nil {
				return nil, fmt.Errorf("%s/%s: %w", agent.Name(), id, err)
			}
			cases = append(cases, testCase)
		}
	}
	return cases, nil
}

func (app application) runReplay(parsed command, stdout, stderr io.Writer) int {
	testCase, err := litmus.LoadCase(app.root, parsed.Agent, parsed.CaseID)
	if err != nil {
		fmt.Fprintf(stderr, "load case: %v\n", err)
		return 1
	}
	result, err := litmus.Replay(app.root, testCase)
	if err != nil {
		fmt.Fprintf(stderr, "replay: %v\n", err)
		return 1
	}
	directory, err := app.writeRun(0, []litmus.CaseResult{result})
	if err != nil {
		fmt.Fprintf(stderr, "write run: %v\n", err)
		return 1
	}
	printCaseResult(stdout, result, directory)
	if !result.Passed {
		return 1
	}
	return 0
}

func (app application) runProbe(parsed command, stdout, stderr io.Writer) int {
	testCase, err := litmus.LoadCase(app.root, parsed.Agent, parsed.CaseID)
	if err != nil {
		fmt.Fprintf(stderr, "load case: %v\n", err)
		return 1
	}
	result, probeErr := app.litmusRunner().Probe(context.Background(), testCase, parsed.BudgetUSD, 0)
	if result.Agent == "" {
		fmt.Fprintf(stderr, "probe: %v\n", probeErr)
		return 1
	}
	directory, err := app.writeRun(parsed.BudgetUSD, []litmus.CaseResult{result})
	if err != nil {
		fmt.Fprintf(stderr, "write run: %v\n", err)
		return 1
	}
	printCaseResult(stdout, result, directory)
	if probeErr != nil {
		fmt.Fprintf(stderr, "probe: %v\n", probeErr)
		return 1
	}
	if !result.Passed {
		return 1
	}
	return 0
}

func (app application) runBatch(parsed command, stdout, stderr io.Writer) int {
	manifest, err := litmus.LoadManifest(app.root, parsed.Manifest)
	if err != nil {
		fmt.Fprintf(stderr, "load manifest: %v\n", err)
		return 1
	}

	cases := make([]litmus.Case, len(manifest.Cases))
	for index, item := range manifest.Cases {
		testCase, err := litmus.LoadCase(app.root, item.Agent, item.CaseID)
		if err != nil {
			fmt.Fprintf(stderr, "load case %s/%s: %v\n", item.Agent, item.CaseID, err)
			return 1
		}
		if parsed.IncludeReplayOnly {
			testCase.Live = true
		}
		if !testCase.Live {
			fmt.Fprintf(stderr, "load case %s/%s: case is not enabled for live probes\n", item.Agent, item.CaseID)
			return 1
		}
		cases[index] = testCase
	}

	runner := app.litmusRunner()
	completed := make([]bool, len(cases))
	resultsByIndex := make([]litmus.CaseResult, len(cases))
	done := make(chan batchCompletion, parsed.Jobs)
	var spent, reserved float64
	next, active := 0, 0
	failed := false
	var batchErr error

	for next < len(cases) || active > 0 {
		for batchErr == nil && active < parsed.Jobs && next < len(cases) {
			testCase := cases[next]
			allocation, err := reserveBatchCase(testCase.MaxBudgetUSD, parsed.BudgetUSD, spent, reserved)
			if err != nil {
				if strings.Contains(err.Error(), "no run budget remains") {
					next = len(cases)
					break
				}
				batchErr = err
				break
			}

			index := next
			next++
			active++
			reserved += allocation
			testCase.MaxBudgetUSD = allocation
			go func(index int, testCase litmus.Case, allocation float64) {
				result, probeErr := runner.Probe(context.Background(), testCase, allocation, 0)
				done <- batchCompletion{
					index:    index,
					reserved: allocation,
					result:   result,
					err:      probeErr,
				}
			}(index, testCase, allocation)
		}

		if active == 0 {
			break
		}

		completion := <-done
		active--
		reserved -= completion.reserved
		if completion.result.Agent == "" {
			next = len(cases)
			if batchErr == nil {
				batchErr = fmt.Errorf(
					"probe %s/%s: %w",
					manifest.Cases[completion.index].Agent,
					manifest.Cases[completion.index].CaseID,
					completion.err,
				)
			}
			continue
		}
		completed[completion.index] = true
		resultsByIndex[completion.index] = completion.result
		spent += completion.result.CostUSD
		if !completion.result.Passed {
			failed = true
		}
		if completion.err != nil && completion.result.ProviderError == "" && batchErr == nil {
			next = len(cases)
			batchErr = fmt.Errorf(
				"probe %s/%s: %w",
				completion.result.Agent,
				completion.result.CaseID,
				completion.err,
			)
		}
	}

	results := make([]litmus.CaseResult, 0, len(cases))
	for index, result := range resultsByIndex {
		if completed[index] {
			results = append(results, result)
		}
	}
	directory, err := app.writeRun(parsed.BudgetUSD, results)
	if err != nil {
		fmt.Fprintf(stderr, "write run: %v\n", err)
		return 1
	}
	for _, result := range results {
		printCaseResult(stdout, result, directory)
	}
	if batchErr != nil {
		fmt.Fprintf(stderr, "batch: %v\n", batchErr)
		return 1
	}
	if failed {
		return 1
	}
	return 0
}

func reserveBatchCase(caseLimit, runBudget, spent, reserved float64) (float64, error) {
	return litmus.EffectiveBudget(caseLimit, runBudget, spent+reserved)
}

func (app application) runGrade(parsed command, stdout, stderr io.Writer) int {
	run, err := litmus.ReadRun(parsed.RunDirectory)
	if err != nil {
		fmt.Fprintf(stderr, "read run: %v\n", err)
		return 1
	}
	grade, err := litmus.Grade(context.Background(), app.root, run, litmus.GradeOptions{
		BudgetUSD: parsed.BudgetUSD,
		Jobs:      parsed.Jobs,
		Executor:  app.runner.Executor,
	})
	if err != nil {
		fmt.Fprintf(stderr, "grade: %v\n", err)
		return 1
	}
	if err := litmus.WriteGrade(app.root, parsed.RunDirectory, grade); err != nil {
		fmt.Fprintf(stderr, "write grade: %v\n", err)
		return 1
	}
	fmt.Fprintf(
		stdout,
		"graded %s cases=%d subject=$%.2f grader=$%.2f %s\n",
		grade.RunID,
		len(grade.Cases),
		grade.SubjectCostUSD,
		grade.GraderCostUSD,
		filepath.Join(parsed.RunDirectory, "grader.json"),
	)
	if grade.Failed() {
		return 1
	}
	return 0
}

func (app application) runCompare(parsed command, stdout, stderr io.Writer) int {
	baseline, err := litmus.ReadRun(parsed.BaselineRun)
	if err != nil {
		fmt.Fprintf(stderr, "read baseline run: %v\n", err)
		return 1
	}
	current, err := litmus.ReadRun(parsed.CurrentRun)
	if err != nil {
		fmt.Fprintf(stderr, "read current run: %v\n", err)
		return 1
	}
	comparison := litmus.Compare(baseline, current)
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(comparison); err != nil {
		fmt.Fprintf(stderr, "write comparison JSON: %v\n", err)
		return 1
	}
	writeComparisonMarkdown(stdout, comparison)
	return 0
}

func (app application) litmusRunner() litmus.Runner {
	runner := app.runner
	runner.Root = app.root
	return runner
}

func (app application) writeRun(budgetUSD float64, results []litmus.CaseResult) (string, error) {
	now := time.Now
	if app.now != nil {
		now = app.now
	}
	revision := gitRevision
	if app.revision != nil {
		revision = app.revision
	}
	return litmus.WriteRun(app.root, litmus.NewRun(now(), revision(app.root), budgetUSD, results))
}

func printCaseResult(writer io.Writer, result litmus.CaseResult, directory string) {
	status := "FAIL"
	if result.Passed {
		status = "PASS"
	}
	fmt.Fprintf(writer, "%s %s/%s $%.2f %s\n", status, result.Agent, result.CaseID, result.CostUSD, directory)
}

func writeComparisonMarkdown(writer io.Writer, comparison litmus.Comparison) {
	fmt.Fprintln(writer, "\n## Litmus Comparison")
	fmt.Fprintln(writer, "\n| Agent | Case | Status | Cost delta USD |")
	fmt.Fprintln(writer, "| --- | --- | --- | ---: |")
	for _, delta := range comparison.Cases {
		fmt.Fprintf(writer, "| %s | %s | %s | %.2f |\n", delta.Agent, delta.CaseID, delta.Status, delta.CostDeltaUSD)
	}
}

func gitRevision(root string) string {
	command := exec.Command("git", "-C", root, "rev-parse", "--short", "HEAD")
	output, err := command.Output()
	if err != nil {
		return "unknown"
	}
	revision := strings.TrimSpace(string(output))
	if revision == "" {
		return "unknown"
	}
	return revision
}
