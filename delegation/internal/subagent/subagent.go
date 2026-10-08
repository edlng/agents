// Package subagent runs one sub-agent launch. Every launch starts from a new
// message list holding only its own input, so no launch can see another
// agent's conversation. The launch ends when the agent calls its submit tool;
// the harness validates the submission before anything reaches the
// coordinator, and an invalid one comes back as a structured error.
package subagent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/edlng/agents/litmus-eval/delegation/internal/audit"
	"github.com/edlng/agents/litmus-eval/delegation/internal/provider"
	"github.com/edlng/agents/litmus-eval/delegation/internal/schema"
	"github.com/edlng/agents/litmus-eval/delegation/internal/tools"
)

// Spec is the fixed definition of one workflow step, loaded from a manifest.
type Spec struct {
	Workflow  string
	Step      string
	Agent     string
	Model     string
	Effort    string
	System    string
	Tools     []string
	Output    string // artifact kind, for example review.v1
	MaxTurns  int
	MaxTokens int
}

type Launch struct {
	Spec       Spec
	ArtifactID string
	Input      string // task and context, rendered by the caller
	Env        tools.Env
	Validation schema.Context // the runner fills Repo, TestRuns, DocsWritten, and DocsOut
	ParentSpan string
}

type Error struct {
	Code    string   `json:"code"` // config_error | provider_error | refusal | truncated | no_submission | max_turns | schema_violation
	Message string   `json:"message"`
	Details []string `json:"details,omitempty"`
}

type Result struct {
	ArtifactID string            `json:"artifact_id"`
	Workflow   string            `json:"workflow"`
	Step       string            `json:"step"`
	OK         bool              `json:"ok"`
	Verdict    string            `json:"verdict,omitempty"`
	Artifact   json.RawMessage   `json:"artifact,omitempty"`
	Error      *Error            `json:"error,omitempty"`
	Files      map[string]string `json:"files,omitempty"` // docs the agent wrote, by path
	Turns      int               `json:"turns"`
	CostUSD    float64           `json:"cost_usd"`
}

func Run(ctx context.Context, p provider.Provider, log *audit.Log, l Launch) Result {
	res := Result{ArtifactID: l.ArtifactID, Workflow: l.Spec.Workflow, Step: l.Spec.Step}
	var span audit.Event
	fail := func(code, msg string, details ...string) Result {
		res.Error = &Error{Code: code, Message: msg, Details: details}
		// Record why the launch failed, so the trail explains every error
		// the coordinator reasoned over.
		log.Append(audit.Event{
			Kind: audit.KindValidation, ParentSpanID: span.SpanID,
			Agent: l.Spec.Agent, Workflow: l.Spec.Workflow, Step: l.Spec.Step,
			Detail: mustJSON(map[string]any{"artifact_id": l.ArtifactID, "valid": false, "error": res.Error}),
		})
		return res
	}
	kind, err := schema.Lookup(l.Spec.Output)
	if err != nil {
		return fail("config_error", err.Error())
	}
	set, err := tools.Build(l.Spec.Tools, l.Env)
	if err != nil {
		return fail("config_error", err.Error())
	}
	defs := append(set.Defs(), kind.Tool)

	messages := []provider.Message{provider.UserText(l.Input)}
	span, err = log.Append(audit.Event{
		Kind: audit.KindDispatch, ParentSpanID: l.ParentSpan,
		Agent: l.Spec.Agent, Workflow: l.Spec.Workflow, Step: l.Spec.Step,
		Detail: mustJSON(map[string]any{
			"artifact_id":      l.ArtifactID,
			"isolated_context": true,
			"initial_messages": len(messages),
			"tools":            toolNames(defs),
		}),
	})
	if err != nil {
		return fail("config_error", "audit: "+err.Error())
	}

	for turn := 1; turn <= l.Spec.MaxTurns; turn++ {
		res.Turns = turn
		resp, err := p.Complete(ctx, provider.Request{
			Model: l.Spec.Model, System: l.Spec.System, Messages: messages,
			Tools: defs, MaxTokens: l.Spec.MaxTokens, Effort: l.Spec.Effort,
		})
		if err != nil {
			return fail("provider_error", err.Error())
		}
		res.CostUSD += resp.Usage.CostUSD
		if _, err := log.Append(audit.LLMEvent(l.Spec.Agent, l.Spec.Workflow, l.Spec.Step, span.SpanID, resp)); err != nil {
			return fail("provider_error", "audit: "+err.Error())
		}
		switch resp.StopReason {
		case "refusal":
			return fail("refusal", "the model declined the request")
		case "max_tokens":
			return fail("truncated", "the response hit max_tokens")
		}
		messages = append(messages, resp.Message)
		calls := resp.ToolCalls()
		if len(calls) == 0 {
			return fail("no_submission", "the agent stopped without calling "+kind.Tool.Name, truncate(resp.Text(), 500))
		}

		var results []provider.Block
		for _, call := range calls {
			if call.Name == kind.Tool.Name {
				return submit(log, span.SpanID, l, kind, set, call.Input, res)
			}
			out, callErr := set.Call(ctx, call.Name, call.Input)
			detail := map[string]any{"tool": call.Name, "input": truncate(string(call.Input), 300), "output_bytes": len(out)}
			if callErr != nil {
				out = callErr.Error()
				detail["error"] = out
			}
			if _, err := log.Append(audit.Event{
				Kind: audit.KindToolCall, ParentSpanID: span.SpanID,
				Agent: l.Spec.Agent, Workflow: l.Spec.Workflow, Step: l.Spec.Step, Detail: mustJSON(detail),
			}); err != nil {
				return fail("provider_error", "audit: "+err.Error())
			}
			results = append(results, provider.Block{Type: "tool_result", ToolUseID: call.ID, Content: out, IsError: callErr != nil})
		}
		messages = append(messages, provider.ToolResults(results...))
	}
	return fail("max_turns", fmt.Sprintf("no submission after %d turns", l.Spec.MaxTurns))
}

