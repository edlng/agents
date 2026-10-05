package audit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendVerifyAndSummary(t *testing.T) {
	root := t.TempDir()
	log, err := Create(root, "cid-1")
	if err != nil {
		t.Fatal(err)
	}
	log.Append(Event{Kind: KindLLMCall, Agent: "coordinator", Model: "m1", InputTokens: 10, OutputTokens: 5, CostUSD: 0.5})
	log.Append(Event{Kind: KindDispatch, Agent: "coordinator"})
	log.Append(Event{Kind: KindLLMCall, Agent: "code-review", Model: "m2", InputTokens: 7, OutputTokens: 3, CostUSD: 0.25})
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Verify(filepath.Join(root, "cid-1", "audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if s.Events != 3 || s.Total.Calls != 2 || s.Total.InputTokens != 17 || s.Total.CostUSD != 0.75 {
		t.Fatalf("summary = %+v", s)
	}
	if s.ByAgent["code-review"].OutputTokens != 3 || len(s.Models) != 2 {
		t.Fatalf("by agent = %+v, models = %v", s.ByAgent, s.Models)
	}
	if _, err := os.Stat(filepath.Join(root, "cid-1", "summary.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(root, "cid-1"); err == nil {
		t.Fatal("Create overwrote an existing trail")
	}
}

func TestVerifyDetectsTampering(t *testing.T) {
	root := t.TempDir()
	log, _ := Create(root, "cid-2")
	log.Append(Event{Kind: KindLLMCall, Agent: "a", CostUSD: 0.10})
	log.Append(Event{Kind: KindLLMCall, Agent: "a", CostUSD: 0.20})
	log.Close()
	path := filepath.Join(root, "cid-2", "audit.jsonl")
	data, _ := os.ReadFile(path)

	edited := strings.Replace(string(data), `"cost_usd":0.1,`, `"cost_usd":0.01,`, 1)
	os.WriteFile(path, []byte(edited), 0o644)
	if _, err := Verify(path); err == nil || !strings.Contains(err.Error(), "event 1: hash mismatch") {
		t.Fatalf("edited cost: err = %v", err)
	}

	lines := strings.SplitAfter(string(data), "\n")
	os.WriteFile(path, []byte(lines[1]), 0o644)
	if _, err := Verify(path); err == nil {
		t.Fatal("deleted first event went undetected")
	}
}
