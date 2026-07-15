package litmus

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Run struct {
	ID        string       `json:"id"`
	Timestamp time.Time    `json:"timestamp"`
	Revision  string       `json:"revision,omitempty"`
	BudgetUSD float64      `json:"budget_usd"`
	Cases     []CaseResult `json:"cases"`
}

type CaseDelta struct {
	Agent        string  `json:"agent"`
	CaseID       string  `json:"case_id"`
	Status       string  `json:"status"`
	CostDeltaUSD float64 `json:"cost_delta_usd"`
}

type Comparison struct {
	BaselineID        string      `json:"baseline_id"`
	CurrentID         string      `json:"current_id"`
	TotalCostDeltaUSD float64     `json:"total_cost_delta_usd"`
	Cases             []CaseDelta `json:"cases"`
}

type runSummary struct {
	ID        string        `json:"id"`
	Timestamp time.Time     `json:"timestamp"`
	Revision  string        `json:"revision,omitempty"`
	BudgetUSD float64       `json:"budget_usd"`
	Totals    runTotals     `json:"totals"`
	Cases     []summaryCase `json:"cases"`
}

type runTotals struct {
	Cases        int     `json:"cases"`
	Passed       int     `json:"passed"`
	Failed       int     `json:"failed"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	TotalTokens  int     `json:"total_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	DurationMS   int64   `json:"duration_ms"`
}

