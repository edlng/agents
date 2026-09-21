package litmus

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

type workflowEvidenceStore struct {
	root       string
	auditPath  string
	sentinels  string
	workflowID string
	runID      string
	caseID     string
	records    []WorkflowAuditRecord
	written    []WorkflowSentinel
}

func newWorkflowEvidenceStore(root, workflowID, runID, caseID string) (*workflowEvidenceStore, error) {
	if strings.TrimSpace(workflowID) == "" || strings.TrimSpace(runID) == "" ||
		strings.TrimSpace(caseID) == "" {
		return nil, fmt.Errorf("workflow evidence identity is required")
	}
	if err := os.MkdirAll(filepath.Join(root, "sentinels"), 0o755); err != nil {
		return nil, fmt.Errorf("create workflow sentinel directory: %w", err)
	}
	auditPath := filepath.Join(root, "audit.jsonl")
	if err := os.WriteFile(auditPath, nil, 0o644); err != nil {
		return nil, fmt.Errorf("create workflow audit log: %w", err)
	}
	return &workflowEvidenceStore{
		root:       root,
		auditPath:  auditPath,
		sentinels:  filepath.Join(root, "sentinels"),
		workflowID: workflowID,
		runID:      runID,
		caseID:     caseID,
	}, nil
}

func (store *workflowEvidenceStore) append(record WorkflowAuditRecord) (WorkflowAuditRecord, error) {
	record.SchemaVersion = MeasuredWorkflowSchemaVersion
	record.WorkflowID = store.workflowID
	record.RunID = store.runID
	record.CaseID = store.caseID
	if record.RecordType == "" {
		record.RecordType = WorkflowAuditStepRecord
	}
	record.Sequence = len(store.records) + 1
	if len(store.records) > 0 {
		record.PreviousRecordHash = store.records[len(store.records)-1].RecordHash
	}
	record.RecordHash = ""
	digest, err := hashJSON(record)
	if err != nil {
		return WorkflowAuditRecord{}, err
	}
	record.RecordHash = digest
	encoded, err := json.Marshal(record)
	if err != nil {
		return WorkflowAuditRecord{}, fmt.Errorf("encode workflow audit record: %w", err)
	}
	file, err := os.OpenFile(store.auditPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return WorkflowAuditRecord{}, fmt.Errorf("open workflow audit log: %w", err)
	}
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		_ = file.Close()
		return WorkflowAuditRecord{}, fmt.Errorf("append workflow audit log: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return WorkflowAuditRecord{}, fmt.Errorf("sync workflow audit log: %w", err)
	}
	if err := file.Close(); err != nil {
		return WorkflowAuditRecord{}, fmt.Errorf("close workflow audit log: %w", err)
	}
	store.records = append(store.records, record)
	return record, nil
}

func serializeWorkflowFailure(failure *WorkflowFailure) (string, error) {
	if failure == nil {
		return "null", nil
	}
	encoded, err := json.Marshal(failure)
	if err != nil {
		return "", fmt.Errorf("encode workflow failure: %w", err)
	}
	encoded, err = canonicalJSONBytes(encoded)
	if err != nil {
		return "", fmt.Errorf("canonicalize workflow failure: %w", err)
	}
	return string(encoded), nil
}

func workflowTerminalDecision(workflow WorkflowResult) string {
	if workflow.Failure != nil {
		return workflow.Failure.Code
	}
	if workflow.Status == WorkflowPendingHuman {
		return WorkflowPendingHuman
	}
	return workflow.Status
}

func workflowTerminalPayload(workflow WorkflowResult, failureJSON string) (string, error) {
	encoded, err := json.Marshal(struct {
		Status      string `json:"status"`
		Decision    string `json:"decision"`
		FailureJSON string `json:"failure_json"`
	}{
		Status:      workflow.Status,
		Decision:    workflowTerminalDecision(workflow),
		FailureJSON: failureJSON,
	})
	if err != nil {
		return "", fmt.Errorf("encode workflow terminal payload: %w", err)
	}
	encoded, err = canonicalJSONBytes(encoded)
	if err != nil {
		return "", fmt.Errorf("canonicalize workflow terminal payload: %w", err)
	}
	return string(encoded), nil
}

