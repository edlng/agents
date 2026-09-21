package litmus

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMeasuredWorkflowCompletesFiveOrderedStepsAndStopsPendingHuman(t *testing.T) {
	config := measuredTestConfig(t)
	executor := &sequenceExecutor{responses: successfulMeasuredResponses()}
	runner := measuredTestRunner(t, executor)

	result, err := runner.Run(context.Background(), measuredTestCase(), config, 2.50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Passed || result.Workflow == nil || result.Workflow.Status != WorkflowPendingHuman {
		t.Fatalf("result = %#v, want successful automated workflow pending human", result)
	}
	if len(result.Workflow.Steps) != 5 || len(result.Workflow.Audit) != 6 ||
		len(result.Workflow.Sentinels) != 5 {
		t.Fatalf("workflow evidence = %#v, want five steps, audit records, and sentinels", result.Workflow)
	}
	wantAgents := []string{"builder", "validator", "code-reviewer", "validator", "documenter"}
	for index, step := range result.Workflow.Steps {
		if step.Sequence != index+1 || step.Agent != wantAgents[index] || step.Attempt != 1 ||
			step.SentinelPath == "" {
			t.Fatalf("step %d = %#v, want ordered accepted step", index, step)
		}
		if !step.TelemetryPresence.CostUSD || step.ProviderResponseID == "" ||
			step.ProviderSessionID == "" {
			t.Fatalf("step %d missing measured telemetry: %#v", index, step)
		}
	}
	terminal := result.Workflow.Audit[len(result.Workflow.Audit)-1]
	if terminal.RecordType != WorkflowAuditTerminalRecord ||
		terminal.StepID != "terminal" ||
		terminal.Status != WorkflowPendingHuman ||
		terminal.Decision != WorkflowPendingHuman ||
		terminal.TerminalFailureJSON != "null" {
		t.Fatalf("terminal audit record = %#v, want pending-human terminal metadata", terminal)
	}
	if !strings.Contains(result.Output, "Status: PENDING_HUMAN") ||
		strings.Contains(result.Output, "human-gate: APPROVE") {
		t.Fatalf("output = %q, want paused human gate without approval", result.Output)
	}
	if executor.calls != 5 {
		t.Fatalf("executor calls = %d, want 5", executor.calls)
	}
}

func TestMeasuredWorkflowRejectsTamperedFinalStepStatus(t *testing.T) {
	result, err := measuredTestRunner(
		t,
		&sequenceExecutor{responses: successfulMeasuredResponses()},
	).Run(
		context.Background(),
		measuredTestCase(),
		measuredTestConfig(t),
		2.50,
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(result.Workflow)
	if err != nil {
		t.Fatal(err)
	}
	var tampered WorkflowResult
	if err := json.Unmarshal(encoded, &tampered); err != nil {
		t.Fatal(err)
	}
	tampered.Steps[4].Status = "BLOCKED"
	tampered.Audit[4].Status = "BLOCKED"
	tampered.Sentinels = tampered.Sentinels[:4]
	previous := ""
	for index := range tampered.Audit {
		tampered.Audit[index].PreviousRecordHash = previous
		tampered.Audit[index].RecordHash = ""
		tampered.Audit[index].RecordHash, err = hashJSON(tampered.Audit[index])
		if err != nil {
			t.Fatal(err)
		}
		previous = tampered.Audit[index].RecordHash
	}
	if err := validateWorkflowEvidence(tampered); err == nil ||
		!strings.Contains(err.Error(), "PENDING_HUMAN requires exactly five ordered final-attempt steps") {
		t.Fatalf("validateWorkflowEvidence() error = %v, want final accepted-status rejection", err)
	}
}

func TestMeasuredWorkflowRetriesFromBuilderAndPreservesEveryInvocation(t *testing.T) {
	responses := []ProviderResponse{
		measuredResponse(`{"status":"done","summary":"first","files":["x.go"],"verification":"go test"}`, 0.01, 1),
		measuredResponse(`{"decision":"FAIL","reason":"test failed","issues":["missing edge case"]}`, 0.01, 2),
	}
	responses = append(responses, successfulMeasuredResponses()...)
	executor := &sequenceExecutor{responses: responses}
	runner := measuredTestRunner(t, executor)

	result, err := runner.Run(context.Background(), measuredTestCase(), measuredTestConfig(t), 2.50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.Workflow.FinalAttempt != 2 || len(result.Workflow.Steps) != 7 ||
		result.Workflow.Totals.Invocations != 7 {
		t.Fatalf("workflow = %#v, want two retained first-attempt calls plus final five", result.Workflow)
	}
	final := result.Workflow.Steps[2:]
	for _, step := range final {
		if step.Attempt != 2 {
			t.Fatalf("final successful step = %#v, want attempt 2", step)
		}
	}
	if !strings.Contains(executor.requests[2].Task, "validator returned FAIL") {
		t.Fatalf("retry builder task missing failure feedback: %s", executor.requests[2].Task)
	}
	if err := validateWorkflowEvidence(*result.Workflow); err != nil {
		t.Fatalf("retained retry evidence does not reconcile: %v", err)
	}
}

func TestMeasuredWorkflowFailsClosedOnMalformedOutput(t *testing.T) {
	executor := &sequenceExecutor{responses: []ProviderResponse{
		measuredResponse("not-json", 0.01, 1),
	}}
	result, err := measuredTestRunner(t, executor).Run(
		context.Background(),
		measuredTestCase(),
		measuredTestConfig(t),
		2.50,
		0,
	)
	if err == nil || result.Passed || result.Workflow.Status != WorkflowBlocked {
		t.Fatalf("Run() = %#v, %v; want blocked malformed output", result, err)
	}
	if result.Workflow.Steps == nil || result.Workflow.Audit == nil || result.Workflow.Sentinels == nil {
		t.Fatalf("workflow evidence contains nil collections: %#v", result.Workflow)
	}
	encoded, marshalErr := json.Marshal(result.Workflow)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if !strings.Contains(string(encoded), `"sentinels":[]`) {
		t.Fatalf("encoded workflow = %s, want an empty sentinel array", encoded)
	}
	if result.Workflow.Failure.Code != "OUTPUT_CONTRACT_FAILED" || len(result.Workflow.Steps) != 1 ||
		result.Workflow.Steps[0].Error == "" {
		t.Fatalf("failure evidence = %#v, want retained malformed invocation", result.Workflow)
	}
	terminal := result.Workflow.Audit[len(result.Workflow.Audit)-1]
	if terminal.RecordType != WorkflowAuditTerminalRecord ||
		terminal.Status != WorkflowBlocked ||
		terminal.Decision != "OUTPUT_CONTRACT_FAILED" ||
		terminal.TerminalFailureJSON == "null" ||
		!strings.Contains(terminal.TerminalFailureJSON, "OUTPUT_CONTRACT_FAILED") {
		t.Fatalf("terminal failure evidence = %#v, want bound workflow failure metadata", terminal)
	}
	if err := validateWorkflowEvidence(*result.Workflow); err != nil {
		t.Fatalf("failure evidence does not reconcile: %v", err)
	}
}

func TestMeasuredWorkflowRequiresDocumenterPathToBeExistingRegularWorkspaceFile(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "missing file", path: "docs/missing.md", want: "does not exist"},
		{name: "unsafe traversal", path: "../outside.md", want: "unsafe"},
		{name: "directory", path: "docs", want: "regular file"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			responses := successfulMeasuredResponses()
			responses[4] = measuredResponse(
				fmt.Sprintf(`{"status":"done","path":%q,"summary":"documented"}`, test.path),
				0.01,
				5,
			)
			result, err := measuredTestRunner(t, &sequenceExecutor{responses: responses}).Run(
				context.Background(),
				measuredTestCase(),
				measuredTestConfig(t),
				2.50,
				0,
			)
			if err == nil || result.Workflow == nil ||
				result.Workflow.Failure == nil ||
				result.Workflow.Failure.OriginStep != "documenter" ||
				result.Workflow.Steps[len(result.Workflow.Steps)-1].Error == "" ||
				!strings.Contains(result.Workflow.Steps[len(result.Workflow.Steps)-1].Error, test.want) {
				t.Fatalf("Run() = %#v, %v; want documenter path rejection containing %q", result, err, test.want)
			}
		})
	}
}

