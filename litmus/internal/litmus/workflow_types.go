package litmus

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

const (
	MeasuredWorkflowSchemaVersion = "stage4.measured-workflow.v1"
	WorkflowPendingHuman          = "PENDING_HUMAN"
	WorkflowPartial               = "PARTIAL"
	WorkflowBlocked               = "BLOCKED"
	WorkflowAuditStepRecord       = "step"
	WorkflowAuditTerminalRecord   = "terminal"
)

// TelemetryPresence records which provider telemetry fields were actually
// supplied. Accepted and retry steps require complete measured telemetry;
// blocked steps may retain a well-formed subset.
type TelemetryPresence struct {
	ModelUsage         bool `json:"model_usage"`
	InputTokens        bool `json:"input_tokens"`
	OutputTokens       bool `json:"output_tokens"`
	CostUSD            bool `json:"cost_usd"`
	DurationMS         bool `json:"duration_ms"`
	ProviderRequestID  bool `json:"provider_request_id"`
	ProviderResponseID bool `json:"provider_response_id"`
	ProviderSessionID  bool `json:"provider_session_id"`
}

type MeasuredWorkflowConfig struct {
	SchemaVersion string                       `json:"schema_version"`
	WorkflowID    string                       `json:"workflow_id"`
	MaxAttempts   int                          `json:"max_attempts"`
	Steps         []MeasuredWorkflowStepConfig `json:"steps"`
}

type MeasuredWorkflowStepConfig struct {
	ID                    string          `json:"id"`
	Agent                 string          `json:"agent"`
	BudgetUSD             float64         `json:"budget_usd"`
	AllowWorkspaceChanges bool            `json:"allow_workspace_changes"`
	JSONSchema            json.RawMessage `json:"json_schema"`
}

type WorkflowResult struct {
	SchemaVersion     string                `json:"schema_version"`
	WorkflowID        string                `json:"workflow_id"`
	RunID             string                `json:"run_id"`
	CaseID            string                `json:"case_id"`
	Status            string                `json:"status"`
	FinalAttempt      int                   `json:"final_attempt"`
	MaxAttempts       int                   `json:"max_attempts"`
	Steps             []WorkflowStepResult  `json:"steps"`
	Audit             []WorkflowAuditRecord `json:"audit"`
	Sentinels         []WorkflowSentinel    `json:"sentinels"`
	Totals            WorkflowTotals        `json:"totals"`
	AuditPath         string                `json:"audit_path,omitempty"`
	SentinelDirectory string                `json:"sentinel_directory,omitempty"`
	Failure           *WorkflowFailure      `json:"failure,omitempty"`
}

type WorkflowStepResult struct {
	Sequence               int                                   `json:"sequence"`
	StepID                 string                                `json:"step_id"`
	Agent                  string                                `json:"agent"`
	Attempt                int                                   `json:"attempt"`
	Status                 string                                `json:"status"`
	Decision               string                                `json:"decision,omitempty"`
	BudgetUSD              float64                               `json:"budget_usd"`
	RequestedModel         string                                `json:"requested_model"`
	ActualModels           []string                              `json:"actual_models,omitempty"`
	ProviderRequestID      string                                `json:"provider_request_id,omitempty"`
	ProviderResponseID     string                                `json:"provider_response_id,omitempty"`
	ProviderSessionID      string                                `json:"provider_session_id,omitempty"`
	ProviderModelUsage     map[string]map[string]json.RawMessage `json:"provider_model_usage,omitempty"`
	TelemetryPresence      TelemetryPresence                     `json:"telemetry_presence"`
	InputTokens            int                                   `json:"input_tokens"`
	OutputTokens           int                                   `json:"output_tokens"`
	CostUSD                float64                               `json:"cost_usd"`
	DurationMS             int64                                 `json:"duration_ms"`
	InputHash              string                                `json:"input_hash"`
	OutputHash             string                                `json:"output_hash"`
	WorkspaceHash          string                                `json:"workspace_hash"`
	ProviderInvocationHash string                                `json:"provider_invocation_hash"`
	Output                 string                                `json:"output"`
	Error                  string                                `json:"error,omitempty"`
	SentinelPath           string                                `json:"sentinel_path,omitempty"`
}

type WorkflowTotals struct {
	Invocations  int     `json:"invocations"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	TotalTokens  int     `json:"total_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	DurationMS   int64   `json:"duration_ms"`
}