func (store *workflowEvidenceStore) appendTerminal(
	workflow WorkflowResult,
) (WorkflowAuditRecord, error) {
	failureJSON, err := serializeWorkflowFailure(workflow.Failure)
	if err != nil {
		return WorkflowAuditRecord{}, err
	}
	payload, err := workflowTerminalPayload(workflow, failureJSON)
	if err != nil {
		return WorkflowAuditRecord{}, err
	}
	workspaceHash := hashWorkflowText("")
	if len(workflow.Steps) > 0 {
		workspaceHash = workflow.Steps[len(workflow.Steps)-1].WorkspaceHash
	}
	return store.append(WorkflowAuditRecord{
		RecordType:             WorkflowAuditTerminalRecord,
		Timestamp:              time.Now().UTC().Format(time.RFC3339Nano),
		StepID:                 "terminal",
		Agent:                  "workflow",
		Attempt:                workflow.FinalAttempt,
		Status:                 workflow.Status,
		Decision:               workflowTerminalDecision(workflow),
		InputHash:              hashWorkflowText(workflow.Status),
		OutputHash:             hashWorkflowText(payload),
		WorkspaceHash:          workspaceHash,
		ProviderInvocationHash: hashWorkflowText("terminal"),
		TerminalFailureJSON:    failureJSON,
	})
}

func (store *workflowEvidenceStore) resealTerminal(workflow WorkflowResult) error {
	if len(store.records) == 0 ||
		store.records[len(store.records)-1].RecordType != WorkflowAuditTerminalRecord {
		return fmt.Errorf("workflow audit has no terminal record to replace")
	}
	records := append([]WorkflowAuditRecord(nil), store.records[:len(store.records)-1]...)
	temporary, err := os.CreateTemp(filepath.Dir(store.auditPath), ".audit-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary workflow audit log: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	for _, record := range records {
		encoded, err := json.Marshal(record)
		if err != nil {
			_ = temporary.Close()
			return fmt.Errorf("encode workflow audit record: %w", err)
		}
		if _, err := temporary.Write(append(encoded, '\n')); err != nil {
			_ = temporary.Close()
			return fmt.Errorf("rewrite workflow audit log: %w", err)
		}
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync rewritten workflow audit log: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close rewritten workflow audit log: %w", err)
	}
	if err := os.Rename(temporaryPath, store.auditPath); err != nil {
		return fmt.Errorf("replace workflow audit log: %w", err)
	}
	store.records = records
	if _, err := store.appendTerminal(workflow); err != nil {
		return fmt.Errorf("append resealed workflow terminal audit record: %w", err)
	}
	return nil
}

