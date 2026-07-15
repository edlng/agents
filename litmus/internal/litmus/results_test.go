package litmus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestWriteRunCreatesReadableArtifacts(t *testing.T) {
	root := t.TempDir()
	run := Run{
		ID:        "20260715T143022-a1b2c3d",
		Timestamp: time.Date(2026, time.July, 15, 14, 30, 22, 0, time.UTC),
		Revision:  "a1b2c3d",
		BudgetUSD: 0.10,
		Cases: []CaseResult{{
			Agent:        "reviewer",
			CaseID:       "case",
			Passed:       true,
			InputTokens:  100,
			OutputTokens: 20,
			CostUSD:      0.04,
			DurationMS:   300,
		}},
	}

	directory, err := WriteRun(root, run)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(directory) != run.ID {
		t.Fatalf("WriteRun() directory = %q, want ID %q", directory, run.ID)
	}
	for _, name := range []string{"summary.json", "report.md", "cases/reviewer--case.json"} {
		if _, err := os.Stat(filepath.Join(directory, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}

	var summary struct {
		Totals struct {
			InputTokens  int     `json:"input_tokens"`
			OutputTokens int     `json:"output_tokens"`
			TotalTokens  int     `json:"total_tokens"`
			CostUSD      float64 `json:"cost_usd"`
			DurationMS   int64   `json:"duration_ms"`
			Passed       int     `json:"passed"`
			Failed       int     `json:"failed"`
		} `json:"totals"`
		Cases []struct {
			DetailPath string `json:"detail_path"`
		} `json:"cases"`
	}
	summaryContents, err := os.ReadFile(filepath.Join(directory, "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(summaryContents, &summary); err != nil {
		t.Fatal(err)
	}
	if summary.Totals.InputTokens != 100 || summary.Totals.OutputTokens != 20 ||
		summary.Totals.TotalTokens != 120 || summary.Totals.CostUSD != 0.04 ||
		summary.Totals.DurationMS != 300 || summary.Totals.Passed != 1 ||
		summary.Totals.Failed != 0 || len(summary.Cases) != 1 ||
		summary.Cases[0].DetailPath != "cases/reviewer--case.json" {
		t.Fatalf("summary = %#v, want aggregate totals and case detail path", summary)
	}

	read, err := ReadRun(directory)
	if err != nil {
		t.Fatal(err)
	}
	if read.ID != run.ID || len(read.Cases) != 1 || read.Cases[0].Agent != "reviewer" ||
		read.Cases[0].CaseID != "case" || !read.Cases[0].Passed || read.Cases[0].CostUSD != 0.04 {
		t.Fatalf("ReadRun() = %#v, want summary run", read)
	}

	report, err := os.ReadFile(filepath.Join(directory, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# Litmus Run 20260715T143022-a1b2c3d",
		"| Passed | 1 |",
		"| reviewer | case | pass |",
	} {
		if !strings.Contains(string(report), want) {
			t.Fatalf("report.md missing %q:\n%s", want, report)
		}
	}
}

func TestNewRunUsesUTCRevisionID(t *testing.T) {
	first := NewRun(
		time.Date(2026, time.July, 15, 7, 30, 22, 100*int(time.Millisecond), time.FixedZone("PDT", -7*60*60)),
		"a1b2c3d",
		0.10,
		nil,
	)
	second := NewRun(
		time.Date(2026, time.July, 15, 7, 30, 22, 101*int(time.Millisecond), time.FixedZone("PDT", -7*60*60)),
		"a1b2c3d",
		0.10,
		nil,
	)

	if first.ID != "20260715T143022.100Z-a1b2c3d" || first.Timestamp.Location() != time.UTC {
		t.Fatalf("NewRun() = %#v, want UTC millisecond timestamp-based ID", first)
	}
	if first.ID == second.ID {
		t.Fatalf("NewRun() IDs = %q and %q, want distinct IDs within one second", first.ID, second.ID)
	}
}

func TestWriteRunDisambiguatesExistingRunID(t *testing.T) {
	root := t.TempDir()
	run := Run{
		ID:        "20260715T143022.100Z-a1b2c3d",
		Timestamp: time.Date(2026, time.July, 15, 14, 30, 22, 100*int(time.Millisecond), time.UTC),
		Revision:  "a1b2c3d",
		BudgetUSD: 0.10,
		Cases:     []CaseResult{{Agent: "reviewer", CaseID: "case"}},
	}

	first, err := WriteRun(root, run)
	if err != nil {
		t.Fatal(err)
	}
	second, err := WriteRun(root, run)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || filepath.Base(second) != run.ID+"-1" {
		t.Fatalf("WriteRun() directories = %q, %q, want a numbered second run", first, second)
	}

	read, err := ReadRun(second)
	if err != nil {
		t.Fatal(err)
	}
	if read.ID != run.ID+"-1" {
		t.Fatalf("ReadRun() ID = %q, want %q", read.ID, run.ID+"-1")
	}
}

func TestReadRunLoadsFullCaseDetailsInStableOrder(t *testing.T) {
	root := t.TempDir()
	run := Run{
		ID:        "20260715T143022-a1b2c3d",
		Timestamp: time.Date(2026, time.July, 15, 14, 30, 22, 0, time.UTC),
		Revision:  "a1b2c3d",
		BudgetUSD: 0.10,
		Cases: []CaseResult{
			{
				Agent:        "z",
				CaseID:       "case",
				Model:        "haiku",
				Output:       "APPROVE",
				Passed:       true,
				InputTokens:  10,
				OutputTokens: 2,
				CostUSD:      0.01,
				DurationMS:   20,
			},
			{
				Agent:       "a",
				CaseID:      "case",
				Model:       "sonnet",
				PromptHash:  "prompt-hash",
				FixtureHash: "fixture-hash",
				Output:      "BLOCK: CWE-94",
				AssertionResults: []AssertionResult{{
					Assertion: Assertion{Type: "contains", Value: "BLOCK"},
					Passed:    true,
					Reason:    "output contains value",
				}},
				Passed:        false,
				InputTokens:   100,
				OutputTokens:  20,
				CostUSD:       0.04,
				DurationMS:    300,
				ProviderError: "provider timed out",
			},
		},
	}

	directory, err := WriteRun(root, run)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ReadRun(directory)
	if err != nil {
		t.Fatal(err)
	}
	want := []CaseResult{run.Cases[1], run.Cases[0]}
	if !reflect.DeepEqual(got.Cases, want) {
		t.Fatalf("ReadRun() cases = %#v, want full stable details %#v", got.Cases, want)
	}
}

func TestReadRunRejectsEscapingCaseDetailPath(t *testing.T) {
	directory := t.TempDir()
	summary := `{
		"id": "20260715T143022-a1b2c3d",
		"timestamp": "2026-07-15T14:30:22Z",
		"budget_usd": 0.10,
		"totals": {},
		"cases": [{
			"agent": "reviewer",
			"case_id": "case",
			"detail_path": "../outside.json"
		}]
	}`
	if err := os.WriteFile(filepath.Join(directory, "summary.json"), []byte(summary), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ReadRun(directory); err == nil {
		t.Fatal("ReadRun() error = nil, want detail path escape rejection")
	}
}

func TestCompareReportsStableCentRoundedDeltas(t *testing.T) {
	baseline := Run{ID: "base", Cases: []CaseResult{
		{Agent: "a", CaseID: "case", Passed: true, CostUSD: 0.031},
		{Agent: "a", CaseID: "improve", Passed: false, CostUSD: 0.05},
		{Agent: "a", CaseID: "same", Passed: true, CostUSD: 0.01},
		{Agent: "z", CaseID: "removed", Passed: true, CostUSD: 0.02},
	}}
	current := Run{ID: "next", Cases: []CaseResult{
		{Agent: "b", CaseID: "added", Passed: true, CostUSD: 0.03},
		{Agent: "a", CaseID: "same", Passed: true, CostUSD: 0.01},
		{Agent: "a", CaseID: "improve", Passed: true, CostUSD: 0.04},
		{Agent: "a", CaseID: "case", Passed: false, CostUSD: 0.051},
	}}

	diff := Compare(baseline, current)
	if diff.BaselineID != "base" || diff.CurrentID != "next" || diff.TotalCostDeltaUSD != 0.02 {
		t.Fatalf("Compare() = %#v, want IDs and cent-rounded total cost delta", diff)
	}
	want := []CaseDelta{
		{Agent: "a", CaseID: "case", Status: "regressed", CostDeltaUSD: 0.02},
		{Agent: "a", CaseID: "improve", Status: "improved", CostDeltaUSD: -0.01},
		{Agent: "a", CaseID: "same", Status: "unchanged", CostDeltaUSD: 0},
		{Agent: "b", CaseID: "added", Status: "added", CostDeltaUSD: 0.03},
		{Agent: "z", CaseID: "removed", Status: "removed", CostDeltaUSD: -0.02},
	}
	if len(diff.Cases) != len(want) {
		t.Fatalf("Compare() returned %d cases, want %d: %#v", len(diff.Cases), len(want), diff)
	}
	for index := range want {
		if diff.Cases[index] != want[index] {
			t.Fatalf("Compare() case %d = %#v, want %#v", index, diff.Cases[index], want[index])
		}
	}
}

func TestWriteRunRejectsUnsafeCaseIdentity(t *testing.T) {
	root := t.TempDir()
	run := Run{
		ID:        "20260715T143022-a1b2c3d",
		Timestamp: time.Date(2026, time.July, 15, 14, 30, 22, 0, time.UTC),
		Revision:  "a1b2c3d",
		BudgetUSD: 0.10,
		Cases: []CaseResult{{
			Agent:  "../../../../../escaped",
			CaseID: "case",
		}},
	}

	if _, err := WriteRun(root, run); err == nil {
		t.Fatal("WriteRun() error = nil, want unsafe case identity rejection")
	}
	if _, err := os.Stat(filepath.Join(root, "escaped--case.json")); !os.IsNotExist(err) {
		t.Fatalf("unsafe write created %q: %v", filepath.Join(root, "escaped--case.json"), err)
	}
}

func TestWriteRunRejectsCaseIdentityDelimiterCollision(t *testing.T) {
	root := t.TempDir()
	first := CaseResult{Agent: "reviewer--with", CaseID: "case"}
	second := CaseResult{Agent: "reviewer", CaseID: "with--case"}
	if caseDetailName(first) != caseDetailName(second) {
		t.Fatalf("case artifact names = %q and %q, want collision", caseDetailName(first), caseDetailName(second))
	}

	run := Run{
		ID:        "20260715T143022-a1b2c3d",
		Timestamp: time.Date(2026, time.July, 15, 14, 30, 22, 0, time.UTC),
		Revision:  "a1b2c3d",
		BudgetUSD: 0.10,
		Cases:     []CaseResult{first, second},
	}

	if _, err := WriteRun(root, run); err == nil {
		t.Fatal("WriteRun() error = nil, want case artifact delimiter collision rejection")
	}
	if _, err := os.Stat(filepath.Join(root, "litmus")); !os.IsNotExist(err) {
		t.Fatalf("WriteRun() created results path for rejected run: %v", err)
	}
}
