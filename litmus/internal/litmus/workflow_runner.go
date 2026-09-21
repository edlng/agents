package litmus

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type MeasuredWorkflowRunner struct {
	Root             string
	Executor         Executor
	Now              func() time.Time
	prepareWorkspace func(context.Context, string, string) error
}

type measuredStepOutput struct {
	Status       string
	Decision     string
	Summary      string
	Files        []string
	Verification string
	Reason       string
	Issues       []string
	Findings     []string
	Path         string
}

func (runner MeasuredWorkflowRunner) Run(
	ctx context.Context,
	testCase Case,
	config MeasuredWorkflowConfig,
	runBudget float64,
	spent float64,
) (CaseResult, error) {
	if err := validateMeasuredWorkflowConfig(config); err != nil {
		return CaseResult{}, err
	}
	if err := validateMeasuredWorkflowCase(testCase); err != nil {
		return CaseResult{}, err
	}
	if !isFinite(runBudget) || runBudget <= 0 || !isFinite(spent) || spent < 0 {
		return CaseResult{}, fmt.Errorf("measured workflow budget must be finite and positive")
	}
	if runBudget-spent <= 0 {
		return CaseResult{}, fmt.Errorf("no measured workflow budget remains")
	}

	workspace, cleanup, err := copyFixture(runner.Root, testCase.Fixture)
	if err != nil {
		return CaseResult{}, err
	}
	defer cleanup()
	prepareWorkspace := runner.prepareWorkspace
	if prepareWorkspace == nil {
		prepareWorkspace = prepareWorkflowWorkspace
	}
	if err := prepareWorkspace(ctx, runner.Root, workspace); err != nil {
		return CaseResult{}, err
	}
	protectedDefinitionHash, err := protectedWorkflowDefinitionHash(workspace)
	if err != nil {
		return CaseResult{}, err
	}
	evidenceRoot, err := os.MkdirTemp("", "litmus-workflow-evidence-")
	if err != nil {
		return CaseResult{}, fmt.Errorf("create measured workflow evidence: %w", err)
	}
	defer os.RemoveAll(evidenceRoot)

	now := runner.Now
	if now == nil {
		now = time.Now
	}
	runID := now().UTC().Format("20060102T150405.000Z") + "-" + testCase.ID
	store, err := newWorkflowEvidenceStore(
		evidenceRoot,
		config.WorkflowID,
		runID,
		testCase.ID,
	)
	if err != nil {
		return CaseResult{}, err
	}
	executor := runner.Executor
	if executor == nil {
		executor = claudeExecutor{}
	}

	workflow := &WorkflowResult{
		SchemaVersion:     MeasuredWorkflowSchemaVersion,
		WorkflowID:        config.WorkflowID,
		RunID:             runID,
		CaseID:            testCase.ID,
		Status:            WorkflowPartial,
		MaxAttempts:       config.MaxAttempts,
		Steps:             []WorkflowStepResult{},
		Audit:             []WorkflowAuditRecord{},
		Sentinels:         []WorkflowSentinel{},
		AuditPath:         filepath.ToSlash(filepath.Join("workflow", testCase.ID, "audit.jsonl")),
		SentinelDirectory: filepath.ToSlash(filepath.Join("workflow", testCase.ID, "sentinels")),
	}
	result := CaseResult{
		Agent:    testCase.Agent,
		CaseID:   testCase.ID,
		Status:   StatusAgentFailure,
		Workflow: workflow,
	}
	var lastFailure string

	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		workflow.FinalAttempt = attempt
		var predecessor *WorkflowSentinel
		var priorOutputs []WorkflowStepResult
		retry := false
		for _, stepConfig := range config.Steps {
			remaining := runBudget - spent - workflow.Totals.CostUSD
			if remaining+1e-9 < stepConfig.BudgetUSD {
				return finishMeasuredWorkflow(
					ctx,
					workspace,
					testCase,
					result,
					store,
					WorkflowBlocked,
					"BUDGET_EXHAUSTED",
					stepConfig.ID,
					attempt,
					fmt.Sprintf(
						"step %s requires $%.2f but only $%.8f remains",
						stepConfig.ID,
						stepConfig.BudgetUSD,
						remaining,
					),
				)
			}
			if predecessor != nil {
				workspaceHash, hashErr := workflowWorkspaceHash(ctx, workspace)
				if hashErr != nil {
					return finishMeasuredWorkflow(
						ctx,
						workspace,
						testCase,
						result,
						store,
						WorkflowBlocked,
						"WORKSPACE_HASH_FAILED",
						stepConfig.ID,
						attempt,
						hashErr.Error(),
					)
				}
				if err := store.requireSentinel(*predecessor, workspaceHash); err != nil {
					return finishMeasuredWorkflow(
						ctx,
						workspace,
						testCase,
						result,
						store,
						WorkflowBlocked,
						"SENTINEL_MISMATCH",
						stepConfig.ID,
						attempt,
						err.Error(),
					)
				}
			}

			step, parsed, stepErr := runner.invokeMeasuredStep(
				ctx,
				executor,
				workspace,
				testCase,
				stepConfig,
				attempt,
				predecessor,
				priorOutputs,
				lastFailure,
				protectedDefinitionHash,
				len(workflow.Steps)+1,
			)
			if stepErr != nil && step.StepID == "" {
				return finishMeasuredWorkflow(
					ctx,
					workspace,
					testCase,
					result,
					store,
					WorkflowBlocked,
					measuredStepFailureCode(stepErr),
					stepConfig.ID,
					attempt,
					stepErr.Error(),
				)
			}
			workflow.Steps = append(workflow.Steps, step)
			addWorkflowTotals(&workflow.Totals, step)
			record, auditErr := store.append(WorkflowAuditRecord{
				Timestamp:              now().UTC().Format(time.RFC3339Nano),
				StepID:                 step.StepID,
				Agent:                  step.Agent,
				Attempt:                step.Attempt,
				Status:                 step.Status,
				Decision:               step.Decision,
				InputHash:              step.InputHash,
				OutputHash:             step.OutputHash,
				WorkspaceHash:          step.WorkspaceHash,
				ProviderInvocationHash: step.ProviderInvocationHash,
				Error:                  step.Error,
			})
			if auditErr != nil {
				return finishMeasuredWorkflow(
					ctx,
					workspace,
					testCase,
					result,
					store,
					WorkflowBlocked,
					"AUDIT_WRITE_FAILED",
					stepConfig.ID,
					attempt,
					auditErr.Error(),
				)
			}
			if stepErr != nil {
				return finishMeasuredWorkflow(
					ctx,
					workspace,
					testCase,
					result,
					store,
					WorkflowBlocked,
					measuredStepFailureCode(stepErr),
					stepConfig.ID,
					attempt,
					stepErr.Error(),
				)
			}

			if shouldRetryStep(stepConfig.ID, parsed.Decision) {
				lastFailure = fmt.Sprintf("%s returned %s: %s", stepConfig.ID, parsed.Decision, parsed.Reason)
				retry = true
				break
			}
			sentinel, sentinelErr := store.writeSentinel(step, predecessor, now().UTC().Format(time.RFC3339Nano))
			if sentinelErr != nil {
				return finishMeasuredWorkflow(
					ctx,
					workspace,
					testCase,
					result,
					store,
					WorkflowBlocked,
					"SENTINEL_WRITE_FAILED",
					stepConfig.ID,
					attempt,
					sentinelErr.Error(),
				)
			}
			workflow.Steps[len(workflow.Steps)-1].SentinelPath = filepath.ToSlash(
				filepath.Join(workflow.SentinelDirectory, filepath.Base(store.sentinelPath(step.StepID, attempt))),
			)
			predecessor = &sentinel
			priorOutputs = append(priorOutputs, step)
			_ = record
		}
		if !retry {
			return finishMeasuredWorkflow(
				ctx,
				workspace,
				testCase,
				result,
				store,
				WorkflowPendingHuman,
				"",
				"",
				attempt,
				"",
			)
		}
	}

	return finishMeasuredWorkflow(
		ctx,
		workspace,
		testCase,
		result,
		store,
		WorkflowBlocked,
		"MAX_ATTEMPTS_EXCEEDED",
		retryOrigin(lastFailure),
		config.MaxAttempts,
		lastFailure,
	)
}

