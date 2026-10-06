// Package coordinator runs the governing LLM. It holds only dispatch tools,
// defined here, and reaches sub-agents only through the Dispatcher interface.
// Every tool name starts with a dispatch verb (launch_ or invoke_). It cannot
// import the tools, sub-agent, or dispatch packages (enforced by
// imports_test.go), so it has no path to files, commands, or the network.
package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/edlng/agents/litmus-eval/delegation/internal/audit"
	"github.com/edlng/agents/litmus-eval/delegation/internal/provider"
)

// Dispatcher executes a launch tool call and returns its result as JSON.
// Refusals (limits, missing inputs, gates) are results, not errors; an error
// means the harness itself failed and the run must stop.
type Dispatcher interface {
	Dispatch(ctx context.Context, tool string, input json.RawMessage, parentSpan string) (json.RawMessage, error)
}

type ToolBinding struct {
	Name     string `json:"name"`
	Workflow string `json:"workflow"`
}

type Manifest struct {
	Agent        string        `json:"agent"`
	Purpose      string        `json:"purpose"`
	Model        string        `json:"model"`
	Effort       string        `json:"effort"`
	MaxTurns     int           `json:"max_turns"`
	MaxTokens    int           `json:"max_tokens"`
	DispatchOnly bool          `json:"dispatch_only"`
	Tools        []ToolBinding `json:"tools"`
	Evaluation   struct {
		Criteria []string `json:"criteria"`
		Results  string   `json:"results"`
	} `json:"evaluation"`

	System string `json:"-"`
}

// DispatchVerbs are the only tool-name prefixes a coordinator may hold.
var DispatchVerbs = []string{"launch_", "invoke_"}

// LoadManifest reads <dir>/manifest.json and <dir>/prompt.md and checks that
// every declared tool is a dispatch tool with a definition in this package.
func LoadManifest(dir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("coordinator manifest: %w", err)
	}
	prompt, err := os.ReadFile(filepath.Join(dir, "prompt.md"))
	if err != nil {
		return nil, err
	}
	m.System = string(prompt)
	if !m.DispatchOnly {
		return nil, fmt.Errorf("coordinator manifest: dispatch_only must be true")
	}
	for _, t := range m.Tools {
		if !IsDispatchVerb(t.Name) {
			return nil, fmt.Errorf("coordinator manifest: %s is not a dispatch tool", t.Name)
		}
		if _, ok := definitions[t.Name]; !ok {
			return nil, fmt.Errorf("coordinator manifest: no definition for %s", t.Name)
		}
	}
	return &m, nil
}

func IsDispatchVerb(name string) bool {
	for _, v := range DispatchVerbs {
		if strings.HasPrefix(name, v) {
			return true
		}
	}
	return false
}

// Tools returns the provider definitions for the manifest's tools.
func (m *Manifest) ToolDefs() []provider.Tool {
	var defs []provider.Tool
	for _, t := range m.Tools {
		defs = append(defs, definitions[t.Name])
	}
	return defs
}

func (m *Manifest) Workflow(tool string) string {
	for _, t := range m.Tools {
		if t.Name == tool {
			return t.Workflow
		}
	}
	return ""
}

const stepTool = `{"type":"object","properties":{
	"step":{"type":"string","description":"workflow step ID from the catalog"},
	"context_artifacts":{"type":"array","items":{"type":"string"},"description":"artifact IDs from other workflows to show the agent"},
	"prior_challenges":{"type":"array","items":{"type":"object"},"description":"adversarial challenges against this step's artifact, copied from the reviewer result"}},
	"required":["step"]}`

