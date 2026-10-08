package harness

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/edlng/agents/litmus-eval/delegation/internal/audit"
	"github.com/edlng/agents/litmus-eval/delegation/internal/gate"
	"github.com/edlng/agents/litmus-eval/delegation/internal/provider"
)

// fake plays the coordinator from a script and answers each sub-agent with
// the next payload queued for its submit tool. It records what each sub-agent
// was given.
type fake struct {
	mu          sync.Mutex
	coordinator []string            // tool calls as "name {json}"; "" ends the run
	payloads    map[string][]string // submit tool -> queued inputs
	turn        int
	inputs      map[string][]string // submit tool -> first user message
}

func (f *fake) Complete(_ context.Context, req provider.Request) (provider.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	resp := provider.Response{ID: fmt.Sprintf("r%d", f.turn), Model: req.Model, Usage: provider.Usage{InputTokens: 100, OutputTokens: 10, CostUSD: 0.001}}
	if strings.HasPrefix(req.System, "# Coordinator") {
		if f.turn >= len(f.coordinator) || f.coordinator[f.turn] == "" {
			resp.StopReason = "end_turn"
			resp.Message = provider.Message{Role: "assistant", Content: []provider.Block{{Type: "text", Text: "done"}}}
			return resp, nil
		}
		name, input, _ := strings.Cut(f.coordinator[f.turn], " ")
		f.turn++
		resp.StopReason = "tool_use"
		resp.Message = provider.Message{Role: "assistant", Content: []provider.Block{{Type: "tool_use", ID: fmt.Sprintf("c%d", f.turn), Name: name, Input: json.RawMessage(input)}}}
		return resp, nil
	}
	submit := req.Tools[len(req.Tools)-1].Name
	queue := f.payloads[submit]
	if len(queue) == 0 {
		return provider.Response{}, fmt.Errorf("no payload queued for %s", submit)
	}
	f.payloads[submit] = queue[1:]
	if f.inputs == nil {
		f.inputs = map[string][]string{}
	}
	f.inputs[submit] = append(f.inputs[submit], req.Messages[0].Content[0].Text)
	resp.StopReason = "tool_use"
	resp.Message = provider.Message{Role: "assistant", Content: []provider.Block{{Type: "tool_use", ID: "s", Name: submit, Input: json.RawMessage(queue[0])}}}
	return resp, nil
}

const (
	approveWrong = `{"verdict":"APPROVE","summary":"Looks fine.","findings":[]}`
	blockRight   = `{"verdict":"BLOCK","summary":"Suffix match.","findings":[{"id":"F1","severity":"critical","file":"endpoint/endpoint.go","line":35,"title":"CWE-918 suffix match","evidence":"HasSuffix admits evilapi.example.com","remediation":"compare for equality"}]}`
	challenge    = `{"reviews":[{"artifact_id":"code-review/security","verdict":"CHALLENGED","challenges":[{"id":"C1","severity":"critical","claim":"no security defects","counter_evidence":"HasSuffix admits evilapi.example.com","file":"endpoint/endpoint.go","line":35}]}]}`
	upheld       = `{"reviews":[{"artifact_id":"code-review/security","verdict":"UPHELD","challenges":[]}]}`
	final        = `{"summary":"The change is blocked by one SSRF defect.","sections":[{"artifact_id":"code-review/security","prose":"F1 blocks the merge. The reviewer's C1 led to the corrected review."}]}`
	finalNoRound = `{"summary":"The change is blocked by one SSRF defect.","sections":[{"artifact_id":"code-review/security","prose":"F1 blocks the merge, and the reviewer upheld it."}]}`
)

