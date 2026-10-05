package audit

import "github.com/edlng/agents/litmus-eval/delegation/internal/provider"

// LLMEvent builds the audit record for one model call.
func LLMEvent(agent, workflow, step, parent string, resp provider.Response) Event {
	return Event{
		Kind:         KindLLMCall,
		ParentSpanID: parent,
		Agent:        agent,
		Workflow:     workflow,
		Step:         step,
		Model:        resp.Model,
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
		CacheRead:    resp.Usage.CacheReadTokens,
		CacheWrite:   resp.Usage.CacheWriteTokens,
		CostUSD:      resp.Usage.CostUSD,
		DurationMS:   resp.DurationMS,
		ResponseID:   resp.ID,
	}
}
