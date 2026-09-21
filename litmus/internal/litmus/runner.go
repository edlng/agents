package litmus

import (
	"bufio"
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
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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
	Workflow     bool               `json:"workflow,omitempty"`
	AllowTools   bool               `json:"allow_tools,omitempty"`
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
	Agent                string                                `json:"agent"`
	CaseID               string                                `json:"case_id"`
	Model                string                                `json:"model,omitempty"`
	ProviderModels       []string                              `json:"provider_models,omitempty"`
	ProviderRequestModel string                                `json:"provider_request_model,omitempty"`
	ProviderRequestID    string                                `json:"provider_request_id,omitempty"`
	ProviderResponseID   string                                `json:"provider_response_id,omitempty"`
	ProviderSessionID    string                                `json:"provider_session_id,omitempty"`
	ProviderModelUsage   map[string]map[string]json.RawMessage `json:"provider_model_usage,omitempty"`
	PromptHash           string                                `json:"prompt_hash,omitempty"`
	FixtureHash          string                                `json:"fixture_hash,omitempty"`
	Output               string                                `json:"output"`
	AssertionResults     []AssertionResult                     `json:"assertion_results"`
	ValidatorResults     []ValidatorResult                     `json:"validator_results,omitempty"`
	Status               CaseStatus                            `json:"status,omitempty"`
	Passed               bool                                  `json:"passed"`
	InputTokens          int                                   `json:"input_tokens"`
	OutputTokens         int                                   `json:"output_tokens"`
	CostUSD              float64                               `json:"cost_usd"`
	DurationMS           int64                                 `json:"duration_ms"`
	ProviderError        string                                `json:"provider_error,omitempty"`
	GitDiff              string                                `json:"git_diff,omitempty"`
	GitStatus            string                                `json:"git_status,omitempty"`
	Workflow             *WorkflowResult                       `json:"workflow,omitempty"`
}

type ProviderRequest struct {
	Agent        string
	Task         string
	SystemPrompt string
	Model        string
	BudgetUSD    float64
	Workspace    string
	AllowTools   bool
	Workflow     bool
	JSONSchema   string
}

type ProviderResponse struct {
	Output             string
	ProviderModels     []string
	ProviderRequestID  string
	ProviderResponseID string
	ProviderSessionID  string
	ProviderModelUsage map[string]map[string]json.RawMessage
	InputTokens        int
	OutputTokens       int
	CostUSD            float64
	Duration           time.Duration
	TelemetryPresence  TelemetryPresence
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
		if validator.Type == "command" || validator.Type == "workspace_command" {
			if err := validateWorkspaceCommand(validator.Command); err != nil {
				return Case{}, err
			}
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

	return loadManifestFile(root, filepath.Join("litmus", "manifests", name+".json"))
}

func LoadManifestPath(root, relativePath string) (Manifest, error) {
	cleanPath, err := validateManifestPath(relativePath)
	if err != nil {
		return Manifest{}, err
	}
	return loadManifestFile(root, cleanPath)
}

func loadManifestFile(root, relativePath string) (Manifest, error) {
	var manifest Manifest
	if err := loadJSON(root, filepath.Join(root, relativePath), &manifest); err != nil {
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

func validateManifestPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("manifest path is required")
	}
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("manifest path must be relative to the repository root")
	}
	cleanPath := filepath.Clean(path)
	if cleanPath == "." || cleanPath == ".." ||
		strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("manifest path must remain within the repository root")
	}
	return cleanPath, nil
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

func prepareWorkflowWorkspace(ctx context.Context, root, workspace string) error {
	if _, err := runExternalCommand(ctx, workspace, "git", "init", "--quiet"); err != nil {
		return fmt.Errorf("initialize workflow git repository: %w", err)
	}
	if _, err := runExternalCommand(
		ctx,
		root,
		"node",
		"scripts/install.mjs",
		"claude",
		"--scope",
		"project",
		"--target",
		workspace,
	); err != nil {
		return fmt.Errorf("install workflow Claude agents: %w", err)
	}
	if _, err := runExternalCommand(ctx, workspace, "git", "add", "--all", "--force"); err != nil {
		return fmt.Errorf("stage workflow baseline: %w", err)
	}
	if _, err := runExternalCommand(
		ctx,
		workspace,
		"git",
		"-c",
		"user.name=Litmus",
		"-c",
		"user.email=litmus@example.invalid",
		"-c",
		"commit.gpgsign=false",
		"commit",
		"--quiet",
		"--allow-empty",
		"-m",
		"litmus workflow baseline",
	); err != nil {
		return fmt.Errorf("commit workflow baseline: %w", err)
	}
	return nil
}

