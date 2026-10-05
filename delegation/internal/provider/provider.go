// Package provider defines the model-call contract shared by the coordinator
// and sub-agents. Runtime code depends only on the Provider interface; the
// concrete backends are the Anthropic API, a replay of recorded responses, and
// (under delegation/evals only) a Claude CLI adapter for development runs.
package provider

import (
	"context"
	"encoding/json"
	"fmt"
)

// Block is one content block in a message.
type Block struct {
	Type      string          `json:"type"` // text | tool_use | tool_result
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

// Message is one conversation turn. Native carries a provider's own encoding
// of an assistant turn (for example thinking blocks with signatures) so the
// turn can be sent back unchanged. Content is always populated for the harness.
type Message struct {
	Role    string          `json:"role"`
	Content []Block         `json:"content"`
	Native  json.RawMessage `json:"native,omitempty"`
}

// Tool is a client-side tool definition. InputSchema is a JSON Schema object.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type Request struct {
	Model     string    `json:"model"`
	System    string    `json:"system"`
	Messages  []Message `json:"messages"`
	Tools     []Tool    `json:"tools,omitempty"`
	MaxTokens int       `json:"max_tokens"`
	Effort    string    `json:"effort,omitempty"`
}

type Usage struct {
	InputTokens      int64   `json:"input_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
	CacheReadTokens  int64   `json:"cache_read_tokens"`
	CacheWriteTokens int64   `json:"cache_write_tokens"`
	CostUSD          float64 `json:"cost_usd"`
}

type Response struct {
	ID         string  `json:"id"`
	Model      string  `json:"model"`
	StopReason string  `json:"stop_reason"` // end_turn | tool_use | max_tokens | refusal
	Message    Message `json:"message"`     // assistant turn to append to history
	Usage      Usage   `json:"usage"`
	DurationMS int64   `json:"duration_ms"`
}

// ToolCalls returns the tool_use blocks of the response in order.
func (r Response) ToolCalls() []Block {
	var calls []Block
	for _, b := range r.Message.Content {
		if b.Type == "tool_use" {
			calls = append(calls, b)
		}
	}
	return calls
}

// Text returns the concatenated text blocks of the response.
func (r Response) Text() string {
	var s string
	for _, b := range r.Message.Content {
		if b.Type == "text" {
			s += b.Text
		}
	}
	return s
}

type Provider interface {
	Complete(ctx context.Context, req Request) (Response, error)
}

// UserText builds a user message with one text block.
func UserText(text string) Message {
	return Message{Role: "user", Content: []Block{{Type: "text", Text: text}}}
}

// ToolResults builds the user message that answers every tool call of a turn.
func ToolResults(results ...Block) Message {
	return Message{Role: "user", Content: results}
}

// price is USD per million tokens.
type price struct{ in, out, cacheRead, cacheWrite float64 }

// Anthropic first-party list prices (claude-api skill, cached 2026-09-25).
// Cache writes use the 5-minute TTL rate of 1.25x input.
var prices = map[string]price{
	"claude-opus-5-5":   {in: 4, out: 20, cacheRead: 0.20, cacheWrite: 5},
	"claude-sonnet-5-5": {in: 2, out: 10, cacheRead: 0.20, cacheWrite: 2.5},
	"claude-haiku-4-5":  {in: 1, out: 5, cacheRead: 0.10, cacheWrite: 1.25},
}

// Cost prices a usage record. Unknown models fail rather than report $0.
func Cost(model string, u Usage) (float64, error) {
	p, ok := prices[model]
	if !ok {
		return 0, fmt.Errorf("no price for model %q", model)
	}
	return (float64(u.InputTokens)*p.in +
		float64(u.OutputTokens)*p.out +
		float64(u.CacheReadTokens)*p.cacheRead +
		float64(u.CacheWriteTokens)*p.cacheWrite) / 1e6, nil
}
