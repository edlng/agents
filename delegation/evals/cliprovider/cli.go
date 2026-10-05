// Package cliprovider adapts `claude -p` to the provider interface for
// development and evaluation runs. It lives under evals/ because the rubric
// exempts CLI use there; the shipped delegate binary must never import it
// (enforced by delegation/cmd/delegate/imports_test.go).
//
// The CLI runs with every built-in tool disabled, so it acts as a plain model.
// Tool calls come back as JSON text under a --json-schema envelope and the
// harness executes them exactly as it does for the API provider.
package cliprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/edlng/agents/litmus-eval/delegation/internal/provider"
)

type CLI struct {
	// Binary defaults to "claude".
	Binary string
	// MaxBudgetUSD caps a single call. Zero means no cap.
	MaxBudgetUSD float64
}

// cliModels maps API model IDs to the IDs the CLI accepts on this machine
// (Bedrock). The CLI silently falls back to its default model for an ID it
// does not recognize, so Complete also checks the model it reports.
var cliModels = map[string]string{
	"claude-opus-5-5":   "claude-opus-5-5",
	"claude-sonnet-5-5": "claude-sonnet-5-5",
	"claude-haiku-4-5":  "global.anthropic.claude-haiku-4-5-20251001-v1:0",
}

type envelope struct {
	Text      string `json:"text"`
	ToolCalls []struct {
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	} `json:"tool_calls"`
}

type cliResult struct {
	IsError          bool     `json:"is_error"`
	Result           string   `json:"result"`
	SessionID        string   `json:"session_id"`
	TotalCostUSD     float64  `json:"total_cost_usd"`
	DurationMS       int64    `json:"duration_ms"`
	StructuredOutput envelope `json:"structured_output"`
	ModelUsage       map[string]struct {
		InputTokens              int64 `json:"inputTokens"`
		OutputTokens             int64 `json:"outputTokens"`
		CacheReadInputTokens     int64 `json:"cacheReadInputTokens"`
		CacheCreationInputTokens int64 `json:"cacheCreationInputTokens"`
	} `json:"modelUsage"`
}

const protocol = "\n\n## Response protocol\n" +
	"You are running inside a harness. Your only native tool is StructuredOutput, and every reply is one\n" +
	"StructuredOutput call: \"text\" holds any prose, and \"tool_calls\" lists the harness tools to run now,\n" +
	"each with \"name\" and \"input\" matching the tool's input_schema. Never call a harness tool natively;\n" +
	"that fails with \"No such tool available\". Harness tool results arrive in the next turn of the\n" +
	"transcript. If a harness tool is named submit_*, you deliver your result only by listing it in\n" +
	"tool_calls; text alone is discarded. An empty tool_calls list ends your work."

func (c CLI) Complete(ctx context.Context, req provider.Request) (provider.Response, error) {
	model, ok := cliModels[req.Model]
	if !ok {
		return provider.Response{}, fmt.Errorf("claude cli: no CLI model ID for %q", req.Model)
	}
	schema, err := envelopeSchema(req.Tools)
	if err != nil {
		return provider.Response{}, err
	}
	system := req.System
	if len(req.Tools) > 0 {
		tools, err := json.MarshalIndent(req.Tools, "", "  ")
		if err != nil {
			return provider.Response{}, err
		}
		system += protocol + "\n\n## Harness tools (request them through tool_calls)\n" + string(tools)
	}
	binary := c.Binary
	if binary == "" {
		binary = "claude"
	}
	args := []string{
		"-p", "--safe-mode", "--tools", "", "--strict-mcp-config", "--no-session-persistence",
		"--model", model, "--system-prompt", system,
		"--output-format", "json", "--json-schema", schema,
	}
	if req.Effort != "" {
		args = append(args, "--effort", req.Effort)
	}
	if c.MaxBudgetUSD > 0 {
		args = append(args, "--max-budget-usd", fmt.Sprintf("%.4f", c.MaxBudgetUSD))
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdin = strings.NewReader(renderTranscript(req.Messages))
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	start := time.Now()
	runErr := cmd.Run()
	elapsed := time.Since(start).Milliseconds()

	var res cliResult
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		return provider.Response{}, fmt.Errorf("claude cli: %v; decode output: %w; stderr: %s", runErr, err, tail(stderr.String()))
	}
	if res.IsError || runErr != nil {
		return provider.Response{}, fmt.Errorf("claude cli: %v: %s", runErr, res.Result)
	}
	if len(res.ModelUsage) != 1 {
		return provider.Response{}, fmt.Errorf("claude cli: expected usage for one model, got %d", len(res.ModelUsage))
	}
	if _, ok := res.ModelUsage[model]; !ok {
		for got := range res.ModelUsage {
			return provider.Response{}, fmt.Errorf("claude cli: requested %s, CLI ran %s", model, got)
		}
	}
	return toResponse(res, elapsed), nil
}