func TestMeasuredWorkflowAcceptsReportedZeroCostButRejectsAbsentCost(t *testing.T) {
	t.Run("reported zero", func(t *testing.T) {
		executor := &sequenceExecutor{responses: successfulMeasuredResponses()}
		for index := range executor.responses {
			executor.responses[index] = measuredResponse(executor.responses[index].Output, 0, index+1)
		}
		result, err := measuredTestRunner(t, executor).Run(
			context.Background(),
			measuredTestCase(),
			measuredTestConfig(t),
			2.50,
			0,
		)
		if err != nil || !result.Passed || result.CostUSD != 0 {
			t.Fatalf("Run() = %#v, %v; want valid reported zero-cost workflow", result, err)
		}
	})

	t.Run("absent cost", func(t *testing.T) {
		response := measuredResponse(
			`{"status":"done","summary":"built","files":["x.go"],"verification":"go test"}`,
			0,
			1,
		)
		response.TelemetryPresence.CostUSD = false
		executor := &sequenceExecutor{responses: []ProviderResponse{response}}
		result, err := measuredTestRunner(t, executor).Run(
			context.Background(),
			measuredTestCase(),
			measuredTestConfig(t),
			2.50,
			0,
		)
		if err == nil || result.Passed ||
			!strings.Contains(result.Workflow.Steps[0].Error, "missing required measured telemetry") {
			t.Fatalf("Run() = %#v, %v; want absent cost rejected", result, err)
		}
	})
}

