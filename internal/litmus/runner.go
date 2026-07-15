package litmus

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

type Assertion struct {
	Type    string `json:"type"`
	Value   string `json:"value,omitempty"`
	Path    string `json:"path,omitempty"`
	Command string `json:"command,omitempty"`
}

type Case struct {
	ID           string      `json:"id"`
	Agent        string      `json:"agent"`
	Task         string      `json:"task"`
	MaxBudgetUSD float64     `json:"max_budget_usd"`
	Fixture      string      `json:"fixture,omitempty"`
	Assertions   []Assertion `json:"assertions"`
}

type ManifestItem struct {
	Agent  string `json:"agent"`
	CaseID string `json:"case"`
}

type Manifest struct {
	Cases []ManifestItem `json:"cases"`
}

func LoadCase(root, agent, id string) (Case, error) {
	if err := validateComponent("agent", agent); err != nil {
		return Case{}, err
	}
	if err := validateComponent("case id", id); err != nil {
		return Case{}, err
	}

	var testCase Case
	if err := loadJSON(root, filepath.Join(root, "litmus", "cases", agent, id+".json"), &testCase); err != nil {
		return Case{}, err
	}
	if err := validateComponent("case id", testCase.ID); err != nil {
		return Case{}, err
	}
	if err := validateComponent("agent", testCase.Agent); err != nil {
		return Case{}, err
	}
	if testCase.ID != id {
		return Case{}, fmt.Errorf("case id does not match requested id")
	}
	if testCase.Agent != agent {
		return Case{}, fmt.Errorf("case agent does not match requested agent")
	}
	if strings.TrimSpace(testCase.Task) == "" {
		return Case{}, fmt.Errorf("case task is required")
	}
	if testCase.MaxBudgetUSD <= 0 {
		return Case{}, fmt.Errorf("case max_budget_usd must be positive")
	}
	if len(testCase.Assertions) == 0 {
		return Case{}, fmt.Errorf("case assertions are required")
	}
	return testCase, nil
}

func LoadManifest(root, name string) (Manifest, error) {
	if err := validateComponent("manifest name", name); err != nil {
		return Manifest{}, err
	}

	var manifest Manifest
	if err := loadJSON(root, filepath.Join(root, "litmus", "manifests", name+".json"), &manifest); err != nil {
		return Manifest{}, err
	}
	if len(manifest.Cases) == 0 {
		return Manifest{}, fmt.Errorf("manifest cases are required")
	}
	for _, item := range manifest.Cases {
		if err := validateComponent("manifest agent", item.Agent); err != nil {
			return Manifest{}, err
		}
		if err := validateComponent("manifest case id", item.CaseID); err != nil {
			return Manifest{}, err
		}
	}
	return manifest, nil
}

func EffectiveBudget(caseLimit, runLimit, spent float64) (float64, error) {
	if !isFinite(caseLimit) || !isFinite(runLimit) || !isFinite(spent) {
		return 0, fmt.Errorf("budget values must be finite")
	}
	if caseLimit <= 0 || runLimit <= 0 {
		return 0, fmt.Errorf("budget limits must be positive")
	}
	if spent < 0 {
		return 0, fmt.Errorf("spent budget must not be negative")
	}

	remaining := runLimit - spent
	if remaining <= 0 {
		return 0, fmt.Errorf("no run budget remains")
	}
	if caseLimit < remaining {
		remaining = caseLimit
	}
	cents, err := floorCents(remaining)
	if err != nil {
		return 0, err
	}
	if cents <= 0 {
		return 0, fmt.Errorf("no run budget remains")
	}
	return cents / 100, nil
}

func floorCents(value float64) (float64, error) {
	cents, err := scaledCents(value)
	if err != nil {
		return 0, err
	}
	rounded := math.Round(cents)
	if math.Abs(cents-rounded) <= math.Nextafter(cents, math.Inf(1))-cents {
		return rounded, nil
	}
	return math.Floor(cents), nil
}

func scaledCents(value float64) (float64, error) {
	cents := value * 100
	if math.IsInf(cents, 0) {
		return 0, fmt.Errorf("budget is too large to convert to cents")
	}
	return cents, nil
}

func loadJSON(root, path string, value any) error {
	resolvedRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	resolvedRoot, err = filepath.EvalSymlinks(resolvedRoot)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	resolvedPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	resolvedPath, err = filepath.EvalSymlinks(resolvedPath)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	relative, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil {
		return fmt.Errorf("check path containment: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return fmt.Errorf("path escapes root")
	}

	contents, err := os.ReadFile(resolvedPath)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(contents, value); err != nil {
		return fmt.Errorf("decode %s: %w", resolvedPath, err)
	}
	return nil
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func validateComponent(kind, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", kind)
	}
	if value == "." || value == ".." || strings.ContainsAny(value, `/\`) {
		return fmt.Errorf("%s must not contain a path traversal component", kind)
	}
	return nil
}
