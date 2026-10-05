// Package anthropic is the Messages API backend for the provider interface.
package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/edlng/agents/litmus-eval/delegation/internal/provider"
)

// Client calls the Messages API through the official Go SDK.
type Client struct {
	client anthropic.Client
}

// New reads ANTHROPIC_API_KEY unless opts override it.
func New(opts ...option.RequestOption) *Client {
	return &Client{client: anthropic.NewClient(opts...)}
}

func (a *Client) Complete(ctx context.Context, req provider.Request) (provider.Response, error) {
	// Check pricing before spending money, so the audit trail never records
	// a paid call it cannot price.
	if _, err := provider.Cost(req.Model, provider.Usage{}); err != nil {
		return provider.Response{}, err
	}
	params, err := toParams(req)
	if err != nil {
		return provider.Response{}, err
	}
	start := time.Now()
	msg, err := a.client.Messages.New(ctx, params)
	if err != nil {
		return provider.Response{}, fmt.Errorf("anthropic: %w", err)
	}
	return fromMessage(msg, time.Since(start).Milliseconds())
}

func toParams(req provider.Request) (anthropic.MessageNewParams, error) {
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(req.Model),
		MaxTokens: int64(req.MaxTokens),
		// The system prompt and tool list are identical on every turn of a
		// loop, so one breakpoint caches both.
		System: []anthropic.TextBlockParam{{
			Text:         req.System,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}},
	}
	if req.Effort != "" {
		params.OutputConfig = anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffort(req.Effort)}
	}
	for _, t := range req.Tools {
		var schema struct {
			Properties any      `json:"properties"`
			Required   []string `json:"required"`
		}
		if err := json.Unmarshal(t.InputSchema, &schema); err != nil {
			return params, fmt.Errorf("tool %s schema: %w", t.Name, err)
		}
		tool := anthropic.ToolParam{
			Name:        t.Name,
			Description: anthropic.String(t.Description),
			InputSchema: anthropic.ToolInputSchemaParam{Properties: schema.Properties, Required: schema.Required},
		}
		params.Tools = append(params.Tools, anthropic.ToolUnionParam{OfTool: &tool})
	}
	for i, m := range req.Messages {
		p, err := toMessageParam(m)
		if err != nil {
			return params, fmt.Errorf("message %d: %w", i, err)
		}
		params.Messages = append(params.Messages, p)
	}
	return params, nil
}

func toMessageParam(m provider.Message) (anthropic.MessageParam, error) {
	if len(m.Native) > 0 {
		var p anthropic.MessageParam
		if err := json.Unmarshal(m.Native, &p); err != nil {
			return p, fmt.Errorf("native turn: %w", err)
		}
		return p, nil
	}
	var blocks []anthropic.ContentBlockParamUnion
	for _, b := range m.Content {
		switch b.Type {
		case "text":
			blocks = append(blocks, anthropic.NewTextBlock(b.Text))
		case "tool_use":
			blocks = append(blocks, anthropic.NewToolUseBlock(b.ID, b.Input, b.Name))
		case "tool_result":
			blocks = append(blocks, anthropic.NewToolResultBlock(b.ToolUseID, b.Content, b.IsError))
		default:
			return anthropic.MessageParam{}, fmt.Errorf("unsupported block type %q", b.Type)
		}
	}
	if m.Role == "assistant" {
		return anthropic.NewAssistantMessage(blocks...), nil
	}
	return anthropic.NewUserMessage(blocks...), nil
}

func fromMessage(msg *anthropic.Message, durationMS int64) (provider.Response, error) {
	native, err := json.Marshal(msg.ToParam())
	if err != nil {
		return provider.Response{}, fmt.Errorf("encode native turn: %w", err)
	}
	out := provider.Response{
		ID:         msg.ID,
		Model:      string(msg.Model),
		StopReason: string(msg.StopReason),
		Message:    provider.Message{Role: "assistant", Native: native},
		Usage: provider.Usage{
			InputTokens:      msg.Usage.InputTokens,
			OutputTokens:     msg.Usage.OutputTokens,
			CacheReadTokens:  msg.Usage.CacheReadInputTokens,
			CacheWriteTokens: msg.Usage.CacheCreationInputTokens,
		},
		DurationMS: durationMS,
	}
	for _, block := range msg.Content {
		switch v := block.AsAny().(type) {
		case anthropic.TextBlock:
			out.Message.Content = append(out.Message.Content, provider.Block{Type: "text", Text: v.Text})
		case anthropic.ToolUseBlock:
			out.Message.Content = append(out.Message.Content, provider.Block{
				Type: "tool_use", ID: v.ID, Name: v.Name, Input: json.RawMessage(v.JSON.Input.Raw()),
			})
		}
	}
	if out.Usage.CostUSD, err = provider.Cost(out.Model, out.Usage); err != nil {
		return out, err
	}
	return out, nil
}
