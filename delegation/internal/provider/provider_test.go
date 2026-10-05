package provider

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestCost(t *testing.T) {
	got, err := Cost("claude-sonnet-5-5", Usage{InputTokens: 1_000_000, OutputTokens: 100_000, CacheReadTokens: 1_000_000, CacheWriteTokens: 1_000_000})
	if err != nil {
		t.Fatal(err)
	}
	if want := 2.0 + 1.0 + 0.20 + 2.5; got != want {
		t.Fatalf("cost = %v, want %v", got, want)
	}
	if _, err := Cost("gpt-unknown", Usage{}); err == nil {
		t.Fatal("unknown model priced without error")
	}
}

type fixed struct{ resp Response }

func (f fixed) Complete(context.Context, Request) (Response, error) { return f.resp, nil }

func TestRecordThenReplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rec.jsonl")
	want := Response{ID: "r1", Model: "claude-haiku-4-5", StopReason: "end_turn", Message: Message{Role: "assistant", Content: []Block{{Type: "text", Text: "ok"}}}}
	rec, err := NewRecorder(fixed{want}, path)
	if err != nil {
		t.Fatal(err)
	}
	req := Request{Model: "claude-haiku-4-5", Messages: []Message{UserText("hi")}, MaxTokens: 10}
	if _, err := rec.Complete(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	rec.Close()

	replay, err := LoadReplay(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := replay.Complete(context.Background(), req)
	if err != nil || got.ID != "r1" || got.Text() != "ok" {
		t.Fatalf("replay = %+v, %v", got, err)
	}
	if _, err := replay.Complete(context.Background(), req); err == nil || !strings.Contains(err.Error(), "exhausted") {
		t.Fatalf("second replay err = %v", err)
	}

	drifted, _ := LoadReplay(path)
	req.System = "changed prompt"
	if _, err := drifted.Complete(context.Background(), req); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("drifted replay err = %v", err)
	}
}
