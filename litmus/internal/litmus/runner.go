package litmus

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Assertion struct {
	Type    string `json:"type"`
	Value   string `json:"value,omitempty"`
	Path    string `json:"path,omitempty"`
	Command string `json:"command,omitempty"`
}

type Case struct {
	ID           string             `json:"id"`
	Agent        string             `json:"agent"`
	Task         string             `json:"task"`
	MaxBudgetUSD float64            `json:"max_budget_usd"`
	Live         bool               `json:"live"`
	Fixture      string             `json:"fixture,omitempty"`
	Assertions   []Assertion        `json:"assertions"`
	Validators   []Validator        `json:"validators,omitempty"`
	ModelGrader  *ModelGraderConfig `json:"model_grader,omitempty"`
	JSONSchema   json.RawMessage    `json:"json_schema,omitempty"`
}

type ManifestItem struct {
	Agent  string `json:"agent"`
	CaseID string `json:"case"`
}

type Manifest struct {
	Cases []ManifestItem `json:"cases"`
}

type AssertionResult struct {
	Assertion Assertion `json:"assertion"`
	Passed    bool      `json:"passed"`
	Reason    string    `json:"reason"`
}

type CaseStatus string

const (
	StatusPass              CaseStatus = "pass"
	StatusAgentFailure      CaseStatus = "agent_failure"
	StatusInfrastructureErr CaseStatus = "infra_error"
	StatusGraderError       CaseStatus = "grader_error"
)

type CaseResult struct {
	Agent            string            `json:"agent"`
	CaseID           string            `json:"case_id"`
	Model            string            `json:"model,omitempty"`
	PromptHash       string            `json:"prompt_hash,omitempty"`
	FixtureHash      string            `json:"fixture_hash,omitempty"`
	Output           string            `json:"output"`
	AssertionResults []AssertionResult `json:"assertion_results"`
	ValidatorResults []ValidatorResult `json:"validator_results,omitempty"`
	Status           CaseStatus        `json:"status,omitempty"`
	Passed           bool              `json:"passed"`
	InputTokens      int               `json:"input_tokens"`
	OutputTokens     int               `json:"output_tokens"`
	CostUSD          float64           `json:"cost_usd"`
	DurationMS       int64             `json:"duration_ms"`
	ProviderError    string            `json:"provider_error,omitempty"`
}

type ProviderRequest struct {
	Agent        string
	Task         string
	SystemPrompt string
	Model        string
	BudgetUSD    float64
	Workspace    string
	AllowTools   bool
	JSONSchema   string
}

type ProviderResponse struct {
	Output       string
	InputTokens  int
	OutputTokens int
	CostUSD      float64
	Duration     time.Duration
}

type Executor interface {
	Execute(context.Context, ProviderRequest) (ProviderResponse, error)
}

