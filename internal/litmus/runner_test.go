package litmus

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadCaseRejectsInvalidBudget(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "litmus/cases/reviewer/bad.json"), `{
		"id": "bad",
		"agent": "reviewer",
		"task": "x",
		"max_budget_usd": 0,
		"assertions": [{"type": "contains", "value": "x"}]
	}`)

	_, err := LoadCase(root, "reviewer", "bad")
	if err == nil || !strings.Contains(err.Error(), "max_budget_usd") {
		t.Fatalf("LoadCase() error = %v, want max_budget_usd validation", err)
	}
}

func TestEffectiveBudgetUsesRemainingRunBudget(t *testing.T) {
	got, err := EffectiveBudget(0.10, 0.80, 0.74)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0.06 {
		t.Fatalf("EffectiveBudget() = %.2f, want 0.06", got)
	}
}

func TestEffectiveBudgetRejectsInvalidLimits(t *testing.T) {
	for _, test := range []struct {
		name                       string
		caseLimit, runLimit, spent float64
	}{
		{name: "case limit", caseLimit: 0, runLimit: 0.80, spent: 0},
		{name: "run limit", caseLimit: 0.10, runLimit: 0, spent: 0},
		{name: "exhausted", caseLimit: 0.10, runLimit: 0.80, spent: 0.80},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := EffectiveBudget(test.caseLimit, test.runLimit, test.spent); err == nil {
				t.Fatal("EffectiveBudget() error = nil, want an error")
			}
		})
	}
}

func TestEffectiveBudgetRejectsNonFiniteValues(t *testing.T) {
	tests := []struct {
		name                       string
		caseLimit, runLimit, spent float64
	}{
		{name: "NaN case limit", caseLimit: math.NaN(), runLimit: 0.80, spent: 0},
		{name: "NaN run limit", caseLimit: 0.10, runLimit: math.NaN(), spent: 0},
		{name: "NaN spent", caseLimit: 0.10, runLimit: 0.80, spent: math.NaN()},
		{name: "positive infinity case limit", caseLimit: math.Inf(1), runLimit: 0.80, spent: 0},
		{name: "negative infinity case limit", caseLimit: math.Inf(-1), runLimit: 0.80, spent: 0},
		{name: "positive infinity run limit", caseLimit: 0.10, runLimit: math.Inf(1), spent: 0},
		{name: "negative infinity run limit", caseLimit: 0.10, runLimit: math.Inf(-1), spent: 0},
		{name: "positive infinity spent", caseLimit: 0.10, runLimit: 0.80, spent: math.Inf(1)},
		{name: "negative infinity spent", caseLimit: 0.10, runLimit: 0.80, spent: math.Inf(-1)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := EffectiveBudget(test.caseLimit, test.runLimit, test.spent); err == nil || !strings.Contains(err.Error(), "finite") {
				t.Fatalf("EffectiveBudget() error = %v, want non-finite budget rejection", err)
			}
		})
	}
}

func TestEffectiveBudgetRejectsUnsafeValues(t *testing.T) {
	tests := []struct {
		name                       string
		caseLimit, runLimit, spent float64
	}{
		{name: "sub-cent case limit", caseLimit: 0.005, runLimit: 1, spent: 0},
		{name: "negative spent", caseLimit: 0.10, runLimit: 0.80, spent: -0.01},
		{name: "cent conversion overflow", caseLimit: math.MaxFloat64, runLimit: math.MaxFloat64, spent: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := EffectiveBudget(test.caseLimit, test.runLimit, test.spent)
			if err == nil {
				t.Fatalf("EffectiveBudget() = %v, nil error; want validation error", got)
			}
		})
	}
}

func TestEffectiveBudgetFloorsToCentsWithinLimits(t *testing.T) {
	tests := []struct {
		name                       string
		caseLimit, runLimit, spent float64
		want                       float64
	}{
		{name: "case limit", caseLimit: 0.109, runLimit: 1, spent: 0, want: 0.10},
		{name: "remaining run limit", caseLimit: 0.20, runLimit: 0.805, spent: 0.70, want: 0.10},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := EffectiveBudget(test.caseLimit, test.runLimit, test.spent)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("EffectiveBudget() = %.2f, want %.2f", got, test.want)
			}
			if math.IsNaN(got) || math.IsInf(got, 0) || got > test.caseLimit || got > test.runLimit-test.spent {
				t.Fatalf("EffectiveBudget() = %v, want finite value within caps", got)
			}
		})
	}
}

func TestEffectiveBudgetPreservesExactDecimalCent(t *testing.T) {
	got, err := EffectiveBudget(1, 0.30, 0.20)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0.10 {
		t.Fatalf("EffectiveBudget() = %.2f, want 0.10", got)
	}
}

