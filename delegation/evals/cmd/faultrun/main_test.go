package main

import (
	"context"
	"testing"

	"github.com/edlng/agents/litmus-eval/delegation/internal/provider"
)

type counter struct{ calls int }

func (c *counter) Complete(context.Context, provider.Request) (provider.Response, error) {
	c.calls++
	return provider.Response{ID: "live"}, nil
}

// Only the first security launch is replaced; its re-launch and every other
// agent reach the wrapped provider.
func TestInjectsOnlyFirstSecurityLaunch(t *testing.T) {
	live := &counter{}
	f := &injector{Provider: live}
	security := provider.Request{System: "# Code reviewer\n## Lens: security", Messages: []provider.Message{provider.UserText("task")}}
	other := provider.Request{System: "# Coordinator", Messages: []provider.Message{provider.UserText("task")}}

	r, _ := f.Complete(context.Background(), security)
	if r.ID != "fault-injected-security-1" || len(r.ToolCalls()) != 1 || r.ToolCalls()[0].Name != "submit_review" {
		t.Fatalf("first security launch = %+v", r)
	}
	for _, req := range []provider.Request{other, security} {
		if r, _ := f.Complete(context.Background(), req); r.ID != "live" {
			t.Fatalf("%q went to %s, want the live provider", req.System, r.ID)
		}
	}
	if live.calls != 2 {
		t.Fatalf("live calls = %d, want 2", live.calls)
	}
}