func validateMeasuredWorkflowCase(testCase Case) error {
	if err := validateComponent("case id", testCase.ID); err != nil {
		return err
	}
	if testCase.Agent != "team-lead" {
		return fmt.Errorf("measured Stage 4 workflow requires a team-lead case")
	}
	if !testCase.Live || !testCase.Workflow {
		return fmt.Errorf("case %s must enable live workflow execution", testCase.ID)
	}
	if strings.TrimSpace(testCase.Task) == "" || len(testCase.Assertions) == 0 {
		return fmt.Errorf("measured workflow case requires a task and assertions")
	}
	return nil
}

func (runner MeasuredWorkflowRunner) invokeMeasuredStep(
	ctx context.Context,
	executor Executor,
	workspace string,
	testCase Case,
	config MeasuredWorkflowStepConfig,
	attempt int,
	predecessor *WorkflowSentinel,
	prior []WorkflowStepResult,
	retryContext string,
	protectedDefinitionHash string,
	sequence int,
) (WorkflowStepResult, measuredStepOutput, error) {
	prompt, model, err := resolveProductionAgent(runner.Root, config.Agent)
	if err != nil {
		return WorkflowStepResult{}, measuredStepOutput{}, err
	}
	currentDefinitionHash, err := protectedWorkflowDefinitionHash(workspace)
	if err != nil {
		return WorkflowStepResult{}, measuredStepOutput{}, err
	}
	if currentDefinitionHash != protectedDefinitionHash {
		return WorkflowStepResult{}, measuredStepOutput{}, fmt.Errorf(
			"protected workflow agent definitions changed before %s",
			config.ID,
		)
	}
	beforeHash, err := workflowWorkspaceHash(ctx, workspace)
	if err != nil {
		return WorkflowStepResult{}, measuredStepOutput{}, err
	}
	input := measuredStepTask(testCase, config, attempt, predecessor, prior, retryContext)
	inputHash := hashWorkflowText(input)
	request := ProviderRequest{
		Agent:        config.Agent,
		Task:         input,
		SystemPrompt: prompt,
		Model:        model,
		BudgetUSD:    config.BudgetUSD,
		Workspace:    workspace,
		AllowTools:   true,
		Workflow:     true,
		JSONSchema:   string(config.JSONSchema),
	}
	response, providerErr := executor.Execute(ctx, request)
	afterHash, hashErr := workflowWorkspaceHash(ctx, workspace)
	if hashErr != nil {
		afterHash = beforeHash
	}
	outputHash := hashWorkflowText(response.Output)
	step := WorkflowStepResult{
		Sequence:           sequence,
		StepID:             config.ID,
		Agent:              config.Agent,
		Attempt:            attempt,
		Status:             "BLOCKED",
		BudgetUSD:          config.BudgetUSD,
		RequestedModel:     providerModel(model),
		ActualModels:       response.ProviderModels,
		ProviderRequestID:  response.ProviderRequestID,
		ProviderResponseID: response.ProviderResponseID,
		ProviderSessionID:  response.ProviderSessionID,
		ProviderModelUsage: response.ProviderModelUsage,
		TelemetryPresence:  response.TelemetryPresence,
		InputTokens:        response.InputTokens,
		OutputTokens:       response.OutputTokens,
		CostUSD:            response.CostUSD,
		DurationMS:         response.Duration.Milliseconds(),
		InputHash:          inputHash,
		OutputHash:         outputHash,
		WorkspaceHash:      afterHash,
		Output:             response.Output,
	}
	step.ProviderInvocationHash, err = providerInvocationHash(step)
	if err != nil {
		step.Error = err.Error()
		return step, measuredStepOutput{}, err
	}
	if hashErr != nil {
		step.Error = hashErr.Error()
		return step, measuredStepOutput{}, hashErr
	}
	if err := validateMeasuredTelemetry(response, config.BudgetUSD, false); err != nil {
		step.Error = err.Error()
		return step, measuredStepOutput{}, err
	}
	if providerErr != nil {
		step.Error = providerErr.Error()
		return step, measuredStepOutput{}, fmt.Errorf("execute %s provider: %w", config.ID, providerErr)
	}
	currentDefinitionHash, err = protectedWorkflowDefinitionHash(workspace)
	if err != nil {
		step.Error = err.Error()
		return step, measuredStepOutput{}, err
	}
	if currentDefinitionHash != protectedDefinitionHash {
		step.Error = "protected workflow agent definitions changed"
		return step, measuredStepOutput{}, errors.New(step.Error)
	}
	if !config.AllowWorkspaceChanges && beforeHash != afterHash {
		step.Error = "read-only workflow step changed the workspace"
		return step, measuredStepOutput{}, errors.New(step.Error)
	}
	if err := validateMeasuredTelemetry(response, config.BudgetUSD, true); err != nil {
		step.Error = err.Error()
		return step, measuredStepOutput{}, err
	}
	parsed, err := decodeMeasuredStepOutput(config.ID, response.Output)
	if err != nil {
		step.Error = err.Error()
		return step, measuredStepOutput{}, err
	}
	if config.ID == "documenter" {
		if err := validateMeasuredDocumenterFile(workspace, parsed.Path); err != nil {
			step.Error = err.Error()
			return step, measuredStepOutput{}, err
		}
	}
	step.Decision = parsed.Decision
	if parsed.Decision == "" {
		step.Decision = parsed.Status
	}
	step.Status = "ACCEPTED"
	if shouldRetryStep(config.ID, parsed.Decision) {
		step.Status = "RETRY"
	}
	return step, parsed, nil
}