func TestEffectiveBudgetPreservesExactCentLimit(t *testing.T) {
	got, err := EffectiveBudget(1, 0.29, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0.29 {
		t.Fatalf("EffectiveBudget() = %.2f, want 0.29", got)
	}
}

func TestEffectiveBudgetUsesRemainingBudgetBeforeCents(t *testing.T) {
	got, err := EffectiveBudget(1, 0.805, 0.001)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0.80 {
		t.Fatalf("EffectiveBudget() = %.2f, want 0.80", got)
	}
}

func TestEffectiveBudgetDoesNotRoundPastFractionalCap(t *testing.T) {
	got, err := EffectiveBudget(0.289, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0.28 || got > 0.289 {
		t.Fatalf("EffectiveBudget() = %.2f, want 0.28 without exceeding the cap", got)
	}
}

func TestLoadCaseRejectsTraversal(t *testing.T) {
	root := t.TempDir()

	for _, test := range []struct {
		name, agent, id string
	}{
		{name: "agent", agent: "../reviewer", id: "case"},
		{name: "case", agent: "reviewer", id: "../case"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := LoadCase(root, test.agent, test.id); err == nil {
				t.Fatal("LoadCase() error = nil, want traversal rejection")
			}
		})
	}
}

func TestLoadCaseRejectsRequestedIdentityMismatch(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "litmus/cases/reviewer/case.json")

	for _, test := range []struct {
		name, contents string
	}{
		{name: "agent", contents: validCaseJSON("case", "other")},
		{name: "id", contents: validCaseJSON("other", "reviewer")},
	} {
		t.Run(test.name, func(t *testing.T) {
			writeFile(t, path, test.contents)
			if _, err := LoadCase(root, "reviewer", "case"); err == nil {
				t.Fatal("LoadCase() error = nil, want identity mismatch error")
			}
		})
	}
}

func TestLoadCaseLoadsValidCase(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "litmus/cases/reviewer/case.json"), validCaseJSON("case", "reviewer"))

	got, err := LoadCase(root, "reviewer", "case")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "case" || got.Agent != "reviewer" || got.Task != "review task" || got.MaxBudgetUSD != 0.10 || len(got.Assertions) != 1 {
		t.Fatalf("LoadCase() = %#v, want decoded case", got)
	}
}

func TestLoadersRejectMalformedJSON(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "litmus/cases/reviewer/case.json"), `{`)
	writeFile(t, filepath.Join(root, "litmus/manifests/manifest.json"), `{`)

	if _, err := LoadCase(root, "reviewer", "case"); err == nil {
		t.Fatal("LoadCase() error = nil, want JSON decode error")
	}
	if _, err := LoadManifest(root, "manifest"); err == nil {
		t.Fatal("LoadManifest() error = nil, want JSON decode error")
	}
}

func TestLoadersRejectSymlinkEscapes(t *testing.T) {
	t.Run("case", func(t *testing.T) {
		root, outside := t.TempDir(), t.TempDir()
		writeFile(t, filepath.Join(outside, "reviewer/case.json"), validCaseJSON("case", "reviewer"))
		symlink(t, outside, filepath.Join(root, "litmus/cases"))

		if _, err := LoadCase(root, "reviewer", "case"); err == nil {
			t.Fatal("LoadCase() error = nil, want symlink escape rejection")
		}
	})

	t.Run("manifest", func(t *testing.T) {
		root, outside := t.TempDir(), t.TempDir()
		writeFile(t, filepath.Join(outside, "manifest.json"), `{"cases":[{"agent":"reviewer","case":"case"}]}`)
		symlink(t, outside, filepath.Join(root, "litmus/manifests"))

		if _, err := LoadManifest(root, "manifest"); err == nil {
			t.Fatal("LoadManifest() error = nil, want symlink escape rejection")
		}
	})
}

func TestLoadManifestRejectsInvalidManifest(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name, contents string
	}{
		{name: "empty cases", contents: `{"cases":[]}`},
		{name: "empty agent", contents: `{"cases":[{"case":"case"}]}`},
		{name: "empty case", contents: `{"cases":[{"agent":"reviewer"}]}`},
		{name: "traversal agent", contents: `{"cases":[{"agent":"../reviewer","case":"case"}]}`},
		{name: "traversal case", contents: `{"cases":[{"agent":"reviewer","case":"../case"}]}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writeFile(t, filepath.Join(root, "litmus/manifests", test.name+".json"), test.contents)

			if _, err := LoadManifest(root, test.name); err == nil {
				t.Fatal("LoadManifest() error = nil, want validation error")
			}
		})
	}

	if _, err := LoadManifest(root, "../manifest"); err == nil {
		t.Fatal("LoadManifest() error = nil, want traversal rejection")
	}
}

func validCaseJSON(id, agent string) string {
	return fmt.Sprintf(`{
		"id": %q,
		"agent": %q,
		"task": "review task",
		"max_budget_usd": 0.10,
		"assertions": [{"type": "contains", "value": "APPROVE"}]
	}`, id, agent)
}

func symlink(t *testing.T, target, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
