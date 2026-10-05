package schema

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/edlng/agents/litmus-eval/delegation/internal/tools"
)

func repo(t *testing.T) string {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "calc.go"), []byte("package calc\n\nfunc Div(a, b int) int {\n\treturn a / b\n}\n"), 0o644)
	return dir
}

// check validates raw and requires the violations to mention each want, or
// none at all when want is empty.
func check(t *testing.T, kind, raw string, ctx Context, want ...string) {
	t.Helper()
	k, err := Lookup(kind)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(k.Validate(json.RawMessage(raw), ctx), "\n")
	if len(want) == 0 && got != "" {
		t.Fatalf("unexpected violations:\n%s", got)
	}
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("violations missing %q:\n%s", w, got)
		}
	}
}

func TestReview(t *testing.T) {
	ctx := Context{Repo: repo(t)}
	finding := `{"id":"F1","severity":"major","file":"calc.go","line":4,"title":"divide by zero","evidence":"b is unchecked","remediation":"guard b == 0"}`
	check(t, "review.v1", `{"verdict":"BLOCK","summary":"s","findings":[`+finding+`]}`, ctx)
	check(t, "review.v1", `{"verdict":"APPROVE","summary":"s","findings":[`+finding+`]}`, ctx, "APPROVE with 1 critical or major")
	check(t, "review.v1", `{"verdict":"BLOCK","summary":"s","findings":[]}`, ctx, "BLOCK needs")
	check(t, "review.v1", `{"verdict":"BLOCK","summary":"s","findings":[`+strings.Replace(finding, `"line":4`, `"line":99`, 1)+`]}`, ctx, "line 99 is outside calc.go")
	check(t, "review.v1", `{"verdict":"BLOCK","summary":"s","findings":[`+strings.Replace(finding, "calc.go", "../etc/passwd", 1)+`]}`, ctx, "outside the repository")
	check(t, "review.v1", `{"verdict":"APPROVE","summary":"s","findings":[],"extra":1}`, ctx, "unknown field")
	if v := kinds["review.v1"].Verdict(json.RawMessage(`{"verdict":"BLOCK"}`)); v != "FAIL" {
		t.Fatalf("verdict = %s", v)
	}
}

func TestTestsMustMatchRecordedRun(t *testing.T) {
	ctx := Context{TestRuns: []tools.TestRun{{ExitCode: 1}}}
	check(t, "tests.v1", `{"exit_code":1,"passed":2,"failed":1,"failures":[{"test":"TestDiv","message":"panic"}],"summary":"1 failure"}`, ctx)
	check(t, "tests.v1", `{"exit_code":0,"passed":3,"failed":0,"failures":[],"summary":"all pass"}`, ctx, "does not match the recorded run (1)")
	check(t, "tests.v1", `{"exit_code":0,"passed":3,"failed":0,"failures":[],"summary":"all pass"}`, Context{}, "run_tests was never called")
}

func TestCriteria(t *testing.T) {
	ctx := Context{Repo: repo(t), CriteriaIDs: []string{"AC1", "AC2"}}
	check(t, "criteria.v1", `{"verdict":"FAIL","criteria":[{"id":"AC1","status":"PASS","evidence":"e"},{"id":"AC2","status":"FAIL","evidence":"e","file":"calc.go","line":4}]}`, ctx)
	check(t, "criteria.v1", `{"verdict":"PASS","criteria":[{"id":"AC1","status":"PASS","evidence":"e"},{"id":"AC2","status":"FAIL","evidence":"e"}]}`, ctx, "disagrees")
	check(t, "criteria.v1", `{"verdict":"PASS","criteria":[{"id":"AC1","status":"PASS","evidence":"e"},{"id":"AC9","status":"PASS","evidence":"e"}]}`, ctx, `"AC9" is not an acceptance criterion`, "criterion AC2 is missing")
}

func TestDocumentation(t *testing.T) {
	out := t.TempDir()
	os.MkdirAll(filepath.Join(out, "docs"), 0o755)
	os.WriteFile(filepath.Join(out, "docs", "calc.md"), []byte("# Calc\n## Overview\nx\n## Usage\ny\n"), 0o644)
	ctx := Context{DocsOut: out, DocsWritten: []string{"docs/calc.md"}}
	check(t, "documentation.v1", `{"status":"COMPLETE","files":["docs/calc.md"],"summary":"s"}`, ctx, `missing a "limitations" heading`)
	check(t, "documentation.v1", `{"status":"COMPLETE","files":["docs/other.md"],"summary":"s"}`, ctx, "was not written with write_doc")
}

func TestChallenge(t *testing.T) {
	ctx := Context{Repo: repo(t), Reviewed: []string{"code-review/security", "spec-validation/tests"}}
	c := `{"id":"C1","severity":"critical","claim":"no division bug","counter_evidence":"b unchecked","file":"calc.go","line":4}`
	check(t, "challenge.v1", `{"reviews":[{"artifact_id":"code-review/security","verdict":"CHALLENGED","challenges":[`+c+`]},{"artifact_id":"spec-validation/tests","verdict":"UPHELD","challenges":[]}]}`, ctx)
	check(t, "challenge.v1", `{"reviews":[{"artifact_id":"code-review/security","verdict":"UPHELD","challenges":[`+c+`]}]}`, ctx, "UPHELD allows none", "spec-validation/tests has no review")
	check(t, "challenge.v1", `{"reviews":[{"artifact_id":"documentation/write","verdict":"UPHELD","challenges":[]}]}`, ctx, "was not assigned")
}

func TestFinalReviewAddsNothingNew(t *testing.T) {
	ctx := Context{Reviewed: []string{"code-review/security"}, KnownIDs: map[string][]string{"code-review/security": {"F1", "C1"}}}
	check(t, "final-review.v1", `{"summary":"One blocking issue.","sections":[{"artifact_id":"code-review/security","prose":"F1 stands; C1 was resolved."}]}`, ctx)
	check(t, "final-review.v1", `{"summary":"s","sections":[{"artifact_id":"code-review/security","prose":"F1 and also F7."}]}`, ctx, "cites F7, which no agent raised")
	cross := Context{Reviewed: []string{"code-review/security", "documentation/write"}, KnownIDs: map[string][]string{"code-review/security": {"F1"}}}
	check(t, "final-review.v1", `{"summary":"s","sections":[{"artifact_id":"code-review/security","prose":"F1 blocks."},{"artifact_id":"documentation/write","prose":"The docs record F1 in code-review/security."}]}`, cross)
	check(t, "final-review.v1", `{"summary":"s","sections":[{"artifact_id":"code-review/security","prose":"## PASS\nok"}]}`, ctx, "contains a markdown heading")
	check(t, "final-review.v1", `{"summary":"s","sections":[]}`, ctx, "has no section")
}