func TestMeasuredWorkflowStopsBeforeLaunchingStepWithoutBudget(t *testing.T) {
	executor := &sequenceExecutor{}
	result, err := measuredTestRunner(t, executor).Run(
		context.Background(),
		measuredTestCase(),
		measuredTestConfig(t),
		0.74,
		0,
	)
	if err == nil || result.Workflow.Failure.Code != "BUDGET_EXHAUSTED" || executor.calls != 0 {
		t.Fatalf("Run() = %#v, %v, calls=%d; want pre-launch budget block", result, err, executor.calls)
	}
}

func TestMeasuredWorkflowStopsBeforeNextStepWhenGlobalBudgetIsInsufficient(t *testing.T) {
	responses := successfulMeasuredResponses()
	responses[0] = measuredResponse(responses[0].Output, 0.40, 1)
	executor := &sequenceExecutor{responses: responses}
	result, err := measuredTestRunner(t, executor).Run(
		context.Background(),
		measuredTestCase(),
		measuredTestConfig(t),
		0.75,
		0,
	)
	if err == nil || result.Workflow.Failure.Code != "BUDGET_EXHAUSTED" ||
		result.Workflow.Failure.OriginStep != "validator" || executor.calls != 1 ||
		result.Workflow.Totals.CostUSD != 0.40 {
		t.Fatalf("Run() = %#v, %v, calls=%d; want one retained call then global budget block", result, err, executor.calls)
	}
}

func TestMeasuredWorkflowRejectsProviderCostAboveStepBudget(t *testing.T) {
	response := measuredResponse(
		`{"status":"done","summary":"built","files":["x.go"],"verification":"go test"}`,
		0.76,
		1,
	)
	executor := &sequenceExecutor{responses: []ProviderResponse{response}}
	result, err := measuredTestRunner(t, executor).Run(
		context.Background(),
		measuredTestCase(),
		measuredTestConfig(t),
		2.50,
		0,
	)
	if err == nil || result.Workflow.Failure.Code != "STEP_BUDGET_EXCEEDED" ||
		executor.calls != 1 || result.Workflow.Totals.CostUSD != 0.76 {
		t.Fatalf("Run() = %#v, %v; want retained over-budget provider invocation", result, err)
	}
}

func TestMeasuredWorkflowRejectsRawUsageReconciliationMismatch(t *testing.T) {
	response := measuredResponse(
		`{"status":"done","summary":"built","files":["x.go"],"verification":"go test"}`,
		0.01,
		1,
	)
	response.InputTokens++
	executor := &sequenceExecutor{responses: []ProviderResponse{response}}
	result, err := measuredTestRunner(t, executor).Run(
		context.Background(),
		measuredTestCase(),
		measuredTestConfig(t),
		2.50,
		0,
	)
	if err == nil || result.Workflow != nil ||
		!strings.Contains(err.Error(), "resealed measured workflow evidence failed validation") {
		t.Fatalf("Run() = %#v, %v; want inconsistent telemetry evidence discarded", result, err)
	}
}