func captureGitWorkspace(ctx context.Context, workspace string) (string, string) {
	diff, diffErr := runExternalCommand(ctx, workspace, "git", "diff", "--no-ext-diff", "--binary", "HEAD", "--")
	status, statusErr := runExternalCommand(ctx, workspace, "git", "status", "--short", "--untracked-files=all")
	if diffErr != nil || statusErr != nil {
		return "", ""
	}
	return string(diff), string(status)
}

func runExternalCommand(ctx context.Context, directory, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		details := strings.TrimSpace(string(output))
		if details == "" {
			return output, err
		}
		return output, fmt.Errorf("%w: %s", err, details)
	}
	return output, nil
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
	if testCase.Workflow {
		if err := prepareWorkflowWorkspace(ctx, r.Root, workspace); err != nil {
			return CaseResult{}, err
		}
	}

	promptDigest := sha256.Sum256([]byte(prompt))
	request := ProviderRequest{
		Agent:        testCase.Agent,
		Task:         testCase.Task,
		SystemPrompt: prompt,
		Model:        model,
		BudgetUSD:    budget,
		Workspace:    workspace,
		AllowTools:   testCase.Workflow || testCase.AllowTools,
		Workflow:     testCase.Workflow,
		JSONSchema:   string(testCase.JSONSchema),
	}
	executor := r.Executor
	if executor == nil {
		executor = claudeExecutor{}
	}
	response, providerErr := executor.Execute(ctx, request)

	result := CaseResult{
		Agent:                testCase.Agent,
		CaseID:               testCase.ID,
		Model:                model,
		ProviderModels:       response.ProviderModels,
		ProviderRequestModel: providerModel(request.Model),
		ProviderRequestID:    response.ProviderRequestID,
		ProviderResponseID:   response.ProviderResponseID,
		ProviderSessionID:    response.ProviderSessionID,
		ProviderModelUsage:   response.ProviderModelUsage,
		PromptHash:           fmt.Sprintf("%x", promptDigest),
		Output:               response.Output,
		InputTokens:          response.InputTokens,
		OutputTokens:         response.OutputTokens,
		CostUSD:              response.CostUSD,
		DurationMS:           response.Duration.Milliseconds(),
	}
	if testCase.Workflow {
		result.GitDiff, result.GitStatus = captureGitWorkspace(ctx, workspace)
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

	var response ProviderResponse
	var decodeErr error
	if request.Workflow {
		response, decodeErr = decodeWorkflowProviderResponse(stdout.Bytes(), request.Agent, request.JSONSchema)
	} else {
		response, decodeErr = decodeProviderResponse(stdout.Bytes())
	}
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
		"--model", providerModel(request.Model),
		"--max-budget-usd", fmt.Sprintf("%.2f", budget),
	}
	if request.Workflow {
		if err := validateComponent("agent", request.Agent); err != nil {
			return nil, err
		}
		args = append(args,
			"--output-format", "stream-json",
			"--verbose",
			"--forward-subagent-text",
			"--agent", request.Agent,
			"--permission-mode", "auto",
			"--permission-prompts", "none",
		)
	} else {
		args = append(args,
			"--output-format", "json",
			"--system-prompt", request.SystemPrompt,
		)
	}
	if !request.AllowTools {
		args = append(args, "--tools", "")
	}
	if strings.TrimSpace(request.JSONSchema) != "" {
		args = append(args, "--json-schema", request.JSONSchema)
	}
	return args, nil
}