func toResponse(res cliResult, elapsed int64) provider.Response {
	out := provider.Response{ID: res.SessionID, StopReason: "end_turn", DurationMS: elapsed}
	if res.DurationMS > 0 {
		out.DurationMS = res.DurationMS
	}
	for model, u := range res.ModelUsage {
		out.Model = model
		out.Usage.InputTokens += u.InputTokens
		out.Usage.OutputTokens += u.OutputTokens
		out.Usage.CacheReadTokens += u.CacheReadInputTokens
		out.Usage.CacheWriteTokens += u.CacheCreationInputTokens
	}
	// The CLI prices the call itself (Bedrock list price on this machine).
	out.Usage.CostUSD = res.TotalCostUSD
	msg := provider.Message{Role: "assistant"}
	if res.StructuredOutput.Text != "" {
		msg.Content = append(msg.Content, provider.Block{Type: "text", Text: res.StructuredOutput.Text})
	}
	for i, call := range res.StructuredOutput.ToolCalls {
		msg.Content = append(msg.Content, provider.Block{
			Type: "tool_use", ID: fmt.Sprintf("%s_%d", res.SessionID, i), Name: call.Name, Input: call.Input,
		})
	}
	if len(res.StructuredOutput.ToolCalls) > 0 {
		out.StopReason = "tool_use"
	}
	out.Message = msg
	return out
}

func envelopeSchema(tools []provider.Tool) (string, error) {
	call := map[string]any{
		"type":       "object",
		"properties": map[string]any{"name": map[string]any{"type": "string"}, "input": map[string]any{"type": "object"}},
		"required":   []string{"name", "input"},
	}
	if len(tools) > 0 {
		var names []string
		for _, t := range tools {
			names = append(names, t.Name)
		}
		call["properties"].(map[string]any)["name"] = map[string]any{"type": "string", "enum": names}
	}
	b, err := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text":       map[string]any{"type": "string"},
			"tool_calls": map[string]any{"type": "array", "items": call},
		},
		"required": []string{"text", "tool_calls"},
	})
	return string(b), err
}

// renderTranscript flattens the conversation into one prompt. The CLI call is
// stateless, so every turn resends the full history, as the API does.
func renderTranscript(messages []provider.Message) string {
	var b strings.Builder
	for _, m := range messages {
		for _, block := range m.Content {
			switch block.Type {
			case "text":
				fmt.Fprintf(&b, "<%s>\n%s\n</%s>\n", m.Role, block.Text, m.Role)
			case "tool_use":
				fmt.Fprintf(&b, "<tool_call id=%q name=%q>\n%s\n</tool_call>\n", block.ID, block.Name, block.Input)
			case "tool_result":
				fmt.Fprintf(&b, "<tool_result id=%q is_error=%t>\n%s\n</tool_result>\n", block.ToolUseID, block.IsError, block.Content)
			}
		}
	}
	return b.String()
}

func tail(s string) string {
	if len(s) > 500 {
		return s[len(s)-500:]
	}
	return s
}