type Runner struct {
	Root     string
	Executor Executor
	Now      func() time.Time
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
	for _, validator := range testCase.Validators {
		if strings.TrimSpace(validator.Type) == "" {
			return Case{}, fmt.Errorf("validator type is required")
		}
		if validator.Type == "python_tests" && strings.TrimSpace(validator.Path) == "" {
			return Case{}, fmt.Errorf("python_tests validator path is required")
		}
	}
	if testCase.ModelGrader != nil {
		if strings.TrimSpace(testCase.ModelGrader.Model) == "" {
			return Case{}, fmt.Errorf("model_grader model is required")
		}
		if strings.TrimSpace(testCase.ModelGrader.Rubric) == "" {
			return Case{}, fmt.Errorf("model_grader rubric is required")
		}
		if !isFinite(testCase.ModelGrader.MaxBudgetUSD) || testCase.ModelGrader.MaxBudgetUSD <= 0 {
			return Case{}, fmt.Errorf("model_grader max_budget_usd must be positive")
		}
		if testCase.ModelGrader.MaxOutputTokens < 0 {
			return Case{}, fmt.Errorf("model_grader max_output_tokens must not be negative")
		}
	}
	if len(testCase.JSONSchema) > 0 && !json.Valid(testCase.JSONSchema) {
		return Case{}, fmt.Errorf("json_schema must be valid JSON")
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

func EvaluateAssertions(output, workspace string, assertions []Assertion) []AssertionResult {
	results := make([]AssertionResult, 0, len(assertions))
	for _, assertion := range assertions {
		passed, reason := evaluateAssertion(output, workspace, assertion)
		results = append(results, AssertionResult{
			Assertion: assertion,
			Passed:    passed,
			Reason:    reason,
		})
	}
	return results
}

func evaluateAssertion(output, workspace string, assertion Assertion) (bool, string) {
	switch assertion.Type {
	case "contains":
		if assertion.Value == "" {
			return false, "contains assertion requires a value"
		}
		if strings.Contains(output, assertion.Value) {
			return true, "output contains value"
		}
		return false, "output does not contain value"
	case "regex":
		if assertion.Value == "" {
			return false, "regex assertion requires a value"
		}
		pattern, err := regexp.Compile(assertion.Value)
		if err != nil {
			return false, fmt.Sprintf("invalid regex: %v", err)
		}
		if pattern.MatchString(output) {
			return true, "output matches regex"
		}
		return false, "output does not match regex"
	case "not_regex":
		if assertion.Value == "" {
			return false, "not_regex assertion requires a value"
		}
		pattern, err := regexp.Compile(assertion.Value)
		if err != nil {
			return false, fmt.Sprintf("invalid regex: %v", err)
		}
		if pattern.MatchString(output) {
			return false, "output matches forbidden regex"
		}
		return true, "output does not match forbidden regex"
	case "json_path":
		return evaluateJSONPath(output, assertion)
	case "file_contains":
		return evaluateFileContains(workspace, assertion)
	default:
		return false, fmt.Sprintf("unsupported assertion type %q", assertion.Type)
	}
}

func evaluateJSONPath(output string, assertion Assertion) (bool, string) {
	path, err := jsonPath(assertion.Path)
	if err != nil {
		return false, err.Error()
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(output), &object); err != nil {
		return false, fmt.Sprintf("output is not a JSON object: %v", err)
	}
	if object == nil {
		return false, "output is not a JSON object"
	}

	for index, component := range path {
		value, ok := object[component]
		if !ok {
			return false, fmt.Sprintf("JSON path %q was not found", assertion.Path)
		}
		if index == len(path)-1 {
			actual, err := jsonPrimitiveText(value)
			if err != nil {
				return false, fmt.Sprintf("JSON path %q does not contain a primitive value: %v", assertion.Path, err)
			}
			if actual == assertion.Value {
				return true, "JSON path matches value"
			}
			return false, fmt.Sprintf("JSON path %q does not match value", assertion.Path)
		}

		if err := json.Unmarshal(value, &object); err != nil {
			return false, fmt.Sprintf("JSON path %q does not refer to an object", assertion.Path)
		}
		if object == nil {
			return false, fmt.Sprintf("JSON path %q does not refer to an object", assertion.Path)
		}
	}

	return false, fmt.Sprintf("JSON path %q was not found", assertion.Path)
}

func jsonPath(path string) ([]string, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("json_path assertion requires a path")
	}
	components := strings.Split(path, ".")
	for _, component := range components {
		if component == "" {
			return nil, fmt.Errorf("json_path assertion path must be dot-separated object keys")
		}
	}
	return components, nil
}

func jsonPrimitiveText(raw json.RawMessage) (string, error) {
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return "", fmt.Errorf("multiple JSON values")
		}
		return "", err
	}

	switch value := value.(type) {
	case bool:
		return strconv.FormatBool(value), nil
	case string:
		return value, nil
	case json.Number:
		return string(value), nil
	default:
		return "", fmt.Errorf("value is not a JSON primitive")
	}
}