func providerModel(model string) string {
	if strings.EqualFold(strings.TrimSpace(model), "sonnet") {
		return "claude-sonnet-5"
	}
	return model
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
	result, hasResult := raw["result"]
	if hasResult {
		if err := json.Unmarshal(result, &response.Output); err != nil {
			return response, fmt.Errorf("response result must be a string: %w", err)
		}
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
	if !hasResult {
		return response, fmt.Errorf("response result is required")
	}
	return response, nil
}

func decodeWorkflowProviderResponse(contents []byte, agent, requestJSONSchema string) (ProviderResponse, error) {
	scanner := bufio.NewScanner(bytes.NewReader(contents))
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)

	var response ProviderResponse
	var resultRaw map[string]json.RawMessage
	var assistantText []string
	var resultCount int
	var lastEventType string
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var raw map[string]json.RawMessage
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.UseNumber()
		if err := decoder.Decode(&raw); err != nil {
			return response, fmt.Errorf("decode workflow JSONL: %w", err)
		}
		if raw == nil {
			return response, fmt.Errorf("workflow JSONL event must be an object")
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			if err == nil {
				return response, fmt.Errorf("workflow JSONL event contains multiple JSON values")
			}
			return response, fmt.Errorf("decode workflow JSONL trailing value: %w", err)
		}
		eventType, err := optionalString(raw, "type")
		if err != nil {
			return response, err
		}
		lastEventType = eventType
		if eventType == "assistant" {
			assistantText = append(assistantText, workflowTopLevelAssistantText(raw)...)
		}
		if eventType != "result" {
			continue
		}
		resultCount++
		if resultCount > 1 {
			return response, fmt.Errorf("workflow response contains duplicate result events")
		}
		resultRaw = raw
	}
	if err := scanner.Err(); err != nil {
		return response, fmt.Errorf("read workflow JSONL: %w", err)
	}
	if resultCount == 0 {
		return response, fmt.Errorf("workflow response result is required")
	}
	if lastEventType != "result" {
		return response, fmt.Errorf("workflow response result event must be terminal")
	}
	metrics, err := responseMetrics(resultRaw)
	if err != nil {
		return response, err
	}
	response.ProviderModels = metrics.ProviderModels
	response.ProviderRequestID = metrics.ProviderRequestID
	response.ProviderResponseID = metrics.ProviderResponseID
	response.ProviderSessionID = metrics.ProviderSessionID
	response.ProviderModelUsage = metrics.ProviderModelUsage
	response.InputTokens = metrics.InputTokens
	response.OutputTokens = metrics.OutputTokens
	response.CostUSD = metrics.CostUSD
	response.Duration = metrics.Duration
	response.TelemetryPresence = metrics.TelemetryPresence

	output, hasStructuredOutput := resultRaw["structured_output"]
	var canonical []byte
	if hasStructuredOutput {
		canonical, err = validateWorkflowOutput(output, requestJSONSchema)
		if err != nil {
			return response, fmt.Errorf("workflow structured_output does not match request JSON schema: %w", err)
		}
	} else {
		var fallbackErr error
		extracted, extractErr := extractWorkflowAssistantJSON(assistantText)
		if extractErr != nil {
			fallbackErr = fmt.Errorf(
				"workflow result structured_output is absent and assistant fallback is invalid: %w",
				extractErr,
			)
		} else {
			canonical, err = validateWorkflowOutput(extracted, requestJSONSchema)
			if err != nil {
				fallbackErr = fmt.Errorf(
					"workflow assistant fallback does not match request JSON schema: %w",
					err,
				)
				canonical = nil
			}
		}
		if len(canonical) == 0 {
			switch agent {
			case "code-reviewer":
				canonical, err = synthesizeCodeReviewerNativeOutput(assistantText, requestJSONSchema)
			case "documenter":
				canonical, err = synthesizeDocumenterNativeOutput(assistantText, requestJSONSchema)
			default:
				return response, fallbackErr
			}
			if err != nil {
				if fallbackErr == nil {
					fallbackErr = fmt.Errorf("workflow native fallback is invalid: %w", err)
				} else {
					fallbackErr = fmt.Errorf("%v; workflow native fallback is invalid: %w", fallbackErr, err)
				}
				return response, fallbackErr
			}
		}
	}
	response.Output = string(canonical)

	isError, err := optionalBool(resultRaw, "is_error")
	if err != nil {
		return response, err
	}
	providerErrors, err := optionalErrors(resultRaw)
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

func synthesizeCodeReviewerNativeOutput(texts []string, requestJSONSchema string) ([]byte, error) {
	report, err := nativeWorkflowReport(texts)
	if err != nil {
		return nil, err
	}
	decision, err := parseNativeCodeReviewerVerdict(report)
	if err != nil {
		return nil, err
	}

	findings := []string{}
	if decision == "BLOCK" {
		findings = []string{report}
	}
	raw, err := json.Marshal(map[string]any{
		"decision": decision,
		"reason":   report,
		"findings": findings,
	})
	if err != nil {
		return nil, fmt.Errorf("encode native code-reviewer output: %w", err)
	}
	canonical, err := validateWorkflowOutput(raw, requestJSONSchema)
	if err != nil {
		return nil, fmt.Errorf("native code-reviewer output does not match request JSON schema: %w", err)
	}
	return canonical, nil
}

func synthesizeDocumenterNativeOutput(texts []string, requestJSONSchema string) ([]byte, error) {
	report, err := nativeWorkflowReport(texts)
	if err != nil {
		return nil, err
	}
	path, err := extractDocumenterNativePath(report)
	if err != nil {
		return nil, err
	}
	if err := validateDocumenterNativeCompletion(report); err != nil {
		return nil, err
	}

	raw, err := json.Marshal(map[string]any{
		"status":  "done",
		"path":    path,
		"summary": report,
	})
	if err != nil {
		return nil, fmt.Errorf("encode native documenter output: %w", err)
	}
	canonical, err := validateWorkflowOutput(raw, requestJSONSchema)
	if err != nil {
		return nil, fmt.Errorf("native documenter output does not match request JSON schema: %w", err)
	}
	return canonical, nil
}