var definitions = map[string]provider.Tool{
	"launch_code_reviewer": {
		Name:        "launch_code_reviewer",
		Description: "Launch the code-review workflow at one step (security or correctness). Returns the validated review.v1 artifact or a structured error.",
		InputSchema: json.RawMessage(stepTool),
	},
	"launch_spec_validator": {
		Name:        "launch_spec_validator",
		Description: "Launch the spec-validation workflow at one step (tests, then criteria). Returns the validated artifact or a structured error.",
		InputSchema: json.RawMessage(stepTool),
	},
	"launch_documenter": {
		Name:        "launch_documenter",
		Description: "Launch the documentation workflow at its write step. Returns the validated documentation.v1 artifact or a structured error.",
		InputSchema: json.RawMessage(stepTool),
	},
	"launch_adversarial_reviewer": {
		Name:        "launch_adversarial_reviewer",
		Description: "Launch the adversarial reviewer in a fresh context over the named artifacts. Returns UPHELD or CHALLENGED per artifact with challenges.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"artifact_ids":{"type":"array","items":{"type":"string"}}},"required":["artifact_ids"]}`),
	},
	"launch_final_review_writer": {
		Name:        "launch_final_review_writer",
		Description: "Launch the final review writer with one disposition per artifact. Refused until every artifact's current version has an adversarial review.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"dispositions":{"type":"array","items":{"type":"object","properties":{
			"artifact_id":{"type":"string"},"decision":{"type":"string","enum":["accepted","revised","unresolved"]},"reason":{"type":"string"}},
			"required":["artifact_id","decision","reason"]}}},"required":["dispositions"]}`),
	},
	"invoke_human_review": {
		Name:        "invoke_human_review",
		Description: "Invoke human review for something a person must decide before the report goes out. Records the request in the audit trail; it does not pause or approve anything.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"reason":{"type":"string"}},"required":["reason"]}`),
	},
}

type Outcome struct {
	Turns      int     `json:"turns"`
	FinalText  string  `json:"final_text"`
	StopReason string  `json:"stop_reason"` // done | max_turns | refusal | truncated
	CostUSD    float64 `json:"cost_usd"`
}

// Run drives the coordinator until it stops calling tools or runs out of
// turns. brief is the task and workflow catalog; it holds no code.
func Run(ctx context.Context, p provider.Provider, log *audit.Log, m *Manifest, d Dispatcher, brief string) (Outcome, error) {
	var out Outcome
	defs := m.ToolDefs()
	messages := []provider.Message{provider.UserText(brief)}
	span, err := log.Append(audit.Event{Kind: audit.KindDispatch, Agent: m.Agent,
		Detail: mustJSON(map[string]any{"role": "coordinator", "tools": names(defs), "dispatch_only": true})})
	if err != nil {
		return out, err
	}
	for turn := 1; turn <= m.MaxTurns; turn++ {
		out.Turns = turn
		resp, err := p.Complete(ctx, provider.Request{
			Model: m.Model, System: m.System, Messages: messages, Tools: defs,
			MaxTokens: m.MaxTokens, Effort: m.Effort,
		})
		if err != nil {
			return out, fmt.Errorf("coordinator turn %d: %w", turn, err)
		}
		out.CostUSD += resp.Usage.CostUSD
		if _, err := log.Append(audit.LLMEvent(m.Agent, "", "", span.SpanID, resp)); err != nil {
			return out, err
		}
		switch resp.StopReason {
		case "refusal", "max_tokens":
			out.StopReason = map[string]string{"refusal": "refusal", "max_tokens": "truncated"}[resp.StopReason]
			return out, nil
		}
		messages = append(messages, resp.Message)
		calls := resp.ToolCalls()
		if len(calls) == 0 {
			out.FinalText, out.StopReason = resp.Text(), "done"
			return out, nil
		}
		var results []provider.Block
		for _, call := range calls {
			var result json.RawMessage
			if m.Workflow(call.Name) == "" {
				result = mustJSON(map[string]any{"ok": false, "error": map[string]string{"code": "unknown_tool", "message": call.Name + " is not a coordinator tool"}})
			} else if result, err = d.Dispatch(ctx, call.Name, call.Input, span.SpanID); err != nil {
				return out, fmt.Errorf("dispatch %s: %w", call.Name, err)
			}
			results = append(results, provider.Block{Type: "tool_result", ToolUseID: call.ID, Content: string(result)})
		}
		messages = append(messages, provider.ToolResults(results...))
	}
	out.StopReason = "max_turns"
	return out, nil
}

func names(defs []provider.Tool) []string {
	var n []string
	for _, d := range defs {
		n = append(n, d.Name)
	}
	return n
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