func submit(log *audit.Log, parent string, l Launch, kind schema.Kind, set *tools.Set, raw json.RawMessage, res Result) Result {
	vctx := l.Validation
	vctx.Repo = l.Env.Repo
	vctx.TestRuns = set.TestRuns()
	vctx.DocsWritten = set.Written()
	vctx.DocsOut = l.Env.DocsOut
	raw, err := kind.Normalize(raw)
	if err != nil {
		res.Error = &Error{Code: "provider_error", Message: "normalize submission: " + err.Error()}
		return res
	}
	problems := kind.Validate(raw, vctx)

	// Nothing is overwritten: a relaunch moves the previous valid artifact to
	// <id>.v<n>.json, and repeated rejections get distinct names.
	dir := filepath.Join(log.Dir(), "artifacts")
	name := l.ArtifactID + ".json"
	if len(problems) > 0 {
		name = unused(dir, fmt.Sprintf("%s.rejected-%d", l.ArtifactID, res.Turns), ".json")
	}
	path := filepath.Join(dir, filepath.FromSlash(name))
	detail := map[string]any{"artifact_id": l.ArtifactID, "kind": kind.Name, "valid": len(problems) == 0, "problems": problems, "file": "artifacts/" + name}
	writeErr := os.MkdirAll(filepath.Dir(path), 0o755)
	if _, err := os.Stat(path); writeErr == nil && err == nil {
		old := unusedVersion(dir, l.ArtifactID)
		writeErr = os.Rename(path, filepath.Join(dir, filepath.FromSlash(old)))
		detail["superseded"] = "artifacts/" + old
	}
	if writeErr == nil {
		writeErr = os.WriteFile(path, raw, 0o644)
	}
	if _, err := log.Append(audit.Event{
		Kind: audit.KindValidation, ParentSpanID: parent,
		Agent: l.Spec.Agent, Workflow: l.Spec.Workflow, Step: l.Spec.Step, Detail: mustJSON(detail),
	}); err != nil && writeErr == nil {
		writeErr = err
	}
	if writeErr != nil {
		res.Error = &Error{Code: "provider_error", Message: "persist artifact: " + writeErr.Error()}
		return res
	}
	if len(problems) > 0 {
		res.Error = &Error{Code: "schema_violation", Message: kind.Name + " submission failed validation", Details: problems}
		return res
	}
	for _, path := range vctx.DocsWritten {
		data, err := os.ReadFile(filepath.Join(l.Env.DocsOut, filepath.FromSlash(path)))
		if err != nil {
			res.Error = &Error{Code: "provider_error", Message: "read written doc: " + err.Error()}
			return res
		}
		if res.Files == nil {
			res.Files = map[string]string{}
		}
		res.Files[path] = string(data)
	}
	res.OK = true
	res.Artifact = raw
	res.Verdict = kind.Verdict(raw)
	return res
}

func toolNames(defs []provider.Tool) []string {
	var names []string
	for _, d := range defs {
		names = append(names, d.Name)
	}
	return names
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

// unused returns base+ext, or base-2+ext, base-3+ext, ... for the first name
// not yet in dir.
func unused(dir, base, ext string) string {
	name := base + ext
	for n := 2; exists(filepath.Join(dir, filepath.FromSlash(name))); n++ {
		name = fmt.Sprintf("%s-%d%s", base, n, ext)
	}
	return name
}

// unusedVersion returns the first free <id>.v<n>.json name, starting at v1.
func unusedVersion(dir, id string) string {
	for n := 1; ; n++ {
		if name := fmt.Sprintf("%s.v%d.json", id, n); !exists(filepath.Join(dir, filepath.FromSlash(name))) {
			return name
		}
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