func nativeWorkflowReport(texts []string) (string, error) {
	if len(texts) == 0 {
		return "", fmt.Errorf("no top-level assistant text events")
	}
	report := strings.TrimSpace(strings.Join(texts, "\n"))
	if report == "" {
		return "", fmt.Errorf("top-level assistant text is empty")
	}
	return report, nil
}

func parseNativeCodeReviewerVerdict(report string) (string, error) {
	var decision string
	inFence := false
	for _, line := range strings.Split(report, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		candidate, ok := parseNativeCodeReviewerVerdictLine(trimmed)
		if !ok {
			continue
		}
		if decision != "" {
			return "", fmt.Errorf("native code-reviewer report contains multiple verdict lines")
		}
		decision = candidate
	}
	if decision == "" {
		return "", fmt.Errorf("native code-reviewer report must contain exactly one verdict line")
	}
	return decision, nil
}

func parseNativeCodeReviewerVerdictLine(line string) (string, bool) {
	if line == "" {
		return "", false
	}
	for _, marker := range []string{"**", "__"} {
		if strings.HasPrefix(line, marker) {
			line = strings.TrimSpace(line[len(marker):])
			break
		}
	}

	for _, decision := range []string{"APPROVE", "BLOCK"} {
		if !strings.HasPrefix(line, decision) {
			continue
		}
		rest := line[len(decision):]
		for _, marker := range []string{"**", "__"} {
			if strings.HasPrefix(rest, marker) {
				rest = rest[len(marker):]
				break
			}
		}
		rest = strings.TrimSpace(rest)
		if rest == "" || strings.HasPrefix(rest, "—") ||
			strings.HasPrefix(rest, "-") || strings.HasPrefix(rest, ":") {
			return decision, true
		}
		return "", false
	}
	return "", false
}

func extractDocumenterNativePath(report string) (string, error) {
	const marker = "app_docs/"
	var paths []string
	for offset := 0; offset < len(report); {
		index := strings.Index(report[offset:], marker)
		if index < 0 {
			break
		}
		index += offset
		start := index
		for start > 0 && isNativeDocumenterPathChar(report[start-1]) {
			start--
		}
		end := index
		for end < len(report) && isNativeDocumenterPathChar(report[end]) {
			end++
		}
		token := report[start:end]
		pathEnd := strings.LastIndex(token, ".md")
		if pathEnd < 0 {
			return "", fmt.Errorf("native documenter report contains an unsafe app_docs path")
		}
		pathEnd += len(".md")
		for _, suffix := range token[pathEnd:] {
			if !strings.ContainsRune(".,;:!?)]}", suffix) {
				return "", fmt.Errorf("native documenter report contains an unsafe app_docs path")
			}
		}
		path := token[:pathEnd]
		if err := validateNativeDocumenterPath(path); err != nil {
			return "", err
		}
		paths = append(paths, path)
		offset = index + len(marker)
	}
	if len(paths) != 1 {
		return "", fmt.Errorf("native documenter report must contain exactly one safe app_docs/*.md path")
	}
	return paths[0], nil
}

func isNativeDocumenterPathChar(character byte) bool {
	return character == '/' || character == '\\' || character == '.' ||
		character == '-' || character == '_' ||
		(character >= 'a' && character <= 'z') ||
		(character >= 'A' && character <= 'Z') ||
		(character >= '0' && character <= '9')
}

