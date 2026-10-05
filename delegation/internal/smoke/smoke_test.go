package smoke

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/edlng/agents/litmus-eval/delegation/internal/provider"
)

func TestSmokeOverReplay(t *testing.T) {
	replay := provider.NewReplay([]provider.Exchange{
		{Response: provider.Response{ID: "r1", Model: "claude-haiku-4-5", StopReason: "tool_use", Usage: provider.Usage{InputTokens: 10, CostUSD: 0.01},
			Message: provider.Message{Role: "assistant", Content: []provider.Block{{Type: "tool_use", ID: "t1", Name: "add", Input: json.RawMessage(`{"a":2,"b":3}`)}}}}},
		{Response: provider.Response{ID: "r2", Model: "claude-haiku-4-5", StopReason: "end_turn", Usage: provider.Usage{InputTokens: 20, CostUSD: 0.02},
			Message: provider.Message{Role: "assistant", Content: []provider.Block{{Type: "text", Text: "5"}}}}},
	})
	s, err := Run(context.Background(), replay, "claude-haiku-4-5", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if s.Total.Calls != 2 || s.Total.InputTokens != 30 || s.Total.CostUSD != 0.03 {
		t.Fatalf("summary = %+v", s)
	}
}

func TestSmokeFailsWithoutToolCall(t *testing.T) {
	replay := provider.NewReplay([]provider.Exchange{
		{Response: provider.Response{ID: "r1", Model: "claude-haiku-4-5", StopReason: "end_turn",
			Message: provider.Message{Role: "assistant", Content: []provider.Block{{Type: "text", Text: "5"}}}}},
	})
	if _, err := Run(context.Background(), replay, "claude-haiku-4-5", t.TempDir()); err == nil {
		t.Fatal("smoke passed without a tool call")
	}
}