func run(t *testing.T, f *fake, workflows string) (Result, string) {
	t.Helper()
	root := t.TempDir()
	res, err := Run(context.Background(), Config{
		Provider: f, TaskDir: "../../fixtures/endpoint-allowlist", WorkflowsDir: workflows,
		CoordinatorDir: "../../coordinator", RunsRoot: root,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return res, root
}

// The coordinator governs a rework loop: a wrong APPROVE is challenged,
// re-launched with the challenge, re-reviewed, and reported.
func TestReworkLoopEndsPendingHuman(t *testing.T) {
	f := &fake{
		coordinator: []string{
			`launch_code_reviewer {"step":"security"}`,
			`launch_final_review_writer {"dispositions":[{"artifact_id":"code-review/security","decision":"accepted","reason":"x"}]}`,
			`launch_adversarial_reviewer {"artifact_ids":["code-review/security"]}`,
			`launch_code_reviewer {"step":"security","prior_challenges":[{"id":"C1","severity":"critical"}]}`,
			`launch_adversarial_reviewer {"artifact_ids":["code-review/security"]}`,
			`launch_final_review_writer {"dispositions":[{"artifact_id":"code-review/security","decision":"revised","reason":"re-run after C1"}]}`,
			"",
		},
		payloads: map[string][]string{
			"submit_review":       {approveWrong, blockRight},
			"submit_challenges":   {challenge, upheld},
			"submit_final_review": {final},
		},
	}
	res, root := run(t, f, "../../workflows")
	if res.Status != gate.PendingHuman {
		t.Fatalf("status = %s", res.Status)
	}
	if !strings.Contains(f.inputs["submit_review"][1], `"C1"`) {
		t.Fatal("re-launch did not carry the challenge")
	}
	md, _ := os.ReadFile(filepath.Join(res.Dir, "report.md"))
	for _, want := range []string{"## code-review/security: FAIL", "Coordinator disposition: revised (re-run after C1)", "| F1 | critical | `endpoint/endpoint.go:35`"} {
		if !strings.Contains(string(md), want) {
			t.Errorf("report missing %q\n%s", want, md)
		}
	}

	// The audit trail verifies from disk and records the early final-review
	// refusal, both reviews, and the terminal state.
	trail, _ := os.ReadFile(filepath.Join(res.Dir, "audit.jsonl"))
	for _, want := range []string{`"isolated_context":true`, `"kind":"terminal"`, `"status":"PENDING_HUMAN"`,
		`"kind":"tool_result"`, `"code":"review_pending"`} {
		if !strings.Contains(string(trail), want) {
			t.Errorf("audit missing %s", want)
		}
	}
	s, err := audit.Verify(filepath.Join(res.Dir, "audit.jsonl"))
	if err != nil || s.ByAgent["coordinator"].Calls != 7 || s.ByAgent["adversarial-reviewer"].Calls != 2 {
		t.Fatalf("summary = %+v, %v", s.ByAgent, err)
	}

	// Only a recorded human decision moves the run.
	if _, err := gate.Record(root, res.CorrelationID, "", "approve", "", signer); err == nil {
		t.Fatal("decision without a reviewer accepted")
	}
	if _, err := gate.Record(root, res.CorrelationID, "edlng", "continue", "", signer); err == nil {
		t.Fatal("continue accepted on a non-elevated run")
	}
	if _, err := gate.Record(root, res.CorrelationID, "edlng", "approve", "ok", signer); err != nil {
		t.Fatal(err)
	}
	if st, _ := gate.Status(res.Dir); st != gate.Complete {
		t.Fatalf("status after approve = %s", st)
	}
	if _, err := gate.Record(root, res.CorrelationID, "edlng", "reject", "", signer); err == nil {
		t.Fatal("second decision accepted after COMPLETE")
	}
	if _, err := audit.Verify(filepath.Join(res.Dir, "audit.jsonl")); err != nil {
		t.Fatalf("trail after decision: %v", err)
	}
	if _, err := gate.VerifyDecisions(res.Dir, signer); err != nil {
		t.Fatal(err)
	}
	if _, err := gate.Record(root, res.CorrelationID, "edlng", "approve", "", nil); err == nil || !strings.Contains(err.Error(), "must be signed") {
		t.Fatal("unsigned decision accepted")
	}
	// Editing a recorded decision breaks its signature.
	path := filepath.Join(res.Dir, "decisions.jsonl")
	data, _ := os.ReadFile(path)
	os.WriteFile(path, []byte(strings.Replace(string(data), `"reviewer":"edlng"`, `"reviewer":"mallory"`, 1)), 0o644)
	if _, err := gate.VerifyDecisions(res.Dir, signer); err == nil {
		t.Fatal("edited decision verified")
	}
}

// A tampered report cannot be approved.
func TestDecisionBindsReport(t *testing.T) {
	f := &fake{
		coordinator: []string{
			`launch_code_reviewer {"step":"security"}`,
			`launch_adversarial_reviewer {"artifact_ids":["code-review/security"]}`,
			`launch_final_review_writer {"dispositions":[{"artifact_id":"code-review/security","decision":"accepted","reason":"upheld"}]}`,
			"",
		},
		payloads: map[string][]string{"submit_review": {blockRight}, "submit_challenges": {upheld}, "submit_final_review": {finalNoRound}},
	}
	res, root := run(t, f, "../../workflows")
	os.WriteFile(filepath.Join(res.Dir, "report.md"), []byte("# All good, ship it\n"), 0o644)
	if _, err := gate.Record(root, res.CorrelationID, "edlng", "approve", "", signer); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("approve of tampered report: %v", err)
	}
}

// With documentation scored toy, the run is elevated and needs continue
// before approve.
func TestSubstanceGateElevates(t *testing.T) {
	dir := t.TempDir()
	copyDir(t, "../../workflows", dir)
	m := filepath.Join(dir, "documentation", "manifest.json")
	data, _ := os.ReadFile(m)
	os.WriteFile(m, []byte(strings.Replace(string(data), `"substance": "peripheral"`, `"substance": "toy"`, 1)), 0o644)
	f := &fake{
		coordinator: []string{
			`launch_code_reviewer {"step":"security"}`,
			`launch_adversarial_reviewer {"artifact_ids":["code-review/security"]}`,
			`launch_final_review_writer {"dispositions":[{"artifact_id":"code-review/security","decision":"accepted","reason":"upheld"}]}`,
			"",
		},
		payloads: map[string][]string{"submit_review": {blockRight}, "submit_challenges": {upheld}, "submit_final_review": {finalNoRound}},
	}
	res, root := run(t, f, dir)
	if res.Status != gate.Elevated {
		t.Fatalf("status = %s", res.Status)
	}
	md, _ := os.ReadFile(filepath.Join(res.Dir, "report.md"))
	if !strings.Contains(string(md), "documentation is scored toy") {
		t.Errorf("report does not explain the elevation:\n%s", md)
	}
	if rec, err := gate.ReadRun(res.Dir); err != nil || rec.Workflows != dir {
		t.Errorf("run.json workflows = %q, %v; want %q", rec.Workflows, err, dir)
	}
	if _, err := gate.Record(root, res.CorrelationID, "edlng", "approve", "", signer); err == nil {
		t.Fatal("approve accepted while elevated")
	}
	gate.Record(root, res.CorrelationID, "edlng", "continue", "substance accepted", signer)
	if _, err := gate.Record(root, res.CorrelationID, "edlng", "approve", "", signer); err != nil {
		t.Fatal(err)
	}
}

// fakeSigner stands in for gpg: the signature is a hash of the record.
type fakeSigner struct{}

func (fakeSigner) Sign(data []byte) (string, string, error) {
	return fmt.Sprintf("sig:%x", sha256.Sum256(data)), "FAKEFPR", nil
}

func (fakeSigner) Verify(data []byte, sig string) (string, error) {
	if sig != fmt.Sprintf("sig:%x", sha256.Sum256(data)) {
		return "", fmt.Errorf("bad signature")
	}
	return "FAKEFPR", nil
}

var signer = fakeSigner{}

func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		rel, _ := filepath.Rel(src, p)
		if info.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		data, _ := os.ReadFile(p)
		return os.WriteFile(filepath.Join(dst, rel), data, 0o644)
	})
}

