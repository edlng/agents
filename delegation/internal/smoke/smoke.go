// Package smoke runs a two-call tool round trip against a provider and
// records it in an audit trail. It checks that a provider returns tool calls
// the harness can execute and reports usage the audit trail can price.
package smoke

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/edlng/agents/litmus-eval/delegation/internal/audit"
	"github.com/edlng/agents/litmus-eval/delegation/internal/provider"
)

var addTool = provider.Tool{
	Name:        "add",
	Description: "Add two integers and return the sum.",
	InputSchema: json.RawMessage(`{"type":"object","properties":{"a":{"type":"integer"},"b":{"type":"integer"}},"required":["a","b"]}`),
}

// Run asks the model to add 2 and 3 with the add tool and expects "5" back.
func Run(ctx context.Context, p provider.Provider, model, runsRoot string) (audit.Summary, error) {
	log, err := audit.Create(runsRoot, audit.NewID())
	if err != nil {
		return audit.Summary{}, err
	}
	sum, runErr := run(ctx, p, model, log)
	if err := log.Close(); err != nil && runErr == nil {
		runErr = err
	}
	if runErr != nil {
		return sum, fmt.Errorf("smoke %s: %w", log.CorrelationID(), runErr)
	}
	return audit.Verify(log.Dir() + "/audit.jsonl")
}

func run(ctx context.Context, p provider.Provider, model string, log *audit.Log) (audit.Summary, error) {
	req := provider.Request{
		Model:     model,
		System:    "You are a calculator. Use the add tool for arithmetic, then state the result as a bare number.",
		Messages:  []provider.Message{provider.UserText("What is 2 + 3?")},
		Tools:     []provider.Tool{addTool},
		MaxTokens: 1024,
		Effort:    "low",
	}
	for turn := 1; turn <= 2; turn++ {
		resp, err := p.Complete(ctx, req)
		if err != nil {
			return audit.Summary{}, err
		}
		if _, err := log.Append(audit.LLMEvent("smoke", "smoke", fmt.Sprintf("turn-%d", turn), "", resp)); err != nil {
			return audit.Summary{}, err
		}
		req.Messages = append(req.Messages, resp.Message)
		calls := resp.ToolCalls()
		if turn == 1 {
			if len(calls) != 1 || calls[0].Name != "add" {
				return audit.Summary{}, fmt.Errorf("turn 1: want one add call, got %d calls (stop %s)", len(calls), resp.StopReason)
			}
			var in struct{ A, B int }
			if err := json.Unmarshal(calls[0].Input, &in); err != nil {
				return audit.Summary{}, fmt.Errorf("turn 1: add input: %w", err)
			}
			req.Messages = append(req.Messages, provider.ToolResults(provider.Block{
				Type: "tool_result", ToolUseID: calls[0].ID, Content: fmt.Sprint(in.A + in.B),
			}))
			continue
		}
		if len(calls) != 0 || !strings.Contains(resp.Text(), "5") {
			return audit.Summary{}, fmt.Errorf("turn 2: want final text with 5, got %q and %d calls", resp.Text(), len(calls))
		}
	}
	return audit.Summary{}, nil
}