func (store *workflowEvidenceStore) writeSentinel(
	step WorkflowStepResult,
	predecessor *WorkflowSentinel,
	createdAt string,
) (WorkflowSentinel, error) {
	sentinel := WorkflowSentinel{
		SchemaVersion:          MeasuredWorkflowSchemaVersion,
		WorkflowID:             store.workflowID,
		RunID:                  store.runID,
		CaseID:                 store.caseID,
		Sequence:               step.Sequence,
		StepID:                 step.StepID,
		Agent:                  step.Agent,
		Attempt:                step.Attempt,
		Status:                 step.Status,
		InputHash:              step.InputHash,
		OutputHash:             step.OutputHash,
		WorkspaceHash:          step.WorkspaceHash,
		ProviderInvocationHash: step.ProviderInvocationHash,
		CreatedAt:              createdAt,
	}
	if predecessor != nil {
		sentinel.PredecessorHash = predecessor.SentinelHash
	}
	digest, err := hashSentinel(sentinel)
	if err != nil {
		return WorkflowSentinel{}, err
	}
	sentinel.SentinelHash = digest
	path := store.sentinelPath(sentinel.StepID, sentinel.Attempt)
	temporary, err := os.CreateTemp(store.sentinels, ".sentinel-*.tmp")
	if err != nil {
		return WorkflowSentinel{}, fmt.Errorf("create temporary sentinel: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(sentinel); err != nil {
		_ = temporary.Close()
		return WorkflowSentinel{}, fmt.Errorf("encode workflow sentinel: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return WorkflowSentinel{}, fmt.Errorf("sync workflow sentinel: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return WorkflowSentinel{}, fmt.Errorf("close workflow sentinel: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return WorkflowSentinel{}, fmt.Errorf("publish workflow sentinel: %w", err)
	}
	store.written = append(store.written, sentinel)
	return sentinel, nil
}

func (store *workflowEvidenceStore) requireSentinel(
	expected WorkflowSentinel,
	workspaceHash string,
) error {
	path := store.sentinelPath(expected.StepID, expected.Attempt)
	contents, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("required predecessor sentinel %s: %w", expected.StepID, err)
	}
	var actual WorkflowSentinel
	decoder := json.NewDecoder(strings.NewReader(string(contents)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&actual); err != nil {
		return fmt.Errorf("decode predecessor sentinel %s: %w", expected.StepID, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("decode predecessor sentinel %s: trailing JSON", expected.StepID)
	}
	if actual.SentinelHash == "" {
		return fmt.Errorf("predecessor sentinel %s has no hash", expected.StepID)
	}
	digest, err := hashSentinel(actual)
	if err != nil {
		return err
	}
	expectedBytes, _ := json.Marshal(expected)
	actualBytes, _ := json.Marshal(actual)
	if digest != actual.SentinelHash || string(expectedBytes) != string(actualBytes) {
		return fmt.Errorf("predecessor sentinel %s is stale or tampered", expected.StepID)
	}
	if actual.WorkspaceHash != workspaceHash {
		return fmt.Errorf("workspace changed after predecessor sentinel %s", expected.StepID)
	}
	return nil
}

func (store *workflowEvidenceStore) sentinelPath(stepID string, attempt int) string {
	return filepath.Join(store.sentinels, fmt.Sprintf("%s-attempt-%d.passed.json", stepID, attempt))
}

func (store *workflowEvidenceStore) verifyAudit() error {
	file, err := os.Open(store.auditPath)
	if err != nil {
		return fmt.Errorf("open workflow audit log: %w", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	var previous string
	var count int
	for scanner.Scan() {
		count++
		var record WorkflowAuditRecord
		decoder := json.NewDecoder(strings.NewReader(scanner.Text()))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&record); err != nil {
			return fmt.Errorf("decode audit record %d: %w", count, err)
		}
		digest := record.RecordHash
		record.RecordHash = ""
		computed, err := hashJSON(record)
		if err != nil {
			return err
		}
		if digest != computed || record.Sequence != count || record.PreviousRecordHash != previous {
			return fmt.Errorf("workflow audit chain is invalid at sequence %d", count)
		}
		previous = digest
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read workflow audit log: %w", err)
	}
	if count != len(store.records) {
		return fmt.Errorf("workflow audit record count mismatch")
	}
	return nil
}

func workflowWorkspaceHash(ctx context.Context, workspace string) (string, error) {
	digest := sha256.New()
	err := filepath.WalkDir(workspace, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relative, err := filepath.Rel(workspace, path)
		if err != nil {
			return fmt.Errorf("relativize workflow path: %w", err)
		}
		if relative == "." {
			return nil
		}
		relative = filepath.ToSlash(relative)
		if skipWorkflowHashPath(relative) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if err := validateRelativePath(relative); err != nil {
			return fmt.Errorf("hash workflow path %q: %w", relative, err)
		}
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("inspect workflow path %q: %w", relative, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("workflow workspace must not contain symlinks: %s", relative)
		}
		writeWorkspaceHashField(digest, []byte(relative))
		writeWorkspaceHashField(digest, []byte(strconv.FormatUint(uint64(info.Mode()), 10)))
		switch {
		case info.IsDir():
			writeWorkspaceHashField(digest, []byte("directory"))
		case info.Mode().IsRegular():
			writeWorkspaceHashField(digest, []byte("file"))
			writeWorkspaceHashField(digest, []byte(strconv.FormatInt(info.Size(), 10)))
			file, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("open workflow file %q: %w", relative, err)
			}
			buffer := make([]byte, 64*1024)
			for {
				if err := ctx.Err(); err != nil {
					_ = file.Close()
					return err
				}
				count, readErr := file.Read(buffer)
				if count > 0 {
					_, _ = digest.Write(buffer[:count])
				}
				if errors.Is(readErr, io.EOF) {
					break
				}
				if readErr != nil {
					_ = file.Close()
					return fmt.Errorf("read workflow file %q: %w", relative, readErr)
				}
			}
			if err := file.Close(); err != nil {
				return fmt.Errorf("close workflow file %q: %w", relative, err)
			}
		default:
			return fmt.Errorf("workflow path %q must be a regular file or directory", relative)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("capture workflow filesystem: %w", err)
	}
	return fmt.Sprintf("%x", digest.Sum(nil)), nil
}

func skipWorkflowHashPath(relative string) bool {
	if relative == ".git/index" ||
		relative == ".git/logs" ||
		relative == ".git/objects" ||
		relative == ".git/FETCH_HEAD" ||
		relative == ".git/ORIG_HEAD" ||
		relative == ".git/MERGE_HEAD" ||
		relative == ".git/CHERRY_PICK_HEAD" ||
		relative == ".git/REVERT_HEAD" ||
		relative == ".git/BISECT_LOG" {
		return true
	}
	if strings.HasPrefix(relative, ".git/logs/") ||
		strings.HasPrefix(relative, ".git/objects/") {
		return true
	}
	if strings.HasPrefix(relative, ".git/") && strings.HasSuffix(relative, ".lock") {
		return true
	}
	return false
}

func writeWorkspaceHashField(writer io.Writer, value []byte) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = writer.Write(length[:])
	_, _ = writer.Write(value)
}

func protectedWorkflowDefinitionHash(workspace string) (string, error) {
	digest := sha256.New()
	for _, relativeRoot := range []string{".claude/agents", ".claude/skills"} {
		root := filepath.Join(workspace, filepath.FromSlash(relativeRoot))
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return "", fmt.Errorf("inspect protected workflow definitions: %w", err)
		}
		if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("protected workflow definitions must not contain symlinks")
			}
			if entry.IsDir() {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("protected workflow definition must be a regular file")
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(workspace, path)
			if err != nil {
				return err
			}
			_, _ = digest.Write([]byte(filepath.ToSlash(relative)))
			_, _ = digest.Write([]byte{0})
			_, _ = digest.Write(contents)
			_, _ = digest.Write([]byte{0})
			return nil
		}); err != nil {
			return "", fmt.Errorf("hash protected workflow definitions: %w", err)
		}
	}
	return fmt.Sprintf("%x", digest.Sum(nil)), nil
}

func hashSentinel(sentinel WorkflowSentinel) (string, error) {
	sentinel.SentinelHash = ""
	return hashJSON(sentinel)
}

func canonicalJSONBytes(value []byte) ([]byte, error) {
	decoder := json.NewDecoder(strings.NewReader(string(value)))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("multiple JSON values")
		}
		return nil, err
	}
	return json.Marshal(decoded)
}

