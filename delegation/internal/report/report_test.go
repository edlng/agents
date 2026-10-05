package report

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/edlng/agents/litmus-eval/delegation/internal/schema"
)

func TestHeadingsComeFromArtifacts(t *testing.T) {
	in := Input{
		TaskID: "t1", Title: "Allowlist", CorrelationID: "cid", Status: "PENDING_HUMAN",
		Summary: "The change is blocked by one SSRF defect.",
		Artifacts: []Artifact{
			{ID: "code-review/security", Verdict: "FAIL", Decision: "accepted", Reason: "upheld", Workflow: "code-review", Substance: "core",
				Raw:    json.RawMessage(`{"findings":[{"id":"F1","severity":"critical","file":"e.go","line":35,"title":"CWE-918 | suffix match"}]}`),
				Review: schema.ArtifactReview{Verdict: "UPHELD"}, Prose: "F1 blocks the merge."},
			// The writer's prose claims success, but the heading follows the artifact.
			{ID: "spec-validation/tests", Verdict: "PASS", Decision: "unresolved", Reason: "reviewer disputes it", Workflow: "spec-validation", Substance: "core",
				Review:      schema.ArtifactReview{Verdict: "CHALLENGED", Challenges: []schema.Objection{{ID: "C1", Severity: "critical", File: "e_test.go", Line: 3, Claim: "tests cover AC2"}}},
				OpenCritics: 1, Prose: "Everything passed."},
		},
		Substance: [][2]string{{"code-review", "core"}, {"final-review", "core"}},
		CostUSD:   0.42, Calls: 9, Tokens: 12000,
	}
	md, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"## code-review/security: FAIL\n",
		"## spec-validation/tests: UNRESOLVED\n",
		"| code-review/security | FAIL | UPHELD | accepted |",
		"| F1 | critical | `e.go:35` | CWE-918 \\| suffix match |",
		"CHALLENGED (1 critical)",
		"9 model calls, 12000 tokens, $0.4200.",
		"| final-review | core |",
		"## Human decision\n\nPENDING_HUMAN",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("report missing %q\n%s", want, md)
		}
	}

	in.Artifacts[0].Prose = " "
	if _, err := Build(in); err == nil {
		t.Fatal("report built with an empty section")
	}
}