// A recorded run replays through the full harness with no live provider and
// reaches the same terminal state and report.
func TestRecordedRunReplays(t *testing.T) {
	f := &fake{
		coordinator: []string{
			`launch_code_reviewer {"step":"security"}`,
			`launch_adversarial_reviewer {"artifact_ids":["code-review/security"]}`,
			`launch_final_review_writer {"dispositions":[{"artifact_id":"code-review/security","decision":"accepted","reason":"upheld"}]}`,
			"",
		},
		payloads: map[string][]string{"submit_review": {blockRight}, "submit_challenges": {upheld}, "submit_final_review": {finalNoRound}},
	}
	path := filepath.Join(t.TempDir(), "exchanges.jsonl")
	rec, err := provider.NewRecorder(f, path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{TaskDir: "../../fixtures/endpoint-allowlist", WorkflowsDir: "../../workflows", CoordinatorDir: "../../coordinator"}
	cfg.Provider, cfg.RunsRoot = rec, t.TempDir()
	live, err := Run(context.Background(), cfg)
	rec.Close()
	if err != nil {
		t.Fatal(err)
	}

	rp, err := provider.LoadReplay(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Provider, cfg.RunsRoot = rp, t.TempDir()
	replayed, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Status != live.Status || rp.Mismatches != 0 {
		t.Fatalf("replay status %s (live %s), %d request mismatches", replayed.Status, live.Status, rp.Mismatches)
	}
	a, _ := os.ReadFile(filepath.Join(live.Dir, "report.md"))
	b, _ := os.ReadFile(filepath.Join(replayed.Dir, "report.md"))
	strip := func(s []byte, id string) string { return strings.ReplaceAll(string(s), id, "ID") }
	if strip(a, live.CorrelationID) != strip(b, replayed.CorrelationID) {
		t.Fatalf("replayed report differs:\n%s\n---\n%s", a, b)
	}
}
