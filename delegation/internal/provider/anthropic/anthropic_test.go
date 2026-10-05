package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/edlng/agents/litmus-eval/delegation/internal/provider"
)

// The second turn must echo the first assistant turn byte-for-byte, including
// the thinking block and its signature, and carry the tool result.
func TestAnthropicToolRoundTrip(t *testing.T) {
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("request body: %v", err)
		}
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "application/json")
		if len(bodies) == 1 {
			io.WriteString(w, `{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-5-5","stop_reason":"tool_use",
				"content":[{"type":"thinking","thinking":"","signature":"sig-abc"},
				{"type":"tool_use","id":"toolu_1","name":"add","input":{"a":2,"b":3}}],
				"usage":{"input_tokens":100,"output_tokens":20,"cache_read_input_tokens":0,"cache_creation_input_tokens":50}}`)
			return
		}
		io.WriteString(w, `{"id":"msg_2","type":"message","role":"assistant","model":"claude-sonnet-5-5","stop_reason":"end_turn",
			"content":[{"type":"text","text":"5"}],
			"usage":{"input_tokens":10,"output_tokens":2,"cache_read_input_tokens":50,"cache_creation_input_tokens":0}}`)
	}))
	defer server.Close()

	p := New(option.WithBaseURL(server.URL), option.WithAPIKey("test"), option.WithMaxRetries(0))
	req := provider.Request{
		Model:     "claude-sonnet-5-5",
		System:    "sys",
		Messages:  []provider.Message{provider.UserText("2+3?")},
		Tools:     []provider.Tool{{Name: "add", Description: "add", InputSchema: json.RawMessage(`{"type":"object","properties":{"a":{"type":"integer"}},"required":["a"]}`)}},
		MaxTokens: 1024,
		Effort:    "low",
	}
	first, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	calls := first.ToolCalls()
	if first.StopReason != "tool_use" || len(calls) != 1 || calls[0].ID != "toolu_1" || string(calls[0].Input) != `{"a":2,"b":3}` {
		t.Fatalf("first response = %+v", first)
	}
	if want := (100*2.0 + 20*10.0 + 50*2.5) / 1e6; first.Usage.CostUSD != want {
		t.Fatalf("cost = %v, want %v", first.Usage.CostUSD, want)
	}

	req.Messages = append(req.Messages, first.Message, provider.ToolResults(provider.Block{Type: "tool_result", ToolUseID: "toolu_1", Content: "5"}))
	second, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if second.Text() != "5" || second.StopReason != "end_turn" {
		t.Fatalf("second response = %+v", second)
	}

	system := bodies[0]["system"].([]any)[0].(map[string]any)
	if system["cache_control"] == nil {
		t.Error("system block has no cache_control")
	}
	if bodies[0]["output_config"].(map[string]any)["effort"] != "low" {
		t.Error("effort not sent")
	}
	msgs := bodies[1]["messages"].([]any)
	assistant := msgs[1].(map[string]any)["content"].([]any)
	thinking := assistant[0].(map[string]any)
	if thinking["type"] != "thinking" || thinking["signature"] != "sig-abc" {
		t.Errorf("thinking block not echoed: %v", thinking)
	}
	result := msgs[2].(map[string]any)["content"].([]any)[0].(map[string]any)
	if result["type"] != "tool_result" || result["tool_use_id"] != "toolu_1" {
		t.Errorf("tool result = %v", result)
	}
}

func TestAnthropicRejectsUnpricedModelBeforeCalling(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer server.Close()
	p := New(option.WithBaseURL(server.URL), option.WithAPIKey("test"))
	_, err := p.Complete(context.Background(), provider.Request{Model: "claude-unknown", MaxTokens: 10, Messages: []provider.Message{provider.UserText("hi")}})
	if err == nil || !strings.Contains(err.Error(), "no price") || called {
		t.Fatalf("err = %v, called = %v", err, called)
	}
}