func hashJSON(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode hash input: %w", err)
	}
	encoded, err = canonicalJSONBytes(encoded)
	if err != nil {
		return "", fmt.Errorf("canonicalize hash input: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return fmt.Sprintf("%x", digest), nil
}

func hashWorkflowText(value string) string {
	digest := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", digest)
}

func providerInvocationHash(step WorkflowStepResult) (string, error) {
	modelUsage := make(map[string]map[string]any, len(step.ProviderModelUsage))
	for model, usage := range step.ProviderModelUsage {
		normalized := make(map[string]any, len(usage))
		for field, raw := range usage {
			var value any
			if err := json.Unmarshal(raw, &value); err != nil {
				return "", fmt.Errorf(
					"decode provider model usage %s.%s for hashing: %w",
					model,
					field,
					err,
				)
			}
			normalized[field] = value
		}
		modelUsage[model] = normalized
	}
	return hashJSON(struct {
		StepID             string                    `json:"step_id"`
		Agent              string                    `json:"agent"`
		Attempt            int                       `json:"attempt"`
		BudgetUSD          float64                   `json:"budget_usd"`
		RequestedModel     string                    `json:"requested_model"`
		ActualModels       []string                  `json:"actual_models"`
		ProviderRequestID  string                    `json:"provider_request_id"`
		ProviderResponseID string                    `json:"provider_response_id"`
		ProviderSessionID  string                    `json:"provider_session_id"`
		ProviderModelUsage map[string]map[string]any `json:"provider_model_usage"`
		TelemetryPresence  TelemetryPresence         `json:"telemetry_presence"`
		InputTokens        int                       `json:"input_tokens"`
		OutputTokens       int                       `json:"output_tokens"`
		CostUSD            float64                   `json:"cost_usd"`
		DurationMS         int64                     `json:"duration_ms"`
		InputHash          string                    `json:"input_hash"`
		OutputHash         string                    `json:"output_hash"`
		WorkspaceHash      string                    `json:"workspace_hash"`
	}{
		StepID:             step.StepID,
		Agent:              step.Agent,
		Attempt:            step.Attempt,
		BudgetUSD:          step.BudgetUSD,
		RequestedModel:     step.RequestedModel,
		ActualModels:       step.ActualModels,
		ProviderRequestID:  step.ProviderRequestID,
		ProviderResponseID: step.ProviderResponseID,
		ProviderSessionID:  step.ProviderSessionID,
		ProviderModelUsage: modelUsage,
		TelemetryPresence:  step.TelemetryPresence,
		InputTokens:        step.InputTokens,
		OutputTokens:       step.OutputTokens,
		CostUSD:            step.CostUSD,
		DurationMS:         step.DurationMS,
		InputHash:          step.InputHash,
		OutputHash:         step.OutputHash,
		WorkspaceHash:      step.WorkspaceHash,
	})
}

func validateWorkflowEvidence(workflow WorkflowResult) error {
	if err := reconcileWorkflowTotals(workflow); err != nil {
		return err
	}
	if len(workflow.Audit) != len(workflow.Steps)+1 {
		return fmt.Errorf("workflow audit count must equal step records plus terminal record")
	}
	if err := validateWorkflowStepSequence(workflow); err != nil {
		return err
	}
	previousRecordHash := ""
	for index, record := range workflow.Audit {
		if record.Sequence != index+1 || record.PreviousRecordHash != previousRecordHash {
			return fmt.Errorf("workflow audit chain is invalid at sequence %d", index+1)
		}
		if index == len(workflow.Steps) {
			failureJSON, err := serializeWorkflowFailure(workflow.Failure)
			if err != nil {
				return err
			}
			payload, err := workflowTerminalPayload(workflow, failureJSON)
			if err != nil {
				return err
			}
			workspaceHash := hashWorkflowText("")
			if len(workflow.Steps) > 0 {
				workspaceHash = workflow.Steps[len(workflow.Steps)-1].WorkspaceHash
			}
			if record.RecordType != WorkflowAuditTerminalRecord ||
				record.SchemaVersion != MeasuredWorkflowSchemaVersion ||
				record.WorkflowID != workflow.WorkflowID ||
				record.RunID != workflow.RunID ||
				record.CaseID != workflow.CaseID ||
				record.StepID != "terminal" ||
				record.Agent != "workflow" ||
				record.Attempt != workflow.FinalAttempt ||
				record.Status != workflow.Status ||
				record.Decision != workflowTerminalDecision(workflow) ||
				record.InputHash != hashWorkflowText(workflow.Status) ||
				record.OutputHash != hashWorkflowText(payload) ||
				record.WorkspaceHash != workspaceHash ||
				record.ProviderInvocationHash != hashWorkflowText("terminal") ||
				record.TerminalFailureJSON != failureJSON ||
				record.Error != "" {
				return fmt.Errorf("workflow terminal audit record does not match workflow result")
			}
		} else {
			step := workflow.Steps[index]
			if record.RecordType != WorkflowAuditStepRecord ||
				record.SchemaVersion != MeasuredWorkflowSchemaVersion ||
				record.WorkflowID != workflow.WorkflowID ||
				record.RunID != workflow.RunID ||
				record.CaseID != workflow.CaseID ||
				record.StepID != step.StepID ||
				record.Agent != step.Agent ||
				record.Attempt != step.Attempt ||
				record.Status != step.Status ||
				record.Decision != step.Decision ||
				record.InputHash != step.InputHash ||
				record.OutputHash != step.OutputHash ||
				record.WorkspaceHash != step.WorkspaceHash ||
				record.ProviderInvocationHash != step.ProviderInvocationHash ||
				record.Error != step.Error {
				return fmt.Errorf("workflow audit record does not match step %d", index+1)
			}
		}
		recordHash := record.RecordHash
		record.RecordHash = ""
		expected, err := hashJSON(record)
		if err != nil {
			return err
		}
		if recordHash != expected {
			return fmt.Errorf("workflow audit hash is invalid at sequence %d", index+1)
		}
		previousRecordHash = recordHash
	}

	accepted := map[string]WorkflowStepResult{}
	for _, step := range workflow.Steps {
		if step.Sequence < 1 || step.Attempt < 1 || step.Attempt > workflow.MaxAttempts {
			return fmt.Errorf("workflow step has an invalid sequence or attempt")
		}
		if step.OutputHash != hashWorkflowText(step.Output) {
			return fmt.Errorf(
				"workflow step %s attempt %d output hash does not match persisted output",
				step.StepID,
				step.Attempt,
			)
		}
		expectedInvocationHash, err := providerInvocationHash(step)
		if err != nil {
			return err
		}
		if step.ProviderInvocationHash != expectedInvocationHash {
			return fmt.Errorf(
				"workflow step %s attempt %d provider invocation hash is invalid",
				step.StepID,
				step.Attempt,
			)
		}
		switch step.Status {
		case "ACCEPTED", "RETRY", "BLOCKED":
			if err := validateMeasuredTelemetry(ProviderResponse{
				ProviderModels:     step.ActualModels,
				ProviderRequestID:  step.ProviderRequestID,
				ProviderResponseID: step.ProviderResponseID,
				ProviderSessionID:  step.ProviderSessionID,
				ProviderModelUsage: step.ProviderModelUsage,
				InputTokens:        step.InputTokens,
				OutputTokens:       step.OutputTokens,
				CostUSD:            step.CostUSD,
				Duration:           time.Duration(step.DurationMS) * time.Millisecond,
				TelemetryPresence:  step.TelemetryPresence,
			}, step.BudgetUSD, step.Status != "BLOCKED"); err != nil {
				return fmt.Errorf("workflow step %s telemetry: %w", step.StepID, err)
			}
		default:
			return fmt.Errorf("workflow step %s has unsupported status %q", step.StepID, step.Status)
		}
		if step.Status == "ACCEPTED" {
			accepted[fmt.Sprintf("%d:%s", step.Attempt, step.StepID)] = step
		}
	}
	byAttempt := map[int][]WorkflowSentinel{}
	for _, sentinel := range workflow.Sentinels {
		expectedHash, err := hashSentinel(sentinel)
		if err != nil {
			return err
		}
		key := fmt.Sprintf("%d:%s", sentinel.Attempt, sentinel.StepID)
		step, ok := accepted[key]
		if !ok || sentinel.SentinelHash != expectedHash ||
			sentinel.SchemaVersion != MeasuredWorkflowSchemaVersion ||
			sentinel.WorkflowID != workflow.WorkflowID ||
			sentinel.RunID != workflow.RunID ||
			sentinel.CaseID != workflow.CaseID ||
			sentinel.Sequence != step.Sequence ||
			sentinel.Agent != step.Agent ||
			sentinel.Status != step.Status ||
			sentinel.InputHash != step.InputHash ||
			sentinel.OutputHash != step.OutputHash ||
			sentinel.WorkspaceHash != step.WorkspaceHash ||
			sentinel.ProviderInvocationHash != step.ProviderInvocationHash {
			return fmt.Errorf("workflow sentinel %s attempt %d is invalid", sentinel.StepID, sentinel.Attempt)
		}
		delete(accepted, key)
		byAttempt[sentinel.Attempt] = append(byAttempt[sentinel.Attempt], sentinel)
	}
	if len(accepted) != 0 {
		return fmt.Errorf("accepted workflow step has no sentinel")
	}
	for _, sentinels := range byAttempt {
		sort.Slice(sentinels, func(left, right int) bool {
			return sentinels[left].Sequence < sentinels[right].Sequence
		})
		predecessorHash := ""
		for _, sentinel := range sentinels {
			if sentinel.PredecessorHash != predecessorHash {
				return fmt.Errorf("workflow sentinel predecessor chain is invalid")
			}
			predecessorHash = sentinel.SentinelHash
		}
	}
	if workflow.Status == WorkflowPendingHuman {
		want := []string{"builder", "validator", "code-reviewer", "review-challenge", "documenter"}
		wantDecisions := []string{"done", "PASS", "APPROVE", "PASS", "done"}
		var final []string
		var decisions []string
		for _, step := range workflow.Steps {
			if step.Attempt == workflow.FinalAttempt {
				final = append(final, step.StepID)
				decisions = append(decisions, step.Decision)
			}
		}
		finalStatuses := make([]string, 0, len(final))
		for _, step := range workflow.Steps {
			if step.Attempt == workflow.FinalAttempt {
				finalStatuses = append(finalStatuses, step.Status)
			}
		}
		if !slices.Equal(final, want) ||
			!slices.Equal(decisions, wantDecisions) ||
			!slices.Equal(finalStatuses, []string{"ACCEPTED", "ACCEPTED", "ACCEPTED", "ACCEPTED", "ACCEPTED"}) ||
			workflow.Failure != nil {
			return fmt.Errorf("PENDING_HUMAN requires exactly five ordered final-attempt steps")
		}
	}
	return nil
}

func validateWorkflowStepSequence(workflow WorkflowResult) error {
	if workflow.MaxAttempts < 1 || workflow.FinalAttempt < 1 ||
		workflow.FinalAttempt > workflow.MaxAttempts {
		return fmt.Errorf("workflow has an invalid attempt range")
	}
	if len(workflow.Steps) == 0 {
		if workflow.Status == WorkflowPendingHuman {
			return fmt.Errorf("PENDING_HUMAN requires measured workflow steps")
		}
		return nil
	}
	expectedOrder := []string{
		"builder",
		"validator",
		"code-reviewer",
		"review-challenge",
		"documenter",
	}
	stepsByAttempt := make(map[int][]WorkflowStepResult)
	lastAttempt := 0
	for index, step := range workflow.Steps {
		if step.Sequence != index+1 {
			return fmt.Errorf("workflow step sequence is invalid at position %d", index+1)
		}
		if step.Attempt < lastAttempt || step.Attempt > lastAttempt+1 {
			return fmt.Errorf("workflow attempts are not contiguous")
		}
		if lastAttempt == 0 && step.Attempt != 1 {
			return fmt.Errorf("workflow attempts must start at 1")
		}
		lastAttempt = step.Attempt
		stepsByAttempt[step.Attempt] = append(stepsByAttempt[step.Attempt], step)
	}
	if len(workflow.Steps) > 0 && lastAttempt != workflow.FinalAttempt {
		return fmt.Errorf("workflow final_attempt does not match its last step")
	}
	for attempt := 1; attempt <= workflow.FinalAttempt; attempt++ {
		steps := stepsByAttempt[attempt]
		if len(steps) == 0 || len(steps) > len(expectedOrder) {
			return fmt.Errorf("workflow attempt %d has an invalid number of steps", attempt)
		}
		for index, step := range steps {
			if step.StepID != expectedOrder[index] {
				return fmt.Errorf("workflow attempt %d step order is invalid", attempt)
			}
			if index < len(steps)-1 && step.Status == "RETRY" {
				return fmt.Errorf("workflow attempt %d continued after a retry decision", attempt)
			}
		}
		last := steps[len(steps)-1]
		if attempt < workflow.FinalAttempt {
			if last.Status != "RETRY" || !shouldRetryStep(last.StepID, last.Decision) {
				return fmt.Errorf("workflow attempt %d did not end at a retry gate", attempt)
			}
		}
	}
	return nil
}