type summaryCase struct {
	Agent        string  `json:"agent"`
	CaseID       string  `json:"case_id"`
	Passed       bool    `json:"passed"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	DurationMS   int64   `json:"duration_ms"`
	DetailPath   string  `json:"detail_path"`
}

func NewRun(now time.Time, revision string, budgetUSD float64, cases []CaseResult) Run {
	timestamp := now.UTC()
	revision = safeRevision(revision)
	return Run{
		ID:        timestamp.Format("20060102T150405.000Z") + "-" + revision,
		Timestamp: timestamp,
		Revision:  revision,
		BudgetUSD: budgetUSD,
		Cases:     cases,
	}
}

func WriteRun(root string, run Run) (string, error) {
	run, err := normalizedRun(run)
	if err != nil {
		return "", err
	}

	resultsRoot, err := resultDirectory(root)
	if err != nil {
		return "", err
	}
	baseID := run.ID
	var directory string
	for suffix := 0; ; suffix++ {
		run.ID = baseID
		if suffix > 0 {
			run.ID = fmt.Sprintf("%s-%d", baseID, suffix)
		}
		directory = filepath.Join(resultsRoot, run.ID)
		err := os.Mkdir(directory, 0o755)
		if err == nil {
			break
		}
		if !os.IsExist(err) {
			return "", fmt.Errorf("create run directory: %w", err)
		}
	}
	casesDirectory := filepath.Join(directory, "cases")
	if err := os.Mkdir(casesDirectory, 0o755); err != nil {
		return "", fmt.Errorf("create case directory: %w", err)
	}

	summary := summaryFor(run)
	for _, result := range sortedCases(run.Cases) {
		path := filepath.Join(casesDirectory, caseDetailName(result))
		if err := writeIndentedJSON(path, result); err != nil {
			return "", fmt.Errorf("write case result: %w", err)
		}
	}
	if err := writeIndentedJSON(filepath.Join(directory, "summary.json"), summary); err != nil {
		return "", fmt.Errorf("write summary: %w", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "report.md"), []byte(markdownReport(summary)), 0o644); err != nil {
		return "", fmt.Errorf("write report: %w", err)
	}
	return directory, nil
}

func ReadRun(path string) (Run, error) {
	directory, err := resolveRunDirectory(path)
	if err != nil {
		return Run{}, err
	}

	var summary runSummary
	if err := loadJSON(directory, filepath.Join(directory, "summary.json"), &summary); err != nil {
		return Run{}, fmt.Errorf("read summary: %w", err)
	}
	run := Run{
		ID:        summary.ID,
		Timestamp: summary.Timestamp,
		Revision:  summary.Revision,
		BudgetUSD: summary.BudgetUSD,
		Cases:     make([]CaseResult, 0, len(summary.Cases)),
	}
	for _, entry := range summary.Cases {
		result, err := readCaseDetail(directory, entry)
		if err != nil {
			return Run{}, err
		}
		run.Cases = append(run.Cases, result)
	}
	run.Cases = sortedCases(run.Cases)
	return run, nil
}

func Compare(baseline, current Run) Comparison {
	baselineCases := caseMap(baseline.Cases)
	currentCases := caseMap(current.Cases)
	keys := make(map[caseKey]struct{}, len(baselineCases)+len(currentCases))
	for key := range baselineCases {
		keys[key] = struct{}{}
	}
	for key := range currentCases {
		keys[key] = struct{}{}
	}

	ordered := make([]caseKey, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Slice(ordered, func(left, right int) bool {
		if ordered[left].agent == ordered[right].agent {
			return ordered[left].caseID < ordered[right].caseID
		}
		return ordered[left].agent < ordered[right].agent
	})

	comparison := Comparison{
		BaselineID:        baseline.ID,
		CurrentID:         current.ID,
		TotalCostDeltaUSD: centsToUSD(costCents(totalCost(current.Cases) - totalCost(baseline.Cases))),
		Cases:             make([]CaseDelta, 0, len(ordered)),
	}
	for _, key := range ordered {
		previous, wasPresent := baselineCases[key]
		next, isPresent := currentCases[key]
		comparison.Cases = append(comparison.Cases, CaseDelta{
			Agent:        key.agent,
			CaseID:       key.caseID,
			Status:       comparisonStatus(previous, wasPresent, next, isPresent),
			CostDeltaUSD: centsToUSD(costCents(next.CostUSD - previous.CostUSD)),
		})
	}
	return comparison
}

type caseKey struct {
	agent  string
	caseID string
}

func caseMap(cases []CaseResult) map[caseKey]CaseResult {
	results := make(map[caseKey]CaseResult, len(cases))
	for _, result := range cases {
		results[caseKey{agent: result.Agent, caseID: result.CaseID}] = result
	}
	return results
}

func comparisonStatus(previous CaseResult, wasPresent bool, next CaseResult, isPresent bool) string {
	switch {
	case !wasPresent:
		return "added"
	case !isPresent:
		return "removed"
	case previous.Passed && !next.Passed:
		return "regressed"
	case !previous.Passed && next.Passed:
		return "improved"
	default:
		return "unchanged"
	}
}

func normalizedRun(run Run) (Run, error) {
	if run.Revision == "" {
		run.Revision = "unknown"
	}
	if err := validateResultComponent("revision", run.Revision); err != nil {
		return Run{}, err
	}
	if !isFinite(run.BudgetUSD) || run.BudgetUSD < 0 {
		return Run{}, fmt.Errorf("budget_usd must be finite and non-negative")
	}
	if run.ID == "" {
		if run.Timestamp.IsZero() {
			return Run{}, fmt.Errorf("run timestamp is required when ID is not set")
		}
		run.Timestamp = run.Timestamp.UTC()
		run.ID = run.Timestamp.Format("20060102T150405.000Z") + "-" + run.Revision
	} else {
		if err := validateResultComponent("run ID", run.ID); err != nil {
			return Run{}, err
		}
		if run.Timestamp.IsZero() {
			timestamp, err := timestampFromID(run.ID)
			if err != nil {
				return Run{}, err
			}
			run.Timestamp = timestamp
		} else {
			run.Timestamp = run.Timestamp.UTC()
		}
	}

	seen := make(map[caseKey]struct{}, len(run.Cases))
	for _, result := range run.Cases {
		if err := validateCaseIdentityComponent("case agent", result.Agent); err != nil {
			return Run{}, err
		}
		if err := validateCaseIdentityComponent("case ID", result.CaseID); err != nil {
			return Run{}, err
		}
		key := caseKey{agent: result.Agent, caseID: result.CaseID}
		if _, exists := seen[key]; exists {
			return Run{}, fmt.Errorf("duplicate case result for %s/%s", result.Agent, result.CaseID)
		}
		seen[key] = struct{}{}
		if result.InputTokens < 0 || result.OutputTokens < 0 || result.DurationMS < 0 {
			return Run{}, fmt.Errorf("case result metrics must not be negative")
		}
		if !isFinite(result.CostUSD) || result.CostUSD < 0 {
			return Run{}, fmt.Errorf("case cost_usd must be finite and non-negative")
		}
	}
	return run, nil
}

func resultDirectory(root string) (string, error) {
	resolvedRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	resolvedRoot, err = filepath.EvalSymlinks(resolvedRoot)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	for _, component := range []string{"litmus", "results"} {
		path := filepath.Join(resolvedRoot, component)
		info, err := os.Lstat(path)
		switch {
		case err == nil:
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return "", fmt.Errorf("results path component %q must be a directory", component)
			}
		case os.IsNotExist(err):
			if err := os.Mkdir(path, 0o755); err != nil {
				return "", fmt.Errorf("create results path component %q: %w", component, err)
			}
		default:
			return "", fmt.Errorf("inspect results path component %q: %w", component, err)
		}
		resolvedRoot = path
	}
	return resolvedRoot, nil
}

func resolveRunDirectory(path string) (string, error) {
	directory, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve run directory: %w", err)
	}
	directory, err = filepath.EvalSymlinks(directory)
	if err != nil {
		return "", fmt.Errorf("resolve run directory: %w", err)
	}
	info, err := os.Stat(directory)
	if err != nil {
		return "", fmt.Errorf("inspect run directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("run path must be a directory")
	}
	return directory, nil
}

func readCaseDetail(directory string, entry summaryCase) (CaseResult, error) {
	if err := validateCaseIdentityComponent("summary case agent", entry.Agent); err != nil {
		return CaseResult{}, err
	}
	if err := validateCaseIdentityComponent("summary case ID", entry.CaseID); err != nil {
		return CaseResult{}, err
	}
	expectedPath := filepath.ToSlash(filepath.Join("cases", entry.Agent+"--"+entry.CaseID+".json"))
	if entry.DetailPath != expectedPath {
		return CaseResult{}, fmt.Errorf("case detail path must be %q", expectedPath)
	}

	var result CaseResult
	path := filepath.Join(directory, filepath.FromSlash(entry.DetailPath))
	if err := loadJSON(directory, path, &result); err != nil {
		return CaseResult{}, fmt.Errorf("read case detail %q: %w", entry.DetailPath, err)
	}
	if result.Agent != entry.Agent || result.CaseID != entry.CaseID {
		return CaseResult{}, fmt.Errorf("case detail identity does not match summary")
	}
	return result, nil
}

func validateResultComponent(kind, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", kind)
	}
	for index, character := range value {
		if !((character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			(character == '.' && index > 0) ||
			character == '-' || character == '_') {
			return fmt.Errorf("%s must be a safe path component", kind)
		}
	}
	if value == "." || value == ".." {
		return fmt.Errorf("%s must be a safe path component", kind)
	}
	return nil
}

func validateCaseIdentityComponent(kind, value string) error {
	if err := validateResultComponent(kind, value); err != nil {
		return err
	}
	if strings.Contains(value, "--") {
		return fmt.Errorf("%s must not contain double hyphens", kind)
	}
	return nil
}

func safeRevision(revision string) string {
	revision = strings.TrimSpace(revision)
	if err := validateResultComponent("revision", revision); err != nil {
		return "unknown"
	}
	return revision
}

func timestampFromID(id string) (time.Time, error) {
	timestamp, _, ok := strings.Cut(id, "-")
	if !ok {
		return time.Time{}, fmt.Errorf("run timestamp is required when ID is not timestamp-based")
	}
	for _, layout := range []string{"20060102T150405.000Z", "20060102T150405Z", "20060102T150405"} {
		parsed, err := time.Parse(layout, timestamp)
		if err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("run ID has an invalid timestamp")
}

func summaryFor(run Run) runSummary {
	summary := runSummary{
		ID:        run.ID,
		Timestamp: run.Timestamp,
		Revision:  run.Revision,
		BudgetUSD: run.BudgetUSD,
		Cases:     make([]summaryCase, 0, len(run.Cases)),
	}
	for _, result := range sortedCases(run.Cases) {
		summary.Totals.Cases++
		if result.Passed {
			summary.Totals.Passed++
		} else {
			summary.Totals.Failed++
		}
		summary.Totals.InputTokens += result.InputTokens
		summary.Totals.OutputTokens += result.OutputTokens
		summary.Totals.CostUSD += result.CostUSD
		summary.Totals.DurationMS += result.DurationMS
		summary.Cases = append(summary.Cases, summaryCase{
			Agent:        result.Agent,
			CaseID:       result.CaseID,
			Passed:       result.Passed,
			InputTokens:  result.InputTokens,
			OutputTokens: result.OutputTokens,
			CostUSD:      result.CostUSD,
			DurationMS:   result.DurationMS,
			DetailPath:   filepath.ToSlash(filepath.Join("cases", caseDetailName(result))),
		})
	}
	summary.Totals.TotalTokens = summary.Totals.InputTokens + summary.Totals.OutputTokens
	return summary
}

func sortedCases(cases []CaseResult) []CaseResult {
	sorted := append([]CaseResult(nil), cases...)
	sort.Slice(sorted, func(left, right int) bool {
		if sorted[left].Agent == sorted[right].Agent {
			return sorted[left].CaseID < sorted[right].CaseID
		}
		return sorted[left].Agent < sorted[right].Agent
	})
	return sorted
}

func caseDetailName(result CaseResult) string {
	return result.Agent + "--" + result.CaseID + ".json"
}

func writeIndentedJSON(path string, value any) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func markdownReport(summary runSummary) string {
	var report strings.Builder
	fmt.Fprintf(&report, "# Litmus Run %s\n\n", summary.ID)
	report.WriteString("## Metadata\n\n| Field | Value |\n| --- | --- |\n")
	fmt.Fprintf(&report, "| Timestamp | %s |\n", summary.Timestamp.UTC().Format(time.RFC3339))
	fmt.Fprintf(&report, "| Revision | %s |\n", summary.Revision)
	fmt.Fprintf(&report, "| Budget USD | %.2f |\n\n", summary.BudgetUSD)
	report.WriteString("## Totals\n\n| Metric | Value |\n| --- | --- |\n")
	fmt.Fprintf(&report, "| Cases | %d |\n", summary.Totals.Cases)
	fmt.Fprintf(&report, "| Passed | %d |\n", summary.Totals.Passed)
	fmt.Fprintf(&report, "| Failed | %d |\n", summary.Totals.Failed)
	fmt.Fprintf(&report, "| Input tokens | %d |\n", summary.Totals.InputTokens)
	fmt.Fprintf(&report, "| Output tokens | %d |\n", summary.Totals.OutputTokens)
	fmt.Fprintf(&report, "| Total tokens | %d |\n", summary.Totals.TotalTokens)
	fmt.Fprintf(&report, "| Cost USD | %.2f |\n", summary.Totals.CostUSD)
	fmt.Fprintf(&report, "| Duration ms | %d |\n\n", summary.Totals.DurationMS)
	report.WriteString("## Cases\n\n| Agent | Case | Status | Input tokens | Output tokens | Cost USD | Duration ms | Detail |\n| --- | --- | --- | ---: | ---: | ---: | ---: | --- |\n")
	for _, result := range summary.Cases {
		status := "fail"
		if result.Passed {
			status = "pass"
		}
		fmt.Fprintf(
			&report,
			"| %s | %s | %s | %d | %d | %.2f | %d | %s |\n",
			result.Agent,
			result.CaseID,
			status,
			result.InputTokens,
			result.OutputTokens,
			result.CostUSD,
			result.DurationMS,
			result.DetailPath,
		)
	}
	return report.String()
}

func totalCost(cases []CaseResult) float64 {
	var total float64
	for _, result := range cases {
		if isFinite(result.CostUSD) {
			total += result.CostUSD
		}
	}
	return total
}

func costCents(value float64) int64 {
	if !isFinite(value) {
		return 0
	}
	return int64(math.Round(value * 100))
}

func centsToUSD(cents int64) float64 {
	return float64(cents) / 100
}