func evaluateFileContains(workspace string, assertion Assertion) (bool, string) {
	if assertion.Value == "" {
		return false, "file_contains assertion requires a value"
	}
	path, err := workspacePath(workspace, assertion.Path)
	if err != nil {
		return false, err.Error()
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Sprintf("read assertion file: %v", err)
	}
	if strings.Contains(string(contents), assertion.Value) {
		return true, "file contains value"
	}
	return false, "file does not contain value"
}

func workspacePath(workspace, path string) (string, error) {
	if strings.TrimSpace(workspace) == "" {
		return "", fmt.Errorf("workspace is required")
	}
	if err := validateRelativePath(path); err != nil {
		return "", err
	}

	resolvedWorkspace, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return "", fmt.Errorf("resolve workspace: %w", err)
	}
	resolvedPath, err := filepath.EvalSymlinks(filepath.Join(resolvedWorkspace, path))
	if err != nil {
		return "", fmt.Errorf("resolve assertion path: %w", err)
	}
	relative, err := filepath.Rel(resolvedWorkspace, resolvedPath)
	if err != nil {
		return "", fmt.Errorf("check assertion path containment: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("assertion path escapes workspace")
	}
	return resolvedPath, nil
}

func validateRelativePath(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("assertion path is required")
	}
	if filepath.IsAbs(path) || strings.HasPrefix(path, "/") || strings.HasPrefix(path, `\`) {
		return fmt.Errorf("assertion path must be workspace-relative")
	}
	for _, component := range strings.FieldsFunc(path, func(r rune) bool {
		return r == '/' || r == '\\'
	}) {
		if component == ".." {
			return fmt.Errorf("assertion path must not contain parent traversal")
		}
	}
	if filepath.Clean(path) == "." {
		return fmt.Errorf("assertion path is required")
	}
	return nil
}

func Replay(root string, testCase Case) (CaseResult, error) {
	if err := validateComponent("agent", testCase.Agent); err != nil {
		return CaseResult{}, err
	}
	if err := validateComponent("case id", testCase.ID); err != nil {
		return CaseResult{}, err
	}

	var replay struct {
		Output *string `json:"output"`
	}
	path := filepath.Join(root, "litmus", "replays", testCase.Agent, testCase.ID+".json")
	if err := loadJSON(root, path, &replay); err != nil {
		return CaseResult{}, err
	}
	if replay.Output == nil {
		return CaseResult{}, fmt.Errorf("replay output is required")
	}

	workspace, cleanup, err := copyFixture(root, testCase.Fixture)
	if err != nil {
		return CaseResult{}, err
	}
	defer cleanup()

	assertionResults := EvaluateAssertions(*replay.Output, workspace, testCase.Assertions)
	validatorResults := EvaluateValidators(*replay.Output, workspace, testCase.Validators)
	passed, status := evaluationStatus(assertionResults, validatorResults)
	return CaseResult{
		Agent:            testCase.Agent,
		CaseID:           testCase.ID,
		Output:           *replay.Output,
		AssertionResults: assertionResults,
		ValidatorResults: validatorResults,
		Status:           status,
		Passed:           passed,
		CostUSD:          0,
	}, nil
}

func EvaluateValidators(output, workspace string, validators []Validator) []ValidatorResult {
	results := make([]ValidatorResult, 0, len(validators))
	for _, validator := range validators {
		results = append(results, runValidator(output, workspace, validator))
	}
	return results
}

func evaluationStatus(assertions []AssertionResult, validators []ValidatorResult) (bool, CaseStatus) {
	for _, result := range validators {
		if result.Error {
			return false, StatusGraderError
		}
	}
	if !assertionsPassed(assertions) {
		return false, StatusAgentFailure
	}
	for _, result := range validators {
		if !result.Passed {
			return false, StatusAgentFailure
		}
	}
	return true, StatusPass
}

func statusForAssertions(results []AssertionResult) CaseStatus {
	if assertionsPassed(results) {
		return StatusPass
	}
	return StatusAgentFailure
}

func assertionsPassed(results []AssertionResult) bool {
	for _, result := range results {
		if !result.Passed {
			return false
		}
	}
	return true
}

func copyFixture(root, fixture string) (string, func(), error) {
	workspace, err := os.MkdirTemp("", "litmus-fixture-")
	if err != nil {
		return "", nil, fmt.Errorf("create fixture workspace: %w", err)
	}
	cleanup := func() {
		_ = os.RemoveAll(workspace)
	}
	if fixture == "" {
		return workspace, cleanup, nil
	}
	if err := validateComponent("fixture", fixture); err != nil {
		cleanup()
		return "", nil, err
	}

	source, err := fixturePath(root, fixture)
	if err != nil {
		cleanup()
		return "", nil, err
	}
	info, err := os.Stat(source)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("stat fixture: %w", err)
	}
	if !info.IsDir() {
		cleanup()
		return "", nil, fmt.Errorf("fixture must be a directory")
	}
	if err := copyDirectory(source, workspace); err != nil {
		cleanup()
		return "", nil, err
	}
	return workspace, cleanup, nil
}

func fixturePath(root, fixture string) (string, error) {
	resolvedRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	resolvedRoot, err = filepath.EvalSymlinks(resolvedRoot)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	requested := filepath.Join(resolvedRoot, "litmus", "fixtures", fixture)
	info, err := os.Lstat(requested)
	if err != nil {
		return "", fmt.Errorf("inspect fixture: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("fixture must not be a symlink")
	}
	source, err := filepath.EvalSymlinks(requested)
	if err != nil {
		return "", fmt.Errorf("resolve fixture: %w", err)
	}
	relative, err := filepath.Rel(resolvedRoot, source)
	if err != nil {
		return "", fmt.Errorf("check fixture containment: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("fixture path escapes root")
	}
	return source, nil
}

func copyDirectory(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("fixture must not contain symlinks: %s", path)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return fmt.Errorf("resolve fixture entry: %w", err)
		}
		if relative == "." {
			return nil
		}
		if entry.Name() == ".gitkeep" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("fixture contains unsupported file: %s", path)
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, contents, info.Mode().Perm())
	})
}

func (r Runner) Probe(ctx context.Context, testCase Case, runBudget, spent float64) (CaseResult, error) {
	budget, err := EffectiveBudget(testCase.MaxBudgetUSD, runBudget, spent)
	if err != nil {
		return CaseResult{}, err
	}
	if err := validateComponent("agent", testCase.Agent); err != nil {
		return CaseResult{}, err
	}
	if err := validateComponent("case id", testCase.ID); err != nil {
		return CaseResult{}, err
	}
	if strings.TrimSpace(testCase.Task) == "" {
		return CaseResult{}, fmt.Errorf("case task is required")
	}
	if len(testCase.Assertions) == 0 {
		return CaseResult{}, fmt.Errorf("case assertions are required")
	}
	if !testCase.Live {
		return CaseResult{}, fmt.Errorf("case %s/%s is not enabled for live probes", testCase.Agent, testCase.ID)
	}

	prompt, model, err := resolveProductionAgent(r.Root, testCase.Agent)
	if err != nil {
		return CaseResult{}, err
	}
	workspace, cleanup, err := copyFixture(r.Root, testCase.Fixture)
	if err != nil {
		return CaseResult{}, err
	}
	defer cleanup()

	promptDigest := sha256.Sum256([]byte(prompt))
	request := ProviderRequest{
		Agent:        testCase.Agent,
		Task:         testCase.Task,
		SystemPrompt: prompt,
		Model:        model,
		BudgetUSD:    budget,
		Workspace:    workspace,
		AllowTools:   false,
		JSONSchema:   string(testCase.JSONSchema),
	}
	executor := r.Executor
	if executor == nil {
		executor = claudeExecutor{}
	}
	response, providerErr := executor.Execute(ctx, request)

	result := CaseResult{
		Agent:        testCase.Agent,
		CaseID:       testCase.ID,
		Model:        model,
		PromptHash:   fmt.Sprintf("%x", promptDigest),
		Output:       response.Output,
		InputTokens:  response.InputTokens,
		OutputTokens: response.OutputTokens,
		CostUSD:      response.CostUSD,
		DurationMS:   response.Duration.Milliseconds(),
	}
	result.AssertionResults = EvaluateAssertions(result.Output, workspace, testCase.Assertions)
	result.ValidatorResults = EvaluateValidators(result.Output, workspace, testCase.Validators)
	result.Passed, result.Status = evaluationStatus(result.AssertionResults, result.ValidatorResults)
	if providerErr != nil {
		result.Passed = false
		result.Status = StatusInfrastructureErr
		result.ProviderError = providerErr.Error()
		return result, fmt.Errorf("execute provider: %w", providerErr)
	}
	return result, nil
}

type claudeExecutor struct{}

func (claudeExecutor) Execute(ctx context.Context, request ProviderRequest) (ProviderResponse, error) {
	if strings.TrimSpace(request.Workspace) == "" {
		return ProviderResponse{}, fmt.Errorf("provider workspace is required")
	}
	args, err := claudeArgs(request)
	if err != nil {
		return ProviderResponse{}, err
	}
	command := exec.CommandContext(ctx, "claude", args...)
	command.Dir = request.Workspace
	command.Stdin = strings.NewReader(request.Task)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	started := time.Now()
	runErr := command.Run()

	response, decodeErr := decodeProviderResponse(stdout.Bytes())
	if response.Duration == 0 {
		response.Duration = time.Since(started)
	}
	if runErr != nil {
		details := strings.TrimSpace(stderr.String())
		if decodeErr != nil {
			if details != "" {
				details += ": "
			}
			details += decodeErr.Error()
		}
		if details == "" {
			return response, fmt.Errorf("claude command failed: %w", runErr)
		}
		return response, fmt.Errorf("claude command failed: %w: %s", runErr, details)
	}
	if decodeErr != nil {
		return response, fmt.Errorf("decode claude response: %w", decodeErr)
	}
	return response, nil
}

func claudeArgs(request ProviderRequest) ([]string, error) {
	budget, err := providerBudget(request.BudgetUSD)
	if err != nil {
		return nil, err
	}
	args := []string{
		"-p",
		"--output-format", "json",
		"--model", request.Model,
		"--max-budget-usd", fmt.Sprintf("%.2f", budget),
		"--system-prompt", request.SystemPrompt,
	}
	if !request.AllowTools {
		args = append(args, "--tools", "")
	}
	if strings.TrimSpace(request.JSONSchema) != "" {
		args = append(args, "--json-schema", request.JSONSchema)
	}
	return args, nil
}

func providerBudget(requested float64) (float64, error) {
	cents, err := floorCents(requested)
	if err != nil {
		return 0, err
	}
	if cents <= 2 {
		return 0, fmt.Errorf("requested budget must be at least $0.03 to reserve provider headroom")
	}
	return (cents - 2) / 100, nil
}

type claudeAgentManifest struct {
	Name string `json:"name"`
}

type claudeAgentFrontmatter struct {
	Name   string `yaml:"name"`
	Model  string `yaml:"model"`
	Effort string `yaml:"effort"`
}

func resolveProductionAgent(root, agent string) (string, string, error) {
	if err := validateComponent("agent", agent); err != nil {
		return "", "", err
	}

	var manifest claudeAgentManifest
	manifestPath := filepath.Join(root, "agents", agent, "manifest.json")
	if err := loadJSON(root, manifestPath, &manifest); err != nil {
		return "", "", fmt.Errorf("load agent manifest: %w", err)
	}
	if manifest.Name != agent {
		return "", "", fmt.Errorf("agent manifest name %q does not match requested agent %q", manifest.Name, agent)
	}

	configPath := filepath.Join(root, "agents", agent, "claude.md")
	contents, err := readRootFile(root, filepath.Join("agents", agent, "claude.md"))
	if err != nil {
		return "", "", fmt.Errorf("read Claude agent %q: %w", configPath, err)
	}
	frontmatter, prompt, err := parseClaudeAgent(string(contents))
	if err != nil {
		return "", "", fmt.Errorf("parse Claude agent %q: %w", configPath, err)
	}
	if frontmatter.Name != agent {
		return "", "", fmt.Errorf("Claude agent name %q does not match requested agent %q", frontmatter.Name, agent)
	}
	model, err := normalizeModel(frontmatter.Model)
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(prompt) == "" {
		return "", "", fmt.Errorf("production prompt for agent %q is empty", agent)
	}
	return prompt, model, nil
}

func parseClaudeAgent(contents string) (claudeAgentFrontmatter, string, error) {
	normalized := strings.ReplaceAll(contents, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return claudeAgentFrontmatter{}, "", fmt.Errorf("missing YAML frontmatter")
	}
	end := strings.Index(normalized[4:], "\n---")
	if end < 0 {
		return claudeAgentFrontmatter{}, "", fmt.Errorf("unterminated YAML frontmatter")
	}
	end += 4
	var frontmatter claudeAgentFrontmatter
	if err := yaml.Unmarshal([]byte(normalized[4:end]), &frontmatter); err != nil {
		return claudeAgentFrontmatter{}, "", fmt.Errorf("invalid YAML frontmatter: %w", err)
	}
	prompt := strings.TrimPrefix(normalized[end+4:], "\n")
	return frontmatter, prompt, nil
}

func readRootFile(root, path string) ([]byte, error) {
	resolvedRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve root: %w", err)
	}
	resolvedRoot, err = filepath.EvalSymlinks(resolvedRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve root: %w", err)
	}
	resolvedPath, err := filepath.EvalSymlinks(filepath.Join(resolvedRoot, path))
	if err != nil {
		return nil, err
	}
	relative, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("check path containment: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return nil, fmt.Errorf("path escapes root")
	}
	return os.ReadFile(resolvedPath)
}

func normalizeModel(model string) (string, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return "", fmt.Errorf("agent model is required")
	}
	for prefix, alias := range map[string]string{
		"claude-sonnet-": "sonnet",
		"claude-haiku-":  "haiku",
		"claude-opus-":   "opus",
	} {
		if strings.HasPrefix(strings.ToLower(model), prefix) {
			return alias, nil
		}
	}
	return model, nil
}

func decodeProviderResponse(contents []byte) (ProviderResponse, error) {
	var raw map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		return ProviderResponse{}, fmt.Errorf("decode JSON: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return ProviderResponse{}, fmt.Errorf("decode JSON: multiple values")
		}
		return ProviderResponse{}, fmt.Errorf("decode JSON: %w", err)
	}
	if raw == nil {
		return ProviderResponse{}, fmt.Errorf("response must be a JSON object")
	}

	response, err := responseMetrics(raw)
	if err != nil {
		return response, err
	}
	result, ok := raw["result"]
	if !ok {
		return response, fmt.Errorf("response result is required")
	}
	if err := json.Unmarshal(result, &response.Output); err != nil {
		return response, fmt.Errorf("response result must be a string: %w", err)
	}

	isError, err := optionalBool(raw, "is_error")
	if err != nil {
		return response, err
	}
	providerErrors, err := optionalErrors(raw)
	if err != nil {
		return response, err
	}
	if isError || providerErrors != "" {
		if providerErrors == "" {
			providerErrors = "provider reported an error"
		}
		return response, fmt.Errorf("provider error: %s", providerErrors)
	}
	return response, nil
}

func responseMetrics(raw map[string]json.RawMessage) (ProviderResponse, error) {
	var response ProviderResponse
	if value, ok := raw["modelUsage"]; ok {
		var usage map[string]map[string]json.RawMessage
		if err := json.Unmarshal(value, &usage); err != nil {
			return response, fmt.Errorf("response modelUsage must be an object: %w", err)
		}
		for model, totals := range usage {
			input, err := optionalNonNegativeInt(totals, "inputTokens")
			if err != nil {
				return response, fmt.Errorf("response modelUsage %q: %w", model, err)
			}
			output, err := optionalNonNegativeInt(totals, "outputTokens")
			if err != nil {
				return response, fmt.Errorf("response modelUsage %q: %w", model, err)
			}
			if input > maxInt-response.InputTokens || output > maxInt-response.OutputTokens {
				return response, fmt.Errorf("response modelUsage token total overflows")
			}
			response.InputTokens += input
			response.OutputTokens += output
		}
	}
	if value, ok := raw["total_cost_usd"]; ok {
		cost, err := nonNegativeFloat(value, "response total_cost_usd")
		if err != nil {
			return response, err
		}
		response.CostUSD = cost
	}
	if value, ok := raw["duration_ms"]; ok {
		durationMS, err := nonNegativeInt(value, "response duration_ms")
		if err != nil {
			return response, err
		}
		if int64(durationMS) > int64(math.MaxInt64/time.Millisecond) {
			return response, fmt.Errorf("response duration_ms is too large")
		}
		response.Duration = time.Duration(durationMS) * time.Millisecond
	}
	return response, nil
}

const maxInt = int(^uint(0) >> 1)

func optionalNonNegativeInt(values map[string]json.RawMessage, key string) (int, error) {
	value, ok := values[key]
	if !ok {
		return 0, nil
	}
	return nonNegativeInt(value, key)
}

func nonNegativeInt(raw json.RawMessage, field string) (int, error) {
	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&number); err != nil {
		return 0, fmt.Errorf("%s must be a non-negative integer: %w", field, err)
	}
	value, err := strconv.ParseInt(string(number), 10, 64)
	if err != nil || value < 0 || value > int64(maxInt) {
		return 0, fmt.Errorf("%s must be a non-negative integer", field)
	}
	return int(value), nil
}

func nonNegativeFloat(raw json.RawMessage, field string) (float64, error) {
	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&number); err != nil {
		return 0, fmt.Errorf("%s must be a non-negative number: %w", field, err)
	}
	value, err := strconv.ParseFloat(string(number), 64)
	if err != nil || !isFinite(value) || value < 0 {
		return 0, fmt.Errorf("%s must be a non-negative number", field)
	}
	return value, nil
}

func optionalBool(raw map[string]json.RawMessage, key string) (bool, error) {
	value, ok := raw[key]
	if !ok {
		return false, nil
	}
	var result bool
	if err := json.Unmarshal(value, &result); err != nil {
		return false, fmt.Errorf("response %s must be a boolean: %w", key, err)
	}
	return result, nil
}

func optionalErrors(raw map[string]json.RawMessage) (string, error) {
	value, ok := raw["errors"]
	if !ok {
		return "", nil
	}
	var message string
	if err := json.Unmarshal(value, &message); err == nil {
		return strings.TrimSpace(message), nil
	}
	var messages []string
	if err := json.Unmarshal(value, &messages); err == nil {
		return strings.TrimSpace(strings.Join(messages, "; ")), nil
	}
	var values []json.RawMessage
	if err := json.Unmarshal(value, &values); err == nil {
		parts := make([]string, 0, len(values))
		for _, item := range values {
			parts = append(parts, string(item))
		}
		return strings.TrimSpace(strings.Join(parts, "; ")), nil
	}
	return "", fmt.Errorf("response errors must be a string or array")
}