func validateNativeDocumenterPath(path string) error {
	if !strings.HasPrefix(path, "app_docs/") || !strings.HasSuffix(path, ".md") ||
		strings.Contains(path, `\`) {
		return fmt.Errorf("native documenter path %q is not a safe relative app_docs Markdown path", path)
	}
	components := strings.Split(path, "/")
	for _, component := range components {
		if component == "" || component == "." || component == ".." {
			return fmt.Errorf("native documenter path %q contains an unsafe component", path)
		}
	}
	return nil
}

func validateDocumenterNativeCompletion(report string) error {
	lower := strings.ToLower(report)
	for _, phrase := range []string{
		"could not",
		"couldn't",
		"cannot",
		"can't",
		"not completed",
		"not complete",
		"not done",
		"not written",
		"did not",
	} {
		if strings.Contains(lower, phrase) {
			return fmt.Errorf("native documenter report contains failure language")
		}
	}
	for _, word := range []string{
		"blocked",
		"blocker",
		"failed",
		"failure",
		"unable",
		"error",
		"aborted",
		"incomplete",
		"pending",
		"skipped",
	} {
		if containsNativeWorkflowWord(lower, word) {
			return fmt.Errorf("native documenter report contains failure language")
		}
	}
	for _, word := range []string{
		"done",
		"complete",
		"completed",
		"finished",
		"created",
		"wrote",
		"written",
		"documented",
		"saved",
	} {
		if containsNativeWorkflowWord(lower, word) {
			return nil
		}
	}
	return fmt.Errorf("native documenter report must contain an unambiguous completion statement")
}

func containsNativeWorkflowWord(text, word string) bool {
	for offset := 0; offset < len(text); {
		index := strings.Index(text[offset:], word)
		if index < 0 {
			return false
		}
		index += offset
		beforeOK := index == 0 || !isNativeWorkflowWordChar(text[index-1])
		after := index + len(word)
		afterOK := after == len(text) || !isNativeWorkflowWordChar(text[after])
		if beforeOK && afterOK {
			return true
		}
		offset = index + len(word)
	}
	return false
}

func isNativeWorkflowWordChar(character byte) bool {
	return (character >= 'a' && character <= 'z') ||
		(character >= 'A' && character <= 'Z') ||
		(character >= '0' && character <= '9') || character == '_'
}

func workflowTopLevelAssistantText(raw map[string]json.RawMessage) []string {
	parentToolUseID, hasParent := raw["parent_tool_use_id"]
	if hasParent && !bytes.Equal(bytes.TrimSpace(parentToolUseID), []byte("null")) {
		return nil
	}

	messageRaw, ok := raw["message"]
	if !ok {
		return nil
	}
	var message struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(messageRaw, &message); err != nil {
		return nil
	}

	text := make([]string, 0, len(message.Content))
	for _, block := range message.Content {
		if block.Type == "text" {
			text = append(text, block.Text)
		}
	}
	return text
}

func extractWorkflowAssistantJSON(texts []string) ([]byte, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("no top-level assistant text events")
	}
	text := strings.TrimSpace(strings.Join(texts, "\n"))
	if text == "" {
		return nil, fmt.Errorf("top-level assistant text is empty")
	}

	candidate, err := extractWorkflowJSONObject(text)
	if err != nil {
		return nil, err
	}
	if _, err := decodeWorkflowJSONObject(candidate); err != nil {
		return nil, err
	}
	return candidate, nil
}

func extractWorkflowJSONObject(text string) ([]byte, error) {
	start := -1
	end := -1
	depth := 0
	inString := false
	escaped := false

	for index := 0; index < len(text); index++ {
		character := text[index]
		if depth > 0 {
			if inString {
				if escaped {
					escaped = false
					continue
				}
				if character == '\\' {
					escaped = true
					continue
				}
				if character == '"' {
					inString = false
				}
				continue
			}

			switch character {
			case '"':
				inString = true
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					end = index + 1
				}
			case '[':
				// Arrays are valid inside the candidate object, but not around
				// or alongside it. This keeps an array-wrapped object from
				// being mistaken for the required top-level object.
			}
			continue
		}

		switch character {
		case '{':
			if start >= 0 {
				return nil, fmt.Errorf("assistant text contains multiple JSON objects")
			}
			start = index
			depth = 1
		case '}':
			return nil, fmt.Errorf("assistant text contains an unmatched closing brace")
		case '[':
			return nil, fmt.Errorf("assistant text contains an array outside the JSON object")
		case ']':
			return nil, fmt.Errorf("assistant text contains an unmatched closing bracket")
		}
	}

	if start < 0 {
		return nil, fmt.Errorf("assistant text must contain exactly one JSON object")
	}
	if depth != 0 {
		if inString {
			return nil, fmt.Errorf("assistant text contains an unterminated JSON string")
		}
		return nil, fmt.Errorf("assistant text contains unmatched opening braces")
	}
	if end <= start {
		return nil, fmt.Errorf("assistant text must contain exactly one JSON object")
	}

	for index := end; index < len(text); index++ {
		switch text[index] {
		case '{':
			return nil, fmt.Errorf("assistant text contains multiple JSON objects")
		case '}':
			return nil, fmt.Errorf("assistant text contains an unmatched closing brace")
		case '[':
			return nil, fmt.Errorf("assistant text contains an array outside the JSON object")
		case ']':
			return nil, fmt.Errorf("assistant text contains an unmatched closing bracket")
		}
	}

	return []byte(text[start:end]), nil
}

func decodeWorkflowJSONObject(raw []byte) (any, error) {
	value, err := decodeSingleJSONValue(raw)
	if err != nil {
		return nil, fmt.Errorf("assistant text must contain exactly one JSON object: %w", err)
	}
	if object, ok := value.(map[string]any); !ok || object == nil {
		return nil, fmt.Errorf("assistant text JSON must be an object")
	}
	return value, nil
}

func decodeSingleJSONValue(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("multiple JSON values")
		}
		return nil, err
	}
	return value, nil
}

type workflowJSONSchema struct {
	typeName             string
	hasType              bool
	hasAdditionalProps   bool
	additionalProperties bool
	hasRequired          bool
	required             []string
	hasProperties        bool
	properties           map[string]*workflowJSONSchema
	hasConst             bool
	constValue           []byte
	hasEnum              bool
	enumValues           [][]byte
	minLength            *int
	minItems             *int
	hasItems             bool
	items                *workflowJSONSchema
}

func validateWorkflowOutput(raw []byte, requestJSONSchema string) ([]byte, error) {
	schema, err := parseWorkflowJSONSchema([]byte(requestJSONSchema))
	if err != nil {
		return nil, err
	}
	if !schema.hasType || schema.typeName != "object" {
		return nil, fmt.Errorf("request JSON schema root must have type object")
	}

	value, err := decodeSingleJSONValue(raw)
	if err != nil {
		return nil, fmt.Errorf("output must be valid JSON: %w", err)
	}
	if err := validateWorkflowJSONValue(value, schema, "$"); err != nil {
		return nil, err
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("canonicalize output: %w", err)
	}
	return canonicalJSONBytes(canonical)
}

func parseWorkflowJSONSchema(raw []byte) (*workflowJSONSchema, error) {
	if strings.TrimSpace(string(raw)) == "" {
		return nil, fmt.Errorf("request JSON schema is required")
	}
	object, err := decodeJSONSchemaObject(raw, "$")
	if err != nil {
		return nil, err
	}
	return parseWorkflowJSONSchemaNode(object, "$")
}

func decodeJSONSchemaObject(raw []byte, path string) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var object map[string]json.RawMessage
	if err := decoder.Decode(&object); err != nil {
		return nil, fmt.Errorf("invalid JSON schema at %s: %w", path, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("invalid JSON schema at %s: multiple values", path)
		}
		return nil, fmt.Errorf("invalid JSON schema at %s: %w", path, err)
	}
	if object == nil {
		return nil, fmt.Errorf("invalid JSON schema at %s: schema node must be an object", path)
	}
	return object, nil
}

func parseWorkflowJSONSchemaNode(raw map[string]json.RawMessage, path string) (*workflowJSONSchema, error) {
	const (
		typeKey                 = "type"
		additionalPropertiesKey = "additionalProperties"
		requiredKey             = "required"
		propertiesKey           = "properties"
		constKey                = "const"
		enumKey                 = "enum"
		minLengthKey            = "minLength"
		minItemsKey             = "minItems"
		itemsKey                = "items"
	)
	for key := range raw {
		switch key {
		case typeKey, additionalPropertiesKey, requiredKey, propertiesKey, constKey, enumKey, minLengthKey, minItemsKey, itemsKey:
		default:
			return nil, fmt.Errorf("unsupported JSON schema keyword %q at %s", key, path)
		}
	}

	schema := &workflowJSONSchema{}
	if value, ok := raw[typeKey]; ok {
		var typeName *string
		if err := json.Unmarshal(value, &typeName); err != nil || typeName == nil {
			return nil, fmt.Errorf("JSON schema type at %s must be a string", path)
		}
		switch *typeName {
		case "object", "string", "array":
			schema.typeName = *typeName
			schema.hasType = true
		default:
			return nil, fmt.Errorf("unsupported JSON schema type %q at %s", *typeName, path)
		}
	}
	if value, ok := raw[additionalPropertiesKey]; ok {
		var additionalProperties *bool
		if err := json.Unmarshal(value, &additionalProperties); err != nil || additionalProperties == nil {
			return nil, fmt.Errorf("additionalProperties at %s must be false", path)
		}
		if *additionalProperties {
			return nil, fmt.Errorf("additionalProperties at %s must be false", path)
		}
		schema.hasAdditionalProps = true
		schema.additionalProperties = false
	}
	if value, ok := raw[requiredKey]; ok {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, fmt.Errorf("required at %s must be an array of unique strings", path)
		}
		var required []string
		if err := json.Unmarshal(value, &required); err != nil {
			return nil, fmt.Errorf("required at %s must be an array of unique strings", path)
		}
		seen := make(map[string]struct{}, len(required))
		for _, name := range required {
			if _, ok := seen[name]; ok {
				return nil, fmt.Errorf("required at %s contains duplicate property %q", path, name)
			}
			seen[name] = struct{}{}
		}
		schema.hasRequired = true
		schema.required = required
	}
	if value, ok := raw[propertiesKey]; ok {
		properties, err := decodeJSONSchemaObject(value, path+".properties")
		if err != nil {
			return nil, err
		}
		schema.hasProperties = true
		schema.properties = make(map[string]*workflowJSONSchema, len(properties))
		for name, propertyRaw := range properties {
			property, err := decodeJSONSchemaObject(propertyRaw, path+".properties."+name)
			if err != nil {
				return nil, err
			}
			parsed, err := parseWorkflowJSONSchemaNode(property, path+".properties."+name)
			if err != nil {
				return nil, err
			}
			schema.properties[name] = parsed
		}
	}
	if value, ok := raw[constKey]; ok {
		canonical, err := canonicalJSONBytes(value)
		if err != nil {
			return nil, fmt.Errorf("const at %s must be valid JSON: %w", path, err)
		}
		schema.hasConst = true
		schema.constValue = canonical
	}
	if value, ok := raw[enumKey]; ok {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, fmt.Errorf("enum at %s must be a non-empty array", path)
		}
		var values []json.RawMessage
		if err := json.Unmarshal(value, &values); err != nil || values == nil || len(values) == 0 {
			return nil, fmt.Errorf("enum at %s must be a non-empty array", path)
		}
		seen := make(map[string]struct{}, len(values))
		schema.enumValues = make([][]byte, 0, len(values))
		for _, item := range values {
			canonical, err := canonicalJSONBytes(item)
			if err != nil {
				return nil, fmt.Errorf("enum at %s contains invalid JSON: %w", path, err)
			}
			key := string(canonical)
			if _, ok := seen[key]; ok {
				return nil, fmt.Errorf("enum at %s contains duplicate values", path)
			}
			seen[key] = struct{}{}
			schema.enumValues = append(schema.enumValues, canonical)
		}
		schema.hasEnum = true
	}
	if value, ok := raw[minLengthKey]; ok {
		minLength, err := workflowSchemaNonNegativeInt(value, path+".minLength")
		if err != nil {
			return nil, err
		}
		schema.minLength = &minLength
	}
	if value, ok := raw[minItemsKey]; ok {
		minItems, err := workflowSchemaNonNegativeInt(value, path+".minItems")
		if err != nil {
			return nil, err
		}
		schema.minItems = &minItems
	}
	if value, ok := raw[itemsKey]; ok {
		itemRaw, err := decodeJSONSchemaObject(value, path+".items")
		if err != nil {
			return nil, err
		}
		items, err := parseWorkflowJSONSchemaNode(itemRaw, path+".items")
		if err != nil {
			return nil, err
		}
		schema.hasItems = true
		schema.items = items
	}

	if !schema.hasType && !schema.hasConst && !schema.hasEnum {
		return nil, fmt.Errorf("JSON schema node at %s must define type, const, or enum", path)
	}
	switch schema.typeName {
	case "object":
		if !schema.hasAdditionalProps || schema.additionalProperties {
			return nil, fmt.Errorf("object schema at %s must set additionalProperties to false", path)
		}
		if !schema.hasProperties {
			return nil, fmt.Errorf("object schema at %s must define properties", path)
		}
		if schema.minLength != nil || schema.minItems != nil || schema.hasItems {
			return nil, fmt.Errorf("object schema at %s contains string or array keywords", path)
		}
	case "string":
		if schema.hasAdditionalProps || schema.hasRequired || schema.hasProperties || schema.minItems != nil || schema.hasItems {
			return nil, fmt.Errorf("string schema at %s contains object or array keywords", path)
		}
	case "array":
		if !schema.hasItems {
			return nil, fmt.Errorf("array schema at %s must define items", path)
		}
		if schema.hasAdditionalProps || schema.hasRequired || schema.hasProperties || schema.minLength != nil {
			return nil, fmt.Errorf("array schema at %s contains object or string keywords", path)
		}
	case "":
		if schema.hasAdditionalProps || schema.hasRequired || schema.hasProperties ||
			schema.minLength != nil || schema.minItems != nil || schema.hasItems {
			return nil, fmt.Errorf("untyped JSON schema node at %s contains type-specific keywords", path)
		}
	}
	if schema.hasProperties {
		for _, name := range schema.required {
			if _, ok := schema.properties[name]; !ok {
				return nil, fmt.Errorf("required property %q at %s is not defined", name, path)
			}
		}
	}
	return schema, nil
}

func workflowSchemaNonNegativeInt(raw json.RawMessage, path string) (int, error) {
	value, err := decodeSingleJSONValue(raw)
	if err != nil {
		return 0, fmt.Errorf("schema integer at %s is invalid: %w", path, err)
	}
	number, ok := value.(json.Number)
	if !ok {
		return 0, fmt.Errorf("schema integer at %s must be a non-negative integer", path)
	}
	parsed, err := strconv.ParseUint(string(number), 10, 64)
	if err != nil || parsed > uint64(maxInt) {
		return 0, fmt.Errorf("schema integer at %s must be a non-negative integer", path)
	}
	return int(parsed), nil
}

func validateWorkflowJSONValue(value any, schema *workflowJSONSchema, path string) error {
	switch schema.typeName {
	case "object":
		object, ok := value.(map[string]any)
		if !ok || object == nil {
			return fmt.Errorf("output at %s must be an object", path)
		}
		for _, name := range schema.required {
			if _, ok := object[name]; !ok {
				return fmt.Errorf("output at %s is missing required property %q", path, name)
			}
		}
		for name := range object {
			property, ok := schema.properties[name]
			if !ok {
				return fmt.Errorf("output at %s contains additional property %q", path, name)
			}
			if err := validateWorkflowJSONValue(propertyValue(object, name), property, path+"."+name); err != nil {
				return err
			}
		}
	case "string":
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("output at %s must be a string", path)
		}
		if schema.minLength != nil && utf8.RuneCountInString(text) < *schema.minLength {
			return fmt.Errorf("output at %s is shorter than minLength %d", path, *schema.minLength)
		}
	case "array":
		items, ok := value.([]any)
		if !ok {
			return fmt.Errorf("output at %s must be an array", path)
		}
		if schema.minItems != nil && len(items) < *schema.minItems {
			return fmt.Errorf("output at %s contains fewer than minItems %d", path, *schema.minItems)
		}
		for index, item := range items {
			if err := validateWorkflowJSONValue(item, schema.items, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	}

	canonical, err := canonicalWorkflowValue(value)
	if err != nil {
		return fmt.Errorf("canonicalize output at %s: %w", path, err)
	}
	if schema.hasConst && !bytes.Equal(canonical, schema.constValue) {
		return fmt.Errorf("output at %s does not match const", path)
	}
	if schema.hasEnum {
		matched := false
		for _, allowed := range schema.enumValues {
			if bytes.Equal(canonical, allowed) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("output at %s does not match enum", path)
		}
	}
	return nil
}

func propertyValue(object map[string]any, name string) any {
	return object[name]
}

func canonicalWorkflowValue(value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return canonicalJSONBytes(encoded)
}

func responseMetrics(raw map[string]json.RawMessage) (ProviderResponse, error) {
	var response ProviderResponse
	var err error
	response.ProviderRequestID, err = optionalString(raw, "request_id")
	if err != nil {
		return response, err
	}
	_, response.TelemetryPresence.ProviderRequestID = raw["request_id"]
	response.ProviderResponseID, err = optionalString(raw, "uuid")
	if err != nil {
		return response, err
	}
	_, response.TelemetryPresence.ProviderResponseID = raw["uuid"]
	response.ProviderSessionID, err = optionalString(raw, "session_id")
	if err != nil {
		return response, err
	}
	_, response.TelemetryPresence.ProviderSessionID = raw["session_id"]
	if value, ok := raw["modelUsage"]; ok {
		response.TelemetryPresence.ModelUsage = true
		var usage map[string]map[string]json.RawMessage
		if err := json.Unmarshal(value, &usage); err != nil {
			return response, fmt.Errorf("response modelUsage must be an object: %w", err)
		}
		response.ProviderModelUsage = usage
		response.ProviderModels = make([]string, 0, len(usage))
		inputPresent := len(usage) > 0
		outputPresent := len(usage) > 0
		for model, totals := range usage {
			response.ProviderModels = append(response.ProviderModels, model)
			if _, ok := totals["inputTokens"]; !ok {
				inputPresent = false
			}
			if _, ok := totals["outputTokens"]; !ok {
				outputPresent = false
			}
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
		response.TelemetryPresence.InputTokens = inputPresent
		response.TelemetryPresence.OutputTokens = outputPresent
		sort.Strings(response.ProviderModels)
	}
	if value, ok := raw["total_cost_usd"]; ok {
		response.TelemetryPresence.CostUSD = true
		cost, err := nonNegativeFloat(value, "response total_cost_usd")
		if err != nil {
			return response, err
		}
		response.CostUSD = cost
	}
	if value, ok := raw["duration_ms"]; ok {
		response.TelemetryPresence.DurationMS = true
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

func optionalString(raw map[string]json.RawMessage, key string) (string, error) {
	value, ok := raw[key]
	if !ok {
		return "", nil
	}
	var result string
	if err := json.Unmarshal(value, &result); err != nil {
		return "", fmt.Errorf("response %s must be a string: %w", key, err)
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
