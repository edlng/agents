package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/edlng/agents/litmus-eval/delegation/evals/corpus"
	"github.com/edlng/agents/litmus-eval/delegation/internal/provider"
	"github.com/edlng/agents/litmus-eval/delegation/internal/schema"
)

const corpusDir = "delegation/corpus"

// corpusCases scores the code-review workflow against real pull-request
// review rounds. Both lenses run, as in a full review; the change counts as
// blocked when either lens blocks.
func corpusCases() []Case {
	cases, err := corpus.Load(corpusDir)
	must(err)
	var out []Case
	for _, cc := range cases {
		cc := cc
		var crit []string
		if len(cc.Expected.Disputed) == 0 {
			crit = append(crit, "matches-human-verdict")
		}
		if cc.Expected.Verdict == "BLOCK" {
			crit = append(crit, "recalls-human-flagged-defects")
		}
		out = append(out, Case{
			ID: cc.ID, Workflow: "code-review", Fixture: cc.ID, Criteria: crit, Split: cc.Split,
			Purpose: fmt.Sprintf("%s review round at %s; humans: %s with %d labeled defects.", cc.PR, cc.HeadSHA[:12], cc.Expected.Verdict, len(cc.Expected.Defects)),
			Prepare: func() (string, func(), error) {
				dir, cleanup, err := corpus.Materialize(cc, corpus.CacheRoot())
				if err != nil {
					return "", cleanup, err
				}
				abs, err := filepath.Abs(dir)
				return abs, cleanup, err
			},
			Run: func(ctx context.Context, e *Env, t *Trial) {
				var findings []schema.Finding
				blocked := false
				for _, step := range []string{"security", "correctness"} {
					res := e.Runner.LaunchStep(ctx, "code-review", step, nil, nil, "")
					r, ok := review(res)
					if !ok {
						t.note("%s: %s", step, errorText(res))
						continue
					}
					blocked = blocked || r.Verdict == "BLOCK"
					findings = append(findings, r.Findings...)
					t.note("%s: %s with %d findings", step, r.Verdict, len(r.Findings))
				}
				if len(cc.Expected.Disputed) == 0 {
					t.Pass["matches-human-verdict"] = blocked == (cc.Expected.Verdict == "BLOCK")
				} else {
					t.note("verdict %s not scored: %d disputed finding(s)", map[bool]string{true: "BLOCK", false: "APPROVE"}[blocked], len(cc.Expected.Disputed))
				}
				if cc.Expected.Verdict != "BLOCK" {
					return
				}
				found := 0
				for _, d := range cc.Expected.Defects {
					how, cost := matchDefect(ctx, e.Provider, d, findings)
					t.CostUSD += cost
					if how != "" {
						found++
					}
					t.note("defect %s:%d %s", d.File, d.Line, map[bool]string{true: "found (" + how + ")", false: "missed"}[how != ""])
				}
				t.Counts = map[string][2]int{"recalls-human-flagged-defects": {found, len(cc.Expected.Defects)}}
				t.Pass["recalls-human-flagged-defects"] = found == len(cc.Expected.Defects)
			},
		})
	}
	return out
}

// matchDefect returns "location" when a blocking finding is in the labeled
// file within 10 lines, "semantic" when a judge says a finding describes the
// same defect, or "" when no finding covers it.
func matchDefect(ctx context.Context, p provider.Provider, d corpus.Defect, findings []schema.Finding) (string, float64) {
	var listed []string
	for i, f := range findings {
		if f.Severity == "minor" {
			continue
		}
		if f.File == d.File && near(f.Line, d.Line, 10) {
			return "location", 0
		}
		listed = append(listed, fmt.Sprintf("%d. %s:%d %s: %s", i, f.File, f.Line, f.Title, f.Evidence))
	}
	if len(listed) == 0 {
		return "", 0
	}
	resp, err := p.Complete(ctx, provider.Request{
		Model:     "claude-haiku-4-5",
		MaxTokens: 800,
		System: `You decide whether an automated code review found a defect that a human reviewer flagged.
A finding matches only if it describes the same underlying defect (same mechanism and consequence), even if
it cites a different line or words it differently. A finding about a different problem in the same code does
not match. Call submit_match once with the matching finding number, or -1.`,
		Messages: []provider.Message{provider.UserText(fmt.Sprintf("Human-flagged defect in %s near line %d:\n%s\n\nAutomated findings:\n%s",
			d.File, d.Line, d.Summary, strings.Join(listed, "\n")))},
		Tools: []provider.Tool{{Name: "submit_match", Description: "Submit the matching finding number or -1.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"finding":{"type":"integer"},"rationale":{"type":"string"}},"required":["finding","rationale"]}`)}},
	})
	if err != nil {
		return "", 0
	}
	calls := resp.ToolCalls()
	var m struct {
		Finding int `json:"finding"`
	}
	if len(calls) != 1 || json.Unmarshal(calls[0].Input, &m) != nil || m.Finding < 0 || m.Finding >= len(findings) {
		return "", resp.Usage.CostUSD
	}
	return "semantic", resp.Usage.CostUSD
}