func TestMeasuredWorkflowProviderHashBindsAllTelemetry(t *testing.T) {
	result, err := measuredTestRunner(
		t,
		&sequenceExecutor{responses: successfulMeasuredResponses()},
	).Run(
		context.Background(),
		measuredTestCase(),
		measuredTestConfig(t),
		2.50,
		0,
	)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*WorkflowResult)
	}{
		{
			name: "raw model usage and aggregate tokens",
			mutate: func(workflow *WorkflowResult) {
				workflow.Steps[0].ProviderModelUsage["claude-test"]["inputTokens"] = json.RawMessage("11")
				workflow.Steps[0].InputTokens++
				workflow.Totals.InputTokens++
				workflow.Totals.TotalTokens++
			},
		},
		{
			name: "duration",
			mutate: func(workflow *WorkflowResult) {
				workflow.Steps[0].DurationMS++
				workflow.Totals.DurationMS++
			},
		},
		{
			name: "telemetry presence",
			mutate: func(workflow *WorkflowResult) {
				workflow.Steps[0].TelemetryPresence.ProviderRequestID = true
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, marshalErr := json.Marshal(result.Workflow)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			var tampered WorkflowResult
			if unmarshalErr := json.Unmarshal(encoded, &tampered); unmarshalErr != nil {
				t.Fatal(unmarshalErr)
			}
			test.mutate(&tampered)
			if validationErr := validateWorkflowEvidence(tampered); validationErr == nil ||
				!strings.Contains(validationErr.Error(), "provider invocation hash is invalid") {
				t.Fatalf("validateWorkflowEvidence() error = %v, want provider hash rejection", validationErr)
			}
		})
	}
}