type WorkflowFailure struct {
	Code       string `json:"code"`
	OriginStep string `json:"origin_step,omitempty"`
	Attempt    int    `json:"attempt,omitempty"`
	Message    string `json:"message"`
}

type WorkflowAuditRecord struct {
	SchemaVersion          string `json:"schema_version"`
	WorkflowID             string `json:"workflow_id"`
	RunID                  string `json:"run_id"`
	CaseID                 string `json:"case_id"`
	Sequence               int    `json:"sequence"`
	Timestamp              string `json:"timestamp"`
	RecordType             string `json:"record_type"`
	StepID                 string `json:"step_id"`
	Agent                  string `json:"agent"`
	Attempt                int    `json:"attempt"`
	Status                 string `json:"status"`
	Decision               string `json:"decision,omitempty"`
	InputHash              string `json:"input_hash"`
	OutputHash             string `json:"output_hash"`
	WorkspaceHash          string `json:"workspace_hash"`
	ProviderInvocationHash string `json:"provider_invocation_hash"`
	PreviousRecordHash     string `json:"previous_record_hash,omitempty"`
	RecordHash             string `json:"record_hash"`
	Error                  string `json:"error,omitempty"`
	TerminalFailureJSON    string `json:"terminal_failure_json,omitempty"`
}

type WorkflowSentinel struct {
	SchemaVersion          string `json:"schema_version"`
	WorkflowID             string `json:"workflow_id"`
	RunID                  string `json:"run_id"`
	CaseID                 string `json:"case_id"`
	Sequence               int    `json:"sequence"`
	StepID                 string `json:"step_id"`
	Agent                  string `json:"agent"`
	Attempt                int    `json:"attempt"`
	Status                 string `json:"status"`
	InputHash              string `json:"input_hash"`
	OutputHash             string `json:"output_hash"`
	WorkspaceHash          string `json:"workspace_hash"`
	ProviderInvocationHash string `json:"provider_invocation_hash"`
	PredecessorHash        string `json:"predecessor_hash,omitempty"`
	CreatedAt              string `json:"created_at"`
	SentinelHash           string `json:"sentinel_hash"`
}

func LoadMeasuredWorkflowConfig(root, name string) (MeasuredWorkflowConfig, error) {
	if err := validateComponent("workflow name", name); err != nil {
		return MeasuredWorkflowConfig{}, err
	}
	var config MeasuredWorkflowConfig
	if err := loadJSON(
		root,
		filepath.Join(root, "litmus", "workflows", name+".json"),
		&config,
	); err != nil {
		return MeasuredWorkflowConfig{}, err
	}
	if err := validateMeasuredWorkflowConfig(config); err != nil {
		return MeasuredWorkflowConfig{}, err
	}
	return config, nil
}

func validateMeasuredWorkflowConfig(config MeasuredWorkflowConfig) error {
	if config.SchemaVersion != MeasuredWorkflowSchemaVersion {
		return fmt.Errorf("unsupported measured workflow schema_version %q", config.SchemaVersion)
	}
	if strings.TrimSpace(config.WorkflowID) == "" {
		return fmt.Errorf("workflow_id is required")
	}
	if config.MaxAttempts < 1 || config.MaxAttempts > 10 {
		return fmt.Errorf("max_attempts must be between 1 and 10")
	}
	want := []struct {
		id    string
		agent string
	}{
		{"builder", "builder"},
		{"validator", "validator"},
		{"code-reviewer", "code-reviewer"},
		{"review-challenge", "validator"},
		{"documenter", "documenter"},
	}
	if len(config.Steps) != len(want) {
		return fmt.Errorf("measured workflow must define exactly five steps")
	}
	for index, step := range config.Steps {
		if step.ID != want[index].id || step.Agent != want[index].agent {
			return fmt.Errorf(
				"workflow step %d must be %s using %s",
				index+1,
				want[index].id,
				want[index].agent,
			)
		}
		if !isFinite(step.BudgetUSD) || step.BudgetUSD <= 0 {
			return fmt.Errorf("workflow step %s budget_usd must be positive", step.ID)
		}
		if len(step.JSONSchema) == 0 || !json.Valid(step.JSONSchema) {
			return fmt.Errorf("workflow step %s json_schema must be valid JSON", step.ID)
		}
	}
	return nil
}
