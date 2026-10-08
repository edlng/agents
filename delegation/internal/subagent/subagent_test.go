package subagent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/edlng/agents/litmus-eval/delegation/internal/audit"
	"github.com/edlng/agents/litmus-eval/delegation/internal/provider"
	"github.com/edlng/agents/litmus-eval/delegation/internal/tools"
)

// scripted returns canned responses and keeps every request it received.
type scripted struct {
	responses []provider.Response
	requests  []provider.Request
}

func (s *scripted) Complete(_ context.Context, req provider.Request) (provider.Response, error) {
	s.requests = append(s.requests, req)
	r := s.responses[len(s.requests)-1]
	if r.Model == "" {
		r.Model = req.Model
	}
	return r, nil
}

func toolUse(id, name, input string) provider.Response {
	return provider.Response{ID: "resp-" + id, StopReason: "tool_use", Usage: provider.Usage{CostUSD: 0.01},
		Message: provider.Message{Role: "assistant", Content: []provider.Block{{Type: "tool_use", ID: id, Name: name, Input: json.RawMessage(input)}}}}
}

func setup(t *testing.T) (Launch, *audit.Log) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "calc.go"), []byte("package calc\n\nfunc Div(a, b int) int {\n\treturn a / b\n}\n"), 0o644)
	log, err := audit.Create(t.TempDir(), "cid")
	if err != nil {
		t.Fatal(err)
	}
	return Launch{
		Spec: Spec{Workflow: "code-review", Step: "security", Agent: "code-reviewer", Model: "claude-sonnet-5-5",
			System: "review", Tools: []string{"read_file"}, Output: "review.v1", MaxTurns: 4, MaxTokens: 1000},
		ArtifactID: "code-review/security",
		Input:      "Review calc.go",
		Env:        tools.Env{Repo: dir},
	}, log
}

const blockReview = `{"verdict":"BLOCK","summary":"s","findings":[{"id":"F1","severity":"major","file":"calc.go","line":4,"title":"t","evidence":"e","remediation":"r"}]}`

func TestValidSubmission(t *testing.T) {
	l, log := setup(t)
	p := &scripted{responses: []provider.Response{
		toolUse("t1", "read_file", `{"path":"calc.go"}`),
		toolUse("t2", "submit_review", blockReview),
	}}
	res := Run(context.Background(), p, log, l)
	if !res.OK || res.Verdict != "FAIL" || res.Turns != 2 || res.CostUSD != 0.02 {
		t.Fatalf("result = %+v", res)
	}
	// Fresh context: the first request holds only the launch input.
	if first := p.requests[0].Messages; len(first) != 1 || first[0].Content[0].Text != "Review calc.go" {
		t.Fatalf("first request messages = %+v", first)
	}
	// The read_file result reached the model on turn 2.
	second := p.requests[1].Messages
	if r := second[len(second)-1].Content[0]; r.Type != "tool_result" || !strings.Contains(r.Content, "return a / b") {
		t.Fatalf("tool result = %+v", r)
	}
	// The agent sees only its declared tools plus its submit tool.
	var names []string
	for _, d := range p.requests[0].Tools {
		names = append(names, d.Name)
	}
	if strings.Join(names, ",") != "read_file,submit_review" {
		t.Fatalf("tools = %v", names)
	}
	if _, err := os.Stat(filepath.Join(log.Dir(), "artifacts", "code-review/security.json")); err != nil {
		t.Fatal(err)
	}
	log.Close()
	s, err := audit.Verify(filepath.Join(log.Dir(), "audit.jsonl"))
	if err != nil || s.ByAgent["code-reviewer"].Calls != 2 || s.Events != 5 { // dispatch, llm, tool, llm, validation
		t.Fatalf("summary = %+v, %v", s, err)
	}
}

func TestInvalidSubmissionIsStructuredError(t *testing.T) {
	l, log := setup(t)
	bad := strings.Replace(blockReview, `"line":4`, `"line":40`, 1)
	res := Run(context.Background(), &scripted{responses: []provider.Response{toolUse("t1", "submit_review", bad)}}, log, l)
	if res.OK || res.Error == nil || res.Error.Code != "schema_violation" || !strings.Contains(strings.Join(res.Error.Details, " "), "line 40") {
		t.Fatalf("result = %+v", res)
	}
	if res.Artifact != nil {
		t.Fatal("invalid artifact returned to caller")
	}
}

func TestStopsWithoutSubmission(t *testing.T) {
	l, log := setup(t)
	text := provider.Response{StopReason: "end_turn", Message: provider.Message{Role: "assistant", Content: []provider.Block{{Type: "text", Text: "looks fine"}}}}
	res := Run(context.Background(), &scripted{responses: []provider.Response{text}}, log, l)
	if res.Error == nil || res.Error.Code != "no_submission" {
		t.Fatalf("result = %+v", res)
	}

	l, log = setup(t)
	l.Spec.MaxTurns = 2
	loop := []provider.Response{toolUse("a", "read_file", `{"path":"calc.go"}`), toolUse("b", "read_file", `{"path":"calc.go"}`)}
	res = Run(context.Background(), &scripted{responses: loop}, log, l)
	if res.Error == nil || res.Error.Code != "max_turns" {
		t.Fatalf("result = %+v", res)
	}
}

// A relaunch keeps the earlier valid artifact, and a repeated rejection gets
// its own file.
func TestRelaunchKeepsEarlierSubmissions(t *testing.T) {
	l, log := setup(t)
	approve := `{"verdict":"APPROVE","summary":"first","findings":[]}`
	bad := strings.Replace(blockReview, `"line":4`, `"line":40`, 1)
	for _, payload := range []string{approve, blockReview, bad, bad} {
		Run(context.Background(), &scripted{responses: []provider.Response{toolUse("t1", "submit_review", payload)}}, log, l)
	}
	dir := filepath.Join(log.Dir(), "artifacts", "code-review")
	for name, want := range map[string]string{
		"security.v1.json": `"APPROVE"`, "security.json": `"BLOCK"`,
		"security.rejected-1.json": `"line":40`, "security.rejected-1-2.json": `"line":40`,
	} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || !strings.Contains(string(data), want) {
			t.Errorf("%s = %q, %v; want it to contain %s", name, data, err, want)
		}
	}
	log.Close()
}