func validateMeasuredDocumenterFile(workspace, path string) error {
	if strings.TrimSpace(workspace) == "" {
		return fmt.Errorf("documenter workspace is required")
	}
	if strings.Contains(path, `\`) {
		return fmt.Errorf("documenter path must use safe workspace-relative separators")
	}
	if err := validateRelativePath(path); err != nil {
		return fmt.Errorf("documenter path is unsafe: %w", err)
	}

	resolvedWorkspace, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return fmt.Errorf("resolve documenter workspace: %w", err)
	}
	candidate := filepath.Join(resolvedWorkspace, filepath.FromSlash(path))
	relative, err := filepath.Rel(resolvedWorkspace, candidate)
	if err != nil {
		return fmt.Errorf("check documenter path containment: %w", err)
	}
	if relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) ||
		filepath.IsAbs(relative) {
		return fmt.Errorf("documenter path escapes workspace")
	}

	resolvedPath, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return fmt.Errorf("documenter output file %q does not exist: %w", path, err)
	}
	resolvedRelative, err := filepath.Rel(resolvedWorkspace, resolvedPath)
	if err != nil {
		return fmt.Errorf("check resolved documenter path containment: %w", err)
	}
	if resolvedRelative == ".." ||
		strings.HasPrefix(resolvedRelative, ".."+string(filepath.Separator)) ||
		filepath.IsAbs(resolvedRelative) {
		return fmt.Errorf("documenter path escapes workspace")
	}

	info, err := os.Lstat(candidate)
	if err != nil {
		return fmt.Errorf("inspect documenter output file %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("documenter output path %q must be a regular file", path)
	}
	return nil
}

func measuredStepTask(
	testCase Case,
	config MeasuredWorkflowStepConfig,
	attempt int,
	predecessor *WorkflowSentinel,
	prior []WorkflowStepResult,
	retryContext string,
) string {
	evidence := make([]map[string]any, 0, len(prior))
	for _, step := range prior {
		evidence = append(evidence, map[string]any{
			"step_id":     step.StepID,
			"decision":    step.Decision,
			"output":      json.RawMessage(step.Output),
			"output_hash": step.OutputHash,
		})
	}
	contextJSON, _ := json.Marshal(evidence)
	predecessorHash := ""
	if predecessor != nil {
		predecessorHash = predecessor.SentinelHash
	}
	return fmt.Sprintf(
		"Measured Stage 4 workflow step.\n"+
			"Case: %s\nAttempt: %d\nStep: %s\nPredecessor sentinel hash: %s\n"+
			"Retry feedback: %s\nOriginal task:\n%s\n\nPrior accepted evidence JSON:\n%s\n\n"+
			"Complete only this role. Return exactly one JSON object matching the supplied schema. "+
			"Do not wrap it in markdown or add prose.",
		testCase.ID,
		attempt,
		config.ID,
		predecessorHash,
		retryContext,
		testCase.Task,
		contextJSON,
	)
}

func decodeMeasuredStepOutput(stepID, output string) (measuredStepOutput, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(output))
	decoder.DisallowUnknownFields()
	var trailing any
	switch stepID {
	case "builder":
		var value struct {
			Status       string   `json:"status"`
			Summary      string   `json:"summary"`
			Files        []string `json:"files"`
			Verification string   `json:"verification"`
		}
		if err := decoder.Decode(&value); err != nil {
			return measuredStepOutput{}, fmt.Errorf("decode builder output: %w", err)
		}
		if value.Status != "done" || strings.TrimSpace(value.Summary) == "" ||
			len(value.Files) == 0 || strings.TrimSpace(value.Verification) == "" {
			return measuredStepOutput{}, fmt.Errorf("builder output violates its contract")
		}
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			return measuredStepOutput{}, fmt.Errorf("builder output must contain one JSON object")
		}
		return measuredStepOutput{
			Status:       value.Status,
			Summary:      value.Summary,
			Files:        value.Files,
			Verification: value.Verification,
		}, nil
	case "validator", "review-challenge":
		var value struct {
			Decision string   `json:"decision"`
			Reason   string   `json:"reason"`
			Issues   []string `json:"issues"`
		}
		if err := decoder.Decode(&value); err != nil {
			return measuredStepOutput{}, fmt.Errorf("decode %s output: %w", stepID, err)
		}
		if (value.Decision != "PASS" && value.Decision != "FAIL") ||
			strings.TrimSpace(value.Reason) == "" ||
			(value.Decision == "FAIL" && len(value.Issues) == 0) {
			return measuredStepOutput{}, fmt.Errorf("%s output violates its contract", stepID)
		}
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			return measuredStepOutput{}, fmt.Errorf("%s output must contain one JSON object", stepID)
		}
		return measuredStepOutput{
			Decision: value.Decision,
			Reason:   value.Reason,
			Issues:   value.Issues,
		}, nil
	case "code-reviewer":
		var value struct {
			Decision string   `json:"decision"`
			Reason   string   `json:"reason"`
			Findings []string `json:"findings"`
		}
		if err := decoder.Decode(&value); err != nil {
			return measuredStepOutput{}, fmt.Errorf("decode code-reviewer output: %w", err)
		}
		if (value.Decision != "APPROVE" && value.Decision != "BLOCK") ||
			strings.TrimSpace(value.Reason) == "" ||
			(value.Decision == "APPROVE" && len(value.Findings) != 0) ||
			(value.Decision == "BLOCK" && len(value.Findings) == 0) {
			return measuredStepOutput{}, fmt.Errorf("code-reviewer output violates its contract")
		}
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			return measuredStepOutput{}, fmt.Errorf("code-reviewer output must contain one JSON object")
		}
		return measuredStepOutput{
			Decision: value.Decision,
			Reason:   value.Reason,
			Findings: value.Findings,
		}, nil
	case "documenter":
		var value struct {
			Status  string `json:"status"`
			Path    string `json:"path"`
			Summary string `json:"summary"`
		}
		if err := decoder.Decode(&value); err != nil {
			return measuredStepOutput{}, fmt.Errorf("decode documenter output: %w", err)
		}
		if value.Status != "done" || strings.TrimSpace(value.Path) == "" ||
			strings.TrimSpace(value.Summary) == "" {
			return measuredStepOutput{}, fmt.Errorf("documenter output violates its contract")
		}
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			return measuredStepOutput{}, fmt.Errorf("documenter output must contain one JSON object")
		}
		return measuredStepOutput{
			Status:  value.Status,
			Path:    value.Path,
			Summary: value.Summary,
		}, nil
	default:
		return measuredStepOutput{}, fmt.Errorf("unsupported measured workflow step %q", stepID)
	}
}

func validateMeasuredTelemetry(
	response ProviderResponse,
	stepBudget float64,
	requireComplete bool,
) error {
	presence := response.TelemetryPresence
	if !isFinite(stepBudget) || stepBudget <= 0 {
		return fmt.Errorf("provider step budget must be finite and positive")
	}

	if presence.ProviderRequestID {
		if strings.TrimSpace(response.ProviderRequestID) == "" {
			return fmt.Errorf("provider request ID telemetry is present but empty")
		}
	} else if response.ProviderRequestID != "" {
		return fmt.Errorf("provider request ID is present without telemetry presence")
	}
	for _, identity := range []struct {
		name    string
		value   string
		present bool
	}{
		{"response", response.ProviderResponseID, presence.ProviderResponseID},
		{"session", response.ProviderSessionID, presence.ProviderSessionID},
	} {
		if identity.present {
			if strings.TrimSpace(identity.value) == "" {
				return fmt.Errorf("provider %s ID telemetry is present but empty", identity.name)
			}
		} else if identity.value != "" {
			return fmt.Errorf("provider %s ID is present without telemetry presence", identity.name)
		}
	}

	if presence.InputTokens {
		if response.InputTokens < 0 {
			return fmt.Errorf("provider input token telemetry must be non-negative")
		}
	} else if response.InputTokens != 0 {
		return fmt.Errorf("provider input tokens are present without telemetry presence")
	}
	if presence.OutputTokens {
		if response.OutputTokens < 0 {
			return fmt.Errorf("provider output token telemetry must be non-negative")
		}
	} else if response.OutputTokens != 0 {
		return fmt.Errorf("provider output tokens are present without telemetry presence")
	}
	if presence.CostUSD {
		if !isFinite(response.CostUSD) || response.CostUSD < 0 {
			return fmt.Errorf("provider cost telemetry must be non-negative and finite")
		}
		if requireComplete && response.CostUSD > stepBudget+1e-9 {
			return fmt.Errorf("provider cost %.8f exceeded step budget %.8f", response.CostUSD, stepBudget)
		}
	} else if response.CostUSD != 0 {
		return fmt.Errorf("provider cost is present without telemetry presence")
	}
	if presence.DurationMS {
		if response.Duration < 0 {
			return fmt.Errorf("provider duration telemetry must be non-negative")
		}
	} else if response.Duration != 0 {
		return fmt.Errorf("provider duration is present without telemetry presence")
	}

	models := make([]string, 0, len(response.ProviderModelUsage))
	for model, usage := range response.ProviderModelUsage {
		if strings.TrimSpace(model) == "" || usage == nil {
			return fmt.Errorf("provider model usage must contain named object values")
		}
		models = append(models, model)
		if err := validateRawModelUsage(model, usage, requireComplete); err != nil {
			return err
		}
	}
	sort.Strings(models)
	if presence.ModelUsage {
		if len(models) != len(response.ProviderModels) {
			return fmt.Errorf("provider model list does not match raw model usage")
		}
		for index, model := range models {
			if response.ProviderModels[index] != model {
				return fmt.Errorf("provider model list does not match raw model usage")
			}
		}
	} else if len(models) != 0 || len(response.ProviderModels) != 0 {
		return fmt.Errorf("provider model usage is present without telemetry presence")
	}

	rawInputPresent, rawOutputPresent, rawCostPresent := rawModelUsagePresence(response.ProviderModelUsage)
	rawInputComplete, rawOutputComplete, rawCostComplete := rawModelUsageCompleteness(response.ProviderModelUsage)
	if rawInputPresent != rawInputComplete ||
		rawOutputPresent != rawOutputComplete ||
		rawCostPresent != rawCostComplete {
		return fmt.Errorf("provider raw model usage has inconsistent partial fields")
	}
	if presence.InputTokens != rawInputComplete {
		return fmt.Errorf("provider input token presence does not match raw model usage")
	}
	if presence.OutputTokens != rawOutputComplete {
		return fmt.Errorf("provider output token presence does not match raw model usage")
	}
	if requireComplete {
		if !presence.ModelUsage || !presence.InputTokens || !presence.OutputTokens ||
			!presence.CostUSD || !presence.DurationMS || !presence.ProviderResponseID ||
			!presence.ProviderSessionID {
			return fmt.Errorf("provider response is missing required measured telemetry")
		}
		if len(response.ProviderModels) == 0 || len(response.ProviderModelUsage) == 0 {
			return fmt.Errorf("provider response is missing actual model usage")
		}
		if !rawCostComplete {
			return fmt.Errorf("provider model usage is missing token or cost telemetry")
		}
	}
	if rawInputComplete {
		inputTokens, _, _ := aggregateRawModelUsage(response.ProviderModelUsage)
		if inputTokens != response.InputTokens {
			return fmt.Errorf("provider aggregate telemetry does not reconcile with raw model usage")
		}
	}
	if rawOutputComplete {
		_, outputTokens, _ := aggregateRawModelUsage(response.ProviderModelUsage)
		if outputTokens != response.OutputTokens {
			return fmt.Errorf("provider aggregate telemetry does not reconcile with raw model usage")
		}
	}
	if rawCostComplete && presence.CostUSD {
		_, _, cost := aggregateRawModelUsage(response.ProviderModelUsage)
		if math.Abs(cost-response.CostUSD) > 1e-8 {
			return fmt.Errorf("provider aggregate telemetry does not reconcile with raw model usage")
		}
	}
	return nil
}

func validateRawModelUsage(
	model string,
	usage map[string]json.RawMessage,
	requireComplete bool,
) error {
	const (
		inputTokensKey  = "inputTokens"
		outputTokensKey = "outputTokens"
		costUSDKey      = "costUSD"
	)
	for key, raw := range usage {
		var err error
		switch {
		case key == costUSDKey:
			_, err = nonNegativeFloat(raw, fmt.Sprintf("provider model usage %q costUSD", model))
		case key == inputTokensKey || key == outputTokensKey ||
			strings.HasSuffix(key, "Tokens") ||
			key == "contextWindow" || key == "maxOutputTokens" ||
			key == "webSearchRequests":
			_, err = nonNegativeInt(raw, fmt.Sprintf("provider model usage %q %s", model, key))
		default:
			if !json.Valid(raw) {
				err = fmt.Errorf("provider model usage %q field %s is invalid JSON", model, key)
			}
		}
		if err != nil {
			return err
		}
	}
	if requireComplete {
		for _, key := range []string{inputTokensKey, outputTokensKey, costUSDKey} {
			if _, ok := usage[key]; !ok {
				return fmt.Errorf("provider model usage %q is missing token or cost telemetry", model)
			}
		}
	}
	return nil
}

func rawModelUsageCompleteness(
	usage map[string]map[string]json.RawMessage,
) (inputComplete, outputComplete, costComplete bool) {
	if len(usage) == 0 {
		return false, false, false
	}
	inputComplete, outputComplete, costComplete = true, true, true
	for _, modelUsage := range usage {
		if _, ok := modelUsage["inputTokens"]; !ok {
			inputComplete = false
		}
		if _, ok := modelUsage["outputTokens"]; !ok {
			outputComplete = false
		}
		if _, ok := modelUsage["costUSD"]; !ok {
			costComplete = false
		}
	}
	return inputComplete, outputComplete, costComplete
}

func rawModelUsagePresence(
	usage map[string]map[string]json.RawMessage,
) (inputPresent, outputPresent, costPresent bool) {
	for _, modelUsage := range usage {
		if _, ok := modelUsage["inputTokens"]; ok {
			inputPresent = true
		}
		if _, ok := modelUsage["outputTokens"]; ok {
			outputPresent = true
		}
		if _, ok := modelUsage["costUSD"]; ok {
			costPresent = true
		}
	}
	return inputPresent, outputPresent, costPresent
}

func aggregateRawModelUsage(
	usage map[string]map[string]json.RawMessage,
) (inputTokens, outputTokens int, cost float64) {
	for _, modelUsage := range usage {
		if raw, ok := modelUsage["inputTokens"]; ok {
			input, _ := nonNegativeInt(raw, "inputTokens")
			inputTokens += input
		}
		if raw, ok := modelUsage["outputTokens"]; ok {
			output, _ := nonNegativeInt(raw, "outputTokens")
			outputTokens += output
		}
		if raw, ok := modelUsage["costUSD"]; ok {
			modelCost, _ := nonNegativeFloat(raw, "costUSD")
			cost += modelCost
		}
	}
	return inputTokens, outputTokens, cost
}

func shouldRetryStep(stepID, decision string) bool {
	return (stepID == "validator" && decision == "FAIL") ||
		(stepID == "code-reviewer" && decision == "BLOCK") ||
		(stepID == "review-challenge" && decision == "FAIL")
}

func addWorkflowTotals(totals *WorkflowTotals, step WorkflowStepResult) {
	totals.Invocations++
	totals.InputTokens += step.InputTokens
	totals.OutputTokens += step.OutputTokens
	totals.TotalTokens = totals.InputTokens + totals.OutputTokens
	totals.CostUSD += step.CostUSD
	totals.DurationMS += step.DurationMS
}

func finishMeasuredWorkflow(
	ctx context.Context,
	workspace string,
	testCase Case,
	result CaseResult,
	store *workflowEvidenceStore,
	status string,
	code string,
	origin string,
	attempt int,
	message string,
) (CaseResult, error) {
	workflow := result.Workflow
	workflow.Status = status
	workflow.FinalAttempt = attempt
	workflow.Audit = append([]WorkflowAuditRecord{}, store.records...)
	workflow.Sentinels = append([]WorkflowSentinel{}, store.written...)
	if code != "" {
		workflow.Failure = &WorkflowFailure{
			Code:       code,
			OriginStep: origin,
			Attempt:    attempt,
			Message:    message,
		}
	}
	if _, err := store.appendTerminal(*workflow); err != nil {
		result.Workflow = nil
		return result, fmt.Errorf(
			"append terminal workflow audit record; measured workflow evidence discarded: %w",
			err,
		)
	}
	workflow.Audit = append([]WorkflowAuditRecord{}, store.records...)
	workflow.Sentinels = append([]WorkflowSentinel{}, store.written...)
	reconciliationErr := store.verifyAudit()
	if reconciliationErr == nil {
		reconciliationErr = validateWorkflowEvidence(*workflow)
	}
	if reconciliationErr != nil {
		failureCode := "TELEMETRY_RECONCILIATION_FAILED"
		if auditErr := store.verifyAudit(); auditErr != nil {
			failureCode = "AUDIT_RECONCILIATION_FAILED"
		}
		workflow.Status = WorkflowBlocked
		workflow.Failure = &WorkflowFailure{
			Code:       failureCode,
			OriginStep: origin,
			Attempt:    attempt,
			Message:    reconciliationErr.Error(),
		}
		status = WorkflowBlocked
		if err := store.resealTerminal(*workflow); err != nil {
			result.Workflow = nil
			return result, fmt.Errorf(
				"measured workflow evidence reconciliation failed after terminal append; evidence discarded: %w",
				err,
			)
		}
		workflow.Audit = append([]WorkflowAuditRecord{}, store.records...)
		if err := store.verifyAudit(); err != nil {
			result.Workflow = nil
			return result, fmt.Errorf(
				"resealed measured workflow audit failed verification; evidence discarded: %w",
				err,
			)
		}
		if err := validateWorkflowEvidence(*workflow); err != nil {
			result.Workflow = nil
			return result, fmt.Errorf(
				"resealed measured workflow evidence failed validation; evidence discarded: %w",
				err,
			)
		}
	}
	workflow.Audit = append([]WorkflowAuditRecord{}, store.records...)
	workflow.Sentinels = append([]WorkflowSentinel{}, store.written...)
	result.Output = measuredWorkflowOutput(*workflow)
	result.InputTokens = workflow.Totals.InputTokens
	result.OutputTokens = workflow.Totals.OutputTokens
	result.CostUSD = workflow.Totals.CostUSD
	result.DurationMS = workflow.Totals.DurationMS
	result.ProviderModels = workflowModels(workflow.Steps)
	result.GitDiff, result.GitStatus = captureGitWorkspace(ctx, workspace)
	result.AssertionResults = EvaluateAssertions(result.Output, workspace, testCase.Assertions)
	result.ValidatorResults = EvaluateValidators(result.Output, workspace, testCase.Validators)
	evaluationPassed, evaluationStatus := evaluationStatus(result.AssertionResults, result.ValidatorResults)
	result.Passed = status == WorkflowPendingHuman && evaluationPassed
	result.Status = evaluationStatus
	if status != WorkflowPendingHuman {
		result.Passed = false
		if code == "PROVIDER_FAILURE" {
			result.Status = StatusInfrastructureErr
			result.ProviderError = message
		} else {
			result.Status = StatusAgentFailure
		}
	}
	if !result.Passed {
		return result, fmt.Errorf("measured workflow %s: %s", workflow.Status, workflowFailureMessage(workflow))
	}
	return result, nil
}

func reconcileWorkflowTotals(workflow WorkflowResult) error {
	var totals WorkflowTotals
	for _, step := range workflow.Steps {
		addWorkflowTotals(&totals, step)
	}
	if totals.Invocations != workflow.Totals.Invocations ||
		totals.InputTokens != workflow.Totals.InputTokens ||
		totals.OutputTokens != workflow.Totals.OutputTokens ||
		totals.TotalTokens != workflow.Totals.TotalTokens ||
		math.Abs(totals.CostUSD-workflow.Totals.CostUSD) > 1e-8 ||
		totals.DurationMS != workflow.Totals.DurationMS {
		return fmt.Errorf("workflow totals do not reconcile with step records")
	}
	return nil
}

func measuredWorkflowOutput(workflow WorkflowResult) string {
	var output strings.Builder
	for _, step := range workflow.Steps {
		fmt.Fprintf(&output, "%s attempt %d output: %s\n", step.StepID, step.Attempt, step.Output)
	}
	if workflow.Status == WorkflowPendingHuman {
		output.WriteString("builder: done\nvalidator: PASS\ncode-reviewer: APPROVE\n")
		output.WriteString("review-challenge: PASS\ndocumenter: done\nStatus: PENDING_HUMAN\n")
	} else {
		fmt.Fprintf(&output, "Status: blocked\nMissing gate: %s\n", workflowFailureOrigin(&workflow))
	}
	return output.String()
}

func workflowModels(steps []WorkflowStepResult) []string {
	seen := map[string]bool{}
	var models []string
	for _, step := range steps {
		for _, model := range step.ActualModels {
			if !seen[model] {
				seen[model] = true
				models = append(models, model)
			}
		}
	}
	return models
}

func workflowFailureMessage(workflow *WorkflowResult) string {
	if workflow.Failure == nil {
		return workflow.Status
	}
	return workflow.Failure.Message
}

func workflowFailureOrigin(workflow *WorkflowResult) string {
	if workflow.Failure == nil || workflow.Failure.OriginStep == "" {
		return "unknown"
	}
	return workflow.Failure.OriginStep
}

func retryOrigin(message string) string {
	for _, step := range []string{"validator", "code-reviewer", "review-challenge"} {
		if strings.HasPrefix(message, step+" returned") {
			return step
		}
	}
	return "unknown"
}

func measuredStepFailureCode(err error) string {
	message := err.Error()
	switch {
	case strings.Contains(message, " provider:"):
		return "PROVIDER_FAILURE"
	case strings.Contains(message, "missing required measured telemetry"),
		strings.Contains(message, "missing required provider identifiers"),
		strings.Contains(message, "missing actual model usage"),
		strings.Contains(message, "missing token or cost telemetry"):
		return "TELEMETRY_MISSING"
	case strings.Contains(message, "does not reconcile"):
		return "TELEMETRY_RECONCILIATION_FAILED"
	case strings.Contains(message, "exceeded step budget"):
		return "STEP_BUDGET_EXCEEDED"
	case strings.Contains(message, "read-only workflow step changed"):
		return "WORKSPACE_MUTATION"
	case strings.Contains(message, "protected workflow agent definitions changed"):
		return "AGENT_DEFINITION_MUTATION"
	case strings.Contains(message, "capture workflow"),
		strings.Contains(message, "hash untracked workflow"):
		return "WORKSPACE_HASH_FAILED"
	default:
		return "OUTPUT_CONTRACT_FAILED"
	}
}