func TestMeasuredWorkflowAuditAndSentinelBindProviderTelemetryHash(t *testing.T) {
	result, err := measuredTestRunner(
		t,
		&sequenceExecutor{responses: successfulMeasuredResponses()},
	).Run(
		context.Background(),
		measuredTestCase(),
		measuredTestConfig(t),
		2.50,
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(result.Workflow)
	if err != nil {
		t.Fatal(err)
	}
	var tampered WorkflowResult
	if err := json.Unmarshal(encoded, &tampered); err != nil {
		t.Fatal(err)
	}
	tampered.Steps[0].DurationMS++
	tampered.Totals.DurationMS++
	tampered.Steps[0].ProviderInvocationHash, err = providerInvocationHash(tampered.Steps[0])
	if err != nil {
		t.Fatal(err)
	}

	previous := ""
	for index := range tampered.Audit {
		if index == 0 {
			tampered.Audit[index].ProviderInvocationHash =
				tampered.Steps[0].ProviderInvocationHash
		}
		tampered.Audit[index].PreviousRecordHash = previous
		tampered.Audit[index].RecordHash = ""
		tampered.Audit[index].RecordHash, err = hashJSON(tampered.Audit[index])
		if err != nil {
			t.Fatal(err)
		}
		previous = tampered.Audit[index].RecordHash
	}

	if err := validateWorkflowEvidence(tampered); err == nil ||
		!strings.Contains(err.Error(), "workflow sentinel builder attempt 1 is invalid") {
		t.Fatalf("validateWorkflowEvidence() error = %v, want sentinel telemetry binding rejection", err)
	}
}

func TestMeasuredWorkflowEvidenceRecomputesPersistedOutputHash(t *testing.T) {
	result, err := measuredTestRunner(
		t,
		&sequenceExecutor{responses: successfulMeasuredResponses()},
	).Run(
		context.Background(),
		measuredTestCase(),
		measuredTestConfig(t),
		2.50,
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	tampered := *result.Workflow
	tampered.Steps = append([]WorkflowStepResult(nil), result.Workflow.Steps...)
	tampered.Steps[0].Output = `{"status":"done","summary":"tampered","files":["x.go"],"verification":"go test"}`
	if err := validateWorkflowEvidence(tampered); err == nil ||
		!strings.Contains(err.Error(), "output hash does not match persisted output") {
		t.Fatalf("validateWorkflowEvidence() error = %v, want persisted-output hash rejection", err)
	}
}

func TestMeasuredWorkflowAuditBindsStepError(t *testing.T) {
	result, err := measuredTestRunner(
		t,
		&sequenceExecutor{
			responses: []ProviderResponse{measuredResponse("not-json", 0.01, 1)},
		},
	).Run(
		context.Background(),
		measuredTestCase(),
		measuredTestConfig(t),
		2.50,
		0,
	)
	if err == nil {
		t.Fatal("Run() error = nil, want malformed-output failure")
	}
	tampered := *result.Workflow
	tampered.Steps = append([]WorkflowStepResult(nil), result.Workflow.Steps...)
	tampered.Audit = append([]WorkflowAuditRecord(nil), result.Workflow.Audit...)
	tampered.Steps[0].Error = "tampered step error"
	if err := validateWorkflowEvidence(tampered); err == nil ||
		!strings.Contains(err.Error(), "does not match step") {
		t.Fatalf("validateWorkflowEvidence() error = %v, want step error binding rejection", err)
	}
}

func TestMeasuredWorkflowProviderFailureRetainsInvocationTelemetry(t *testing.T) {
	response := measuredResponse("partial", 0.02, 1)
	executor := &sequenceExecutor{
		responses: []ProviderResponse{response},
		errors:    []error{fmt.Errorf("provider unavailable")},
	}
	result, err := measuredTestRunner(t, executor).Run(
		context.Background(),
		measuredTestCase(),
		measuredTestConfig(t),
		2.50,
		0,
	)
	if err == nil || len(result.Workflow.Steps) != 1 ||
		result.Workflow.Steps[0].CostUSD != 0.02 ||
		result.Workflow.Totals.CostUSD != 0.02 ||
		result.Workflow.Failure.Code != "PROVIDER_FAILURE" ||
		result.Status != StatusInfrastructureErr ||
		result.ProviderError == "" {
		t.Fatalf("Run() = %#v, %v; want retained partial provider telemetry", result, err)
	}
}

func TestMeasuredWorkflowAcceptsValidPartialBlockedTelemetry(t *testing.T) {
	response := ProviderResponse{
		Output:             "provider stopped after partial telemetry",
		ProviderModels:     []string{"claude-test"},
		ProviderResponseID: "response-1",
		ProviderModelUsage: map[string]map[string]json.RawMessage{
			"claude-test": {
				"inputTokens": json.RawMessage("10"),
			},
		},
		InputTokens: 10,
		TelemetryPresence: TelemetryPresence{
			ModelUsage:         true,
			InputTokens:        true,
			ProviderResponseID: true,
		},
	}
	executor := &sequenceExecutor{
		responses: []ProviderResponse{response},
		errors:    []error{fmt.Errorf("provider unavailable")},
	}
	result, err := measuredTestRunner(t, executor).Run(
		context.Background(),
		measuredTestCase(),
		measuredTestConfig(t),
		2.50,
		0,
	)
	if err == nil || result.Workflow == nil ||
		result.Workflow.Status != WorkflowBlocked ||
		result.Workflow.Steps[0].InputTokens != 10 ||
		result.Workflow.Steps[0].TelemetryPresence.OutputTokens ||
		result.Workflow.Steps[0].TelemetryPresence.CostUSD {
		t.Fatalf("Run() = %#v, %v; want valid partial blocked telemetry", result, err)
	}
	if validationErr := validateWorkflowEvidence(*result.Workflow); validationErr != nil {
		t.Fatalf("valid partial blocked telemetry does not reconcile: %v", validationErr)
	}
}

func TestMeasuredWorkflowRejectsInconsistentPartialBlockedTelemetry(t *testing.T) {
	response := ProviderResponse{
		Output:             "provider stopped after partial telemetry",
		ProviderModels:     []string{"claude-test"},
		ProviderResponseID: "response-1",
		ProviderModelUsage: map[string]map[string]json.RawMessage{
			"claude-test": {
				"inputTokens": json.RawMessage("10"),
			},
		},
		InputTokens: 9,
		TelemetryPresence: TelemetryPresence{
			ModelUsage:         true,
			InputTokens:        true,
			ProviderResponseID: true,
		},
	}
	executor := &sequenceExecutor{
		responses: []ProviderResponse{response},
		errors:    []error{fmt.Errorf("provider unavailable")},
	}
	result, err := measuredTestRunner(t, executor).Run(
		context.Background(),
		measuredTestCase(),
		measuredTestConfig(t),
		2.50,
		0,
	)
	if err == nil || result.Workflow != nil ||
		!strings.Contains(err.Error(), "resealed measured workflow evidence failed validation") {
		t.Fatalf("Run() = %#v, %v; want inconsistent partial telemetry rejected with discarded evidence", result, err)
	}
}

func TestFinishMeasuredWorkflowResealsTerminalAfterReconciliationFailure(t *testing.T) {
	store, err := newWorkflowEvidenceStore(t.TempDir(), "workflow", "run", "case")
	if err != nil {
		t.Fatal(err)
	}
	workflow := &WorkflowResult{
		SchemaVersion: MeasuredWorkflowSchemaVersion,
		WorkflowID:    "workflow",
		RunID:         "run",
		CaseID:        "case",
		MaxAttempts:   1,
		Totals:        WorkflowTotals{},
	}
	result := CaseResult{
		Agent:    "team-lead",
		CaseID:   "case",
		Workflow: workflow,
	}
	finished, err := finishMeasuredWorkflow(
		context.Background(),
		t.TempDir(),
		measuredTestCase(),
		result,
		store,
		WorkflowPendingHuman,
		"",
		"",
		1,
		"",
	)
	if err == nil || finished.Workflow == nil {
		t.Fatalf("finishMeasuredWorkflow() = %#v, %v; want blocked result after reseal", finished, err)
	}
	if finished.Workflow.Status != WorkflowBlocked ||
		finished.Workflow.Failure == nil ||
		finished.Workflow.Audit[len(finished.Workflow.Audit)-1].Status != WorkflowBlocked ||
		finished.Workflow.Audit[len(finished.Workflow.Audit)-1].Decision != finished.Workflow.Failure.Code ||
		!strings.Contains(
			finished.Workflow.Audit[len(finished.Workflow.Audit)-1].TerminalFailureJSON,
			finished.Workflow.Failure.Code,
		) {
		t.Fatalf("resealed workflow = %#v, want failure bound into terminal audit", finished.Workflow)
	}
	if validationErr := validateWorkflowEvidence(*finished.Workflow); validationErr != nil {
		t.Fatalf("resealed workflow does not validate: %v", validationErr)
	}
}

func TestFinishMeasuredWorkflowDiscardsUnboundWorkflowWhenTerminalWriteFails(t *testing.T) {
	store, err := newWorkflowEvidenceStore(t.TempDir(), "workflow", "run", "case")
	if err != nil {
		t.Fatal(err)
	}
	store.auditPath = t.TempDir()
	result := CaseResult{
		Agent:  "team-lead",
		CaseID: "case",
		Workflow: &WorkflowResult{
			SchemaVersion: MeasuredWorkflowSchemaVersion,
			WorkflowID:    "workflow",
			RunID:         "run",
			CaseID:        "case",
			MaxAttempts:   1,
		},
	}

	finished, err := finishMeasuredWorkflow(
		context.Background(),
		t.TempDir(),
		measuredTestCase(),
		result,
		store,
		WorkflowBlocked,
		"INTERNAL_FAILURE",
		"builder",
		1,
		"terminal write failed",
	)
	if err == nil || finished.Workflow != nil ||
		!strings.Contains(err.Error(), "measured workflow evidence discarded") {
		t.Fatalf("finishMeasuredWorkflow() = %#v, %v; want unbound workflow discarded", finished, err)
	}
}

func TestMeasuredWorkflowRejectsReadOnlyStepWorkspaceMutation(t *testing.T) {
	executor := &sequenceExecutor{
		responses: successfulMeasuredResponses(),
		onExecute: func(index int, request ProviderRequest) {
			if index != 1 {
				return
			}
			if err := os.WriteFile(
				filepath.Join(request.Workspace, "validator-mutation.txt"),
				[]byte("not allowed"),
				0o644,
			); err != nil {
				t.Fatal(err)
			}
		},
	}
	result, err := measuredTestRunner(t, executor).Run(
		context.Background(),
		measuredTestCase(),
		measuredTestConfig(t),
		2.50,
		0,
	)
	if err == nil || result.Workflow.Failure.Code != "WORKSPACE_MUTATION" ||
		result.Workflow.Failure.OriginStep != "validator" || executor.calls != 2 {
		t.Fatalf("Run() = %#v, %v; want read-only validator mutation blocked", result, err)
	}
}

func TestMeasuredWorkflowRejectsAgentDefinitionMutation(t *testing.T) {
	executor := &sequenceExecutor{
		responses: successfulMeasuredResponses(),
		onExecute: func(index int, request ProviderRequest) {
			if index != 0 {
				return
			}
			path := filepath.Join(request.Workspace, ".claude", "agents", "validator.md")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("tampered"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
	}
	result, err := measuredTestRunner(t, executor).Run(
		context.Background(),
		measuredTestCase(),
		measuredTestConfig(t),
		2.50,
		0,
	)
	if err == nil || result.Workflow.Failure.Code != "AGENT_DEFINITION_MUTATION" ||
		executor.calls != 1 {
		t.Fatalf("Run() = %#v, %v; want agent-definition mutation block", result, err)
	}
}

func TestMeasuredWorkflowOutputContractsRejectExtraJSON(t *testing.T) {
	_, err := decodeMeasuredStepOutput(
		"validator",
		`{"decision":"PASS","reason":"ok","issues":[]} {"decision":"PASS"}`,
	)
	if err == nil || !strings.Contains(err.Error(), "one JSON object") {
		t.Fatalf("decodeMeasuredStepOutput() error = %v, want trailing JSON rejection", err)
	}
}

func TestLoadMeasuredWorkflowConfigRejectsWrongStepOrder(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "litmus/workflows/bad.json"), `{
		"schema_version": "stage4.measured-workflow.v1",
		"workflow_id": "workflow",
		"max_attempts": 3,
		"steps": [
			{"id":"validator","agent":"validator","budget_usd":1,"json_schema":{}},
			{"id":"builder","agent":"builder","budget_usd":1,"json_schema":{}},
			{"id":"code-reviewer","agent":"code-reviewer","budget_usd":1,"json_schema":{}},
			{"id":"review-challenge","agent":"validator","budget_usd":1,"json_schema":{}},
			{"id":"documenter","agent":"documenter","budget_usd":1,"json_schema":{}}
		]
	}`)
	if _, err := LoadMeasuredWorkflowConfig(root, "bad"); err == nil ||
		!strings.Contains(err.Error(), "must be builder") {
		t.Fatalf("LoadMeasuredWorkflowConfig() error = %v, want ordered-step rejection", err)
	}
}

func measuredTestRunner(t *testing.T, executor Executor) MeasuredWorkflowRunner {
	t.Helper()
	return MeasuredWorkflowRunner{
		Root:     repoRoot(t),
		Executor: executor,
		Now: func() time.Time {
			return time.Date(2026, time.September, 16, 12, 0, 0, 0, time.UTC)
		},
		prepareWorkspace: func(ctx context.Context, _, workspace string) error {
			if err := os.MkdirAll(filepath.Join(workspace, "app_docs"), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(
				filepath.Join(workspace, "app_docs", "feature-x.md"),
				[]byte("# Existing document\n"),
				0o644,
			); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Join(workspace, "docs"), 0o755); err != nil {
				return err
			}
			if _, err := runExternalCommand(ctx, workspace, "git", "init", "--quiet"); err != nil {
				return err
			}
			if _, err := runExternalCommand(ctx, workspace, "git", "add", "--all", "--force"); err != nil {
				return err
			}
			_, err := runExternalCommand(
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
				"baseline",
			)
			return err
		},
	}
}

func measuredTestCase() Case {
	return Case{
		ID:       "measured-test",
		Agent:    "team-lead",
		Task:     "Implement the requested change.",
		Live:     true,
		Workflow: true,
		Assertions: []Assertion{
			{Type: "contains", Value: "builder: done"},
			{Type: "contains", Value: "validator: PASS"},
			{Type: "contains", Value: "code-reviewer: APPROVE"},
		},
	}
}

func measuredTestConfig(t *testing.T) MeasuredWorkflowConfig {
	t.Helper()
	config, err := LoadMeasuredWorkflowConfig(repoRoot(t), "stage4-team-lead")
	if err != nil {
		t.Fatal(err)
	}
	return config
}

func successfulMeasuredResponses() []ProviderResponse {
	outputs := []string{
		`{"status":"done","summary":"built secure x.go","files":["x.go"],"verification":"go test"}`,
		`{"decision":"PASS","reason":"tests pass","issues":[]}`,
		`{"decision":"APPROVE","reason":"sound","findings":[]}`,
		`{"decision":"PASS","reason":"review verified","issues":[]}`,
		`{"status":"done","path":"app_docs/feature-x.md","summary":"documented"}`,
	}
	responses := make([]ProviderResponse, len(outputs))
	for index, output := range outputs {
		responses[index] = measuredResponse(output, 0.01, index+1)
	}
	return responses
}

func measuredResponse(output string, cost float64, id int) ProviderResponse {
	model := "claude-test"
	return ProviderResponse{
		Output:             output,
		ProviderModels:     []string{model},
		ProviderResponseID: fmt.Sprintf("response-%d", id),
		ProviderSessionID:  fmt.Sprintf("session-%d", id),
		ProviderModelUsage: map[string]map[string]json.RawMessage{
			model: {
				"inputTokens":  json.RawMessage("10"),
				"outputTokens": json.RawMessage("5"),
				"costUSD":      json.RawMessage(fmt.Sprintf("%.8f", cost)),
			},
		},
		InputTokens:  10,
		OutputTokens: 5,
		CostUSD:      cost,
		Duration:     time.Millisecond,
		TelemetryPresence: TelemetryPresence{
			ModelUsage:         true,
			InputTokens:        true,
			OutputTokens:       true,
			CostUSD:            true,
			DurationMS:         true,
			ProviderResponseID: true,
			ProviderSessionID:  true,
		},
	}
}

type sequenceExecutor struct {
	responses []ProviderResponse
	errors    []error
	requests  []ProviderRequest
	calls     int
	onExecute func(int, ProviderRequest)
}

func (executor *sequenceExecutor) Execute(
	_ context.Context,
	request ProviderRequest,
) (ProviderResponse, error) {
	executor.requests = append(executor.requests, request)
	index := executor.calls
	executor.calls++
	if executor.onExecute != nil {
		executor.onExecute(index, request)
	}
	if index >= len(executor.responses) {
		return ProviderResponse{}, fmt.Errorf("unexpected provider invocation %d", executor.calls)
	}
	var err error
	if index < len(executor.errors) {
		err = executor.errors[index]
	}
	return executor.responses[index], err
}

func TestWriteRunMaterializesMeasuredAuditAndSentinels(t *testing.T) {
	root := t.TempDir()
	result, err := measuredTestRunner(
		t,
		&sequenceExecutor{responses: successfulMeasuredResponses()},
	).Run(
		context.Background(),
		measuredTestCase(),
		measuredTestConfig(t),
		2.50,
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	run := NewRun(time.Now(), "test", 2.50, []CaseResult{result})
	directory, err := WriteRun(root, run)
	if err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{
		"workflow/measured-test/audit.jsonl",
		"workflow/measured-test/sentinels/builder-attempt-1.passed.json",
	} {
		if _, err := os.Stat(filepath.Join(directory, relative)); err != nil {
			t.Fatalf("missing %s: %v", relative, err)
		}
	}
	read, err := ReadRun(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Cases) != 1 || read.Cases[0].Workflow == nil ||
		read.Cases[0].Workflow.Status != WorkflowPendingHuman ||
		len(read.Cases[0].Workflow.Steps) != 5 ||
		read.Cases[0].Workflow.Totals != result.Workflow.Totals {
		t.Fatalf("ReadRun() = %#v, want measured CaseResult and totals round-trip", read)
	}
}
