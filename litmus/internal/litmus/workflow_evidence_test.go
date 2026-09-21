package litmus

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkflowWorkspaceHashBindsUntrackedContents(t *testing.T) {
	workspace := t.TempDir()
	if _, err := runExternalCommand(context.Background(), workspace, "git", "init", "--quiet"); err != nil {
		t.Fatal(err)
	}
	if _, err := runExternalCommand(
		context.Background(),
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
	); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(workspace, "untracked.txt")
	if err := os.WriteFile(path, []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := workflowWorkspaceHash(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := workflowWorkspaceHash(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("workspace hash did not change with untracked file contents")
	}
}

func TestWorkflowWorkspaceHashIncludesIgnoredFilesAndStableGitMetadata(t *testing.T) {
	workspace := t.TempDir()
	if _, err := runExternalCommand(context.Background(), workspace, "git", "init", "--quiet"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".gitignore"), []byte("ignored.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runExternalCommand(context.Background(), workspace, "git", "add", ".gitignore"); err != nil {
		t.Fatal(err)
	}
	if _, err := runExternalCommand(
		context.Background(),
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
		"-m",
		"baseline",
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "ignored.txt"), []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	base, err := workflowWorkspaceHash(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}

	mutateAndRestore := func(path string, contents []byte) {
		t.Helper()
		original, readErr := os.ReadFile(path)
		existed := readErr == nil
		if readErr != nil && !os.IsNotExist(readErr) {
			t.Fatal(readErr)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, contents, 0o644); err != nil {
			t.Fatal(err)
		}
		mutated, err := workflowWorkspaceHash(context.Background(), workspace)
		if err != nil {
			t.Fatal(err)
		}
		if mutated == base {
			t.Fatalf("workflow hash did not change for %s", path)
		}
		if existed {
			if err := os.WriteFile(path, original, 0o644); err != nil {
				t.Fatal(err)
			}
		} else if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		restored, err := workflowWorkspaceHash(context.Background(), workspace)
		if err != nil {
			t.Fatal(err)
		}
		if restored != base {
			t.Fatalf("workflow hash did not restore after %s", path)
		}
	}

	mutateAndRestore(filepath.Join(workspace, "ignored.txt"), []byte("second"))
	mutateAndRestore(filepath.Join(workspace, ".git", "config"), []byte("[core]\n\trepositoryformatversion = 1\n"))
	mutateAndRestore(filepath.Join(workspace, ".git", "hooks", "pre-commit"), []byte("#!/bin/sh\nexit 1\n"))
	mutateAndRestore(filepath.Join(workspace, ".git", "HEAD"), []byte("ref: refs/heads/tampered\n"))
	mutateAndRestore(filepath.Join(workspace, ".git", "info", "exclude"), []byte("tampered\n"))

	branch, err := runExternalCommand(context.Background(), workspace, "git", "symbolic-ref", "--short", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	refPath := filepath.Join(workspace, ".git", "refs", "heads", strings.TrimSpace(string(branch)))
	refContents, err := os.ReadFile(refPath)
	if err != nil {
		t.Fatal(err)
	}
	mutateAndRestore(refPath, append(append([]byte(nil), refContents...), '\n'))

	volatilePaths := []string{
		filepath.Join(workspace, ".git", "index"),
		filepath.Join(workspace, ".git", "logs", "HEAD"),
		filepath.Join(workspace, ".git", "index.lock"),
	}
	for _, path := range volatilePaths {
		original, readErr := os.ReadFile(path)
		existed := readErr == nil
		if readErr != nil && !os.IsNotExist(readErr) {
			t.Fatal(readErr)
		}
		if err := os.WriteFile(path, []byte("volatile change"), 0o644); err != nil {
			t.Fatal(err)
		}
		unchanged, err := workflowWorkspaceHash(context.Background(), workspace)
		if err != nil {
			t.Fatal(err)
		}
		if unchanged != base {
			t.Fatalf("workflow hash changed for volatile Git path %s", path)
		}
		if existed {
			if err := os.WriteFile(path, original, 0o644); err != nil {
				t.Fatal(err)
			}
		} else if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWorkflowEvidenceRejectsTamperedSentinel(t *testing.T) {
	store, err := newWorkflowEvidenceStore(t.TempDir(), "workflow", "run", "case")
	if err != nil {
		t.Fatal(err)
	}
	step := WorkflowStepResult{
		Sequence: 1, StepID: "builder", Agent: "builder", Attempt: 1,
		Status: "ACCEPTED", InputHash: strings.Repeat("a", 64),
		OutputHash: strings.Repeat("b", 64), WorkspaceHash: strings.Repeat("c", 64),
		ProviderInvocationHash: strings.Repeat("d", 64),
	}
	sentinel, err := store.writeSentinel(step, nil, "2026-09-16T12:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	path := store.sentinelPath("builder", 1)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	contents = []byte(strings.Replace(string(contents), strings.Repeat("b", 64), strings.Repeat("e", 64), 1))
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.requireSentinel(sentinel, step.WorkspaceHash); err == nil ||
		!strings.Contains(err.Error(), "tampered") {
		t.Fatalf("requireSentinel() error = %v, want tamper rejection", err)
	}
}

func TestWorkflowEvidenceAuditIsHashChained(t *testing.T) {
	store, err := newWorkflowEvidenceStore(t.TempDir(), "workflow", "run", "case")
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.append(WorkflowAuditRecord{
		Timestamp: "2026-09-16T12:00:00Z", StepID: "builder", Agent: "builder",
		Attempt: 1, InputHash: strings.Repeat("a", 64), OutputHash: strings.Repeat("b", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.append(WorkflowAuditRecord{
		Timestamp: "2026-09-16T12:00:01Z", StepID: "validator", Agent: "validator",
		Attempt: 1, InputHash: strings.Repeat("c", 64), OutputHash: strings.Repeat("d", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.PreviousRecordHash != first.RecordHash {
		t.Fatalf("second predecessor = %q, want %q", second.PreviousRecordHash, first.RecordHash)
	}
	if err := store.verifyAudit(); err != nil {
		t.Fatal(err)
	}
	path := store.auditPath
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(contents), `"builder"`, `"tampered"`, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.verifyAudit(); err == nil {
		t.Fatal("verifyAudit() error = nil, want hash-chain tamper rejection")
	}
	if filepath.Base(store.sentinelPath("builder", 1)) != "builder-attempt-1.passed.json" {
		t.Fatal("unexpected sentinel naming")
	}
}

func TestWorkflowEvidenceRejectsRetryHistoryThatContinuesAfterRetryGate(t *testing.T) {
	result, err := measuredTestRunner(
		t,
		&sequenceExecutor{responses: append(
			[]ProviderResponse{
				measuredResponse(`{"status":"done","summary":"first","files":["x.go"],"verification":"go test"}`, 0.01, 1),
				measuredResponse(`{"decision":"FAIL","reason":"failed","issues":["issue"]}`, 0.01, 2),
			},
			successfulMeasuredResponses()...,
		)},
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
	tampered.Steps[2].Attempt = 1
	if err := validateWorkflowEvidence(tampered); err == nil {
		t.Fatal("validateWorkflowEvidence() error = nil, want invalid retry history rejected")
	}
}

func TestProviderInvocationHashUsesPortableCanonicalFields(t *testing.T) {
	step := WorkflowStepResult{
		StepID:             "builder",
		Agent:              "builder",
		Attempt:            1,
		BudgetUSD:          0.75,
		RequestedModel:     "claude-sonnet-4-5",
		ActualModels:       []string{"claude-test"},
		ProviderResponseID: "response-1",
		ProviderSessionID:  "session-1",
		ProviderModelUsage: map[string]map[string]json.RawMessage{
			"claude-test": {
				"inputTokens":  json.RawMessage("10"),
				"outputTokens": json.RawMessage("5"),
				"costUSD":      json.RawMessage("0.01000000"),
			},
		},
		TelemetryPresence: TelemetryPresence{
			ModelUsage:         true,
			InputTokens:        true,
			OutputTokens:       true,
			CostUSD:            true,
			DurationMS:         true,
			ProviderResponseID: true,
			ProviderSessionID:  true,
		},
		InputTokens:   10,
		OutputTokens:  5,
		CostUSD:       0.01,
		DurationMS:    1,
		InputHash:     strings.Repeat("a", 64),
		OutputHash:    strings.Repeat("b", 64),
		WorkspaceHash: strings.Repeat("c", 64),
	}
	got, err := providerInvocationHash(step)
	if err != nil {
		t.Fatal(err)
	}
	const want = "728c9e0f439a91852e714468022cd7afaaa5de3992f7adac16fb6f0d8f5550f2"
	if got != want {
		t.Fatalf("providerInvocationHash() = %q, want portable hash %q", got, want)
	}
}
