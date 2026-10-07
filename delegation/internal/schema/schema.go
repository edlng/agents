// Package schema defines every artifact a sub-agent can submit and the
// deterministic checks the harness runs before the coordinator sees it. A
// sub-agent submits by calling its submit tool; the input schema guides the
// model, and Validate is the authority.
package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/edlng/agents/litmus-eval/delegation/internal/provider"
	"github.com/edlng/agents/litmus-eval/delegation/internal/tools"
)

// Context is the evidence a validator checks a submission against.
type Context struct {
	Repo        string          // client repository, for file and line checks
	CriteriaIDs []string        // acceptance criteria of the task
	TestRuns    []tools.TestRun // run_tests calls the harness executed in this launch
	DocsWritten []string        // write_doc paths from this launch
	DocsOut     string          // where those docs were written
	// Reviewed lists the artifact IDs an adversarial review must cover, and
	// the artifact IDs a final review must have one section for.
	Reviewed []string
	// KnownIDs maps an artifact ID to the finding and challenge IDs raised
	// about it. The final review may cite only IDs that appear here.
	KnownIDs map[string][]string
	// ExtraFiles holds files that agents wrote to the run output (docs), by
	// path, so reviewers can cite them like repository files.
	ExtraFiles map[string]string
}

type Kind struct {
	Name      string
	Tool      provider.Tool
	normalize func(raw json.RawMessage) (json.RawMessage, error)
	validate  func(raw json.RawMessage, ctx Context) []string
	verdict   func(raw json.RawMessage) string
}

// Normalize applies harness-owned fields, such as finding and challenge IDs,
// before validation. Kinds without harness-owned fields return raw unchanged.
func (k Kind) Normalize(raw json.RawMessage) (json.RawMessage, error) {
	if k.normalize == nil {
		return raw, nil
	}
	return k.normalize(raw)
}

// numberItems sets "id" on each object under path to prefix + its 1-based
// position, numbering across nested lists in order.
func numberItems(raw json.RawMessage, prefix string, path ...string) (json.RawMessage, error) {
	var v map[string]any
	if err := json.Unmarshal(raw, &v); err != nil {
		return raw, nil // the validator reports the decode error
	}
	n := 0
	var walk func(node any, rest []string)
	walk = func(node any, rest []string) {
		obj, ok := node.(map[string]any)
		if !ok {
			return
		}
		list, _ := obj[rest[0]].([]any)
		for _, item := range list {
			if len(rest) > 1 {
				walk(item, rest[1:])
			} else if m, ok := item.(map[string]any); ok {
				n++
				m["id"] = fmt.Sprintf("%s%d", prefix, n)
			}
		}
	}
	walk(v, path)
	return json.Marshal(v)
}

// Validate returns every violation. An empty result means the artifact passed.
func (k Kind) Validate(raw json.RawMessage, ctx Context) []string { return k.validate(raw, ctx) }

// Verdict returns PASS or FAIL for a validated artifact. Report headings use
// it directly, so they never pass through a model.
func (k Kind) Verdict(raw json.RawMessage) string {
	if k.verdict == nil {
		return ""
	}
	return k.verdict(raw)
}

var kinds = map[string]Kind{}

func register(k Kind) { kinds[k.Name] = k }

func Lookup(name string) (Kind, error) {
	k, ok := kinds[name]
	if !ok {
		return Kind{}, fmt.Errorf("unknown artifact kind %q", name)
	}
	return k, nil
}

func decode(raw json.RawMessage, v any) []string {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return []string{"decode: " + err.Error()}
	}
	return nil
}

type problems []string

func (p *problems) add(format string, args ...any) { *p = append(*p, fmt.Sprintf(format, args...)) }

func (p *problems) need(ok bool, format string, args ...any) {
	if !ok {
		p.add(format, args...)
	}
}

func oneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}

func blank(s string) bool { return strings.TrimSpace(s) == "" }

// checkLocation verifies that a cited file exists in the repository (or in
// the run's written docs) and that the cited line is inside it. Line 0 is
// allowed only when wholeFile is set, and cites the file as a whole.
func checkLocation(p *problems, where string, ctx Context, file string, line int, wholeFile bool) {
	content, ok := ctx.ExtraFiles[file]
	if !ok {
		abs, err := tools.Resolve(ctx.Repo, file)
		if err != nil {
			p.add("%s: file %q: %v", where, file, err)
			return
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			p.add("%s: file %q is not readable", where, file)
			return
		}
		content = string(data)
	}
	if line == 0 && wholeFile {
		return
	}
	n := strings.Count(strings.TrimSuffix(content, "\n"), "\n") + 1
	if line < 1 || line > n {
		p.add("%s: line %d is outside %s (%d lines)", where, line, file, n)
	}
}

func schemaJSON(s string) json.RawMessage {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		panic(fmt.Sprintf("invalid built-in schema: %v", err))
	}
	return json.RawMessage(s)
}

const severityEnum = `{"type":"string","enum":["critical","major","minor"]}`

// ---- review.v1: code-review steps ----

type Review struct {
	Verdict  string    `json:"verdict"`
	Summary  string    `json:"summary"`
	Findings []Finding `json:"findings"`
}

type Finding struct {
	ID          string `json:"id"`
	Severity    string `json:"severity"`
	File        string `json:"file"`
	Line        int    `json:"line"`
	Title       string `json:"title"`
	Evidence    string `json:"evidence"`
	Remediation string `json:"remediation"`
}

var findingID = regexp.MustCompile(`^F\d+$`)

func init() {
	register(Kind{
		Name: "review.v1",
		Tool: provider.Tool{
			Name:        "submit_review",
			Description: "Submit the finished review. Call once, as your last action.",
			InputSchema: schemaJSON(`{"type":"object","properties":{
				"verdict":{"type":"string","enum":["APPROVE","BLOCK"]},
				"summary":{"type":"string"},
				"findings":{"type":"array","description":"The harness numbers findings F1, F2, ... in this order.","items":{"type":"object","properties":{
					"severity":` + severityEnum + `,
					"file":{"type":"string"},"line":{"type":"integer"},
					"title":{"type":"string"},"evidence":{"type":"string"},"remediation":{"type":"string"}},
					"required":["severity","file","line","title","evidence","remediation"]}}},
				"required":["verdict","summary","findings"]}`),
		},
		normalize: func(raw json.RawMessage) (json.RawMessage, error) { return numberItems(raw, "F", "findings") },
		validate: func(raw json.RawMessage, ctx Context) []string {
			var r Review
			if p := decode(raw, &r); p != nil {
				return p
			}
			var p problems
			p.need(oneOf(r.Verdict, "APPROVE", "BLOCK"), "verdict %q must be APPROVE or BLOCK", r.Verdict)
			p.need(!blank(r.Summary), "summary is empty")
			seen := map[string]bool{}
			blocking := 0
			for i, f := range r.Findings {
				where := fmt.Sprintf("findings[%d]", i)
				p.need(findingID.MatchString(f.ID), "%s: id %q must look like F1", where, f.ID)
				p.need(!seen[f.ID], "%s: duplicate id %s", where, f.ID)
				seen[f.ID] = true
				p.need(oneOf(f.Severity, "critical", "major", "minor"), "%s: severity %q", where, f.Severity)
				if f.Severity == "critical" || f.Severity == "major" {
					blocking++
				}
				p.need(!blank(f.Title) && !blank(f.Evidence) && !blank(f.Remediation), "%s: title, evidence, and remediation are required", where)
				checkLocation(&p, where, ctx, f.File, f.Line, false)
			}
			p.need(r.Verdict != "BLOCK" || blocking > 0, "BLOCK needs at least one critical or major finding")
			p.need(r.Verdict != "APPROVE" || blocking == 0, "APPROVE with %d critical or major findings", blocking)
			return p
		},
		verdict: func(raw json.RawMessage) string {
			var r Review
			json.Unmarshal(raw, &r)
			if r.Verdict == "APPROVE" {
				return "PASS"
			}
			return "FAIL"
		},
	})
}

// ---- tests.v1: spec-validation "tests" step ----

type Tests struct {
	ExitCode int           `json:"exit_code"`
	Passed   int           `json:"passed"`
	Failed   int           `json:"failed"`
	Failures []TestFailure `json:"failures"`
	Summary  string        `json:"summary"`
}

type TestFailure struct {
	Test    string `json:"test"`
	Message string `json:"message"`
}

func init() {
	register(Kind{
		Name: "tests.v1",
		Tool: provider.Tool{
			Name:        "submit_tests",
			Description: "Submit the test report. exit_code must be the exit code run_tests returned on its last run.",
			InputSchema: schemaJSON(`{"type":"object","properties":{
				"exit_code":{"type":"integer"},"passed":{"type":"integer"},"failed":{"type":"integer"},
				"failures":{"type":"array","items":{"type":"object","properties":{"test":{"type":"string"},"message":{"type":"string"}},"required":["test","message"]}},
				"summary":{"type":"string"}},
				"required":["exit_code","passed","failed","failures","summary"]}`),
		},
		validate: func(raw json.RawMessage, ctx Context) []string {
			var t Tests
			if p := decode(raw, &t); p != nil {
				return p
			}
			var p problems
			if len(ctx.TestRuns) == 0 {
				p.add("run_tests was never called")
				return p
			}
			last := ctx.TestRuns[len(ctx.TestRuns)-1]
			p.need(t.ExitCode == last.ExitCode, "exit_code %d does not match the recorded run (%d)", t.ExitCode, last.ExitCode)
			p.need(t.Passed >= 0 && t.Failed >= 0, "passed and failed must be non-negative")
			p.need(last.ExitCode == 0 || len(t.Failures) > 0, "a failing run needs at least one failure entry")
			p.need(last.ExitCode != 0 || len(t.Failures) == 0, "a passing run cannot list failures")
			p.need(!blank(t.Summary), "summary is empty")
			return p
		},
		verdict: func(raw json.RawMessage) string {
			var t Tests
			json.Unmarshal(raw, &t)
			if t.ExitCode == 0 {
				return "PASS"
			}
			return "FAIL"
		},
	})
}

// ---- criteria.v1: spec-validation "criteria" step ----

type Criteria struct {
	Verdict  string      `json:"verdict"`
	Criteria []Criterion `json:"criteria"`
}

type Criterion struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Evidence string `json:"evidence"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
}

func init() {
	register(Kind{
		Name: "criteria.v1",
		Tool: provider.Tool{
			Name:        "submit_criteria",
			Description: "Submit one PASS or FAIL per acceptance criterion. Cite file and line when the evidence is in code.",
			InputSchema: schemaJSON(`{"type":"object","properties":{
				"verdict":{"type":"string","enum":["PASS","FAIL"]},
				"criteria":{"type":"array","items":{"type":"object","properties":{
					"id":{"type":"string"},"status":{"type":"string","enum":["PASS","FAIL"]},
					"evidence":{"type":"string"},"file":{"type":"string"},
					"line":{"type":"integer","description":"line in file; omit to cite the whole file"}},
					"required":["id","status","evidence"]}}},
				"required":["verdict","criteria"]}`),
		},
		validate: func(raw json.RawMessage, ctx Context) []string {
			var c Criteria
			if p := decode(raw, &c); p != nil {
				return p
			}
			var p problems
			want := map[string]bool{}
			for _, id := range ctx.CriteriaIDs {
				want[id] = true
			}
			seen := map[string]bool{}
			failed := false
			for i, cr := range c.Criteria {
				where := fmt.Sprintf("criteria[%d]", i)
				p.need(want[cr.ID], "%s: %q is not an acceptance criterion of this task", where, cr.ID)
				p.need(!seen[cr.ID], "%s: duplicate %s", where, cr.ID)
				seen[cr.ID] = true
				p.need(oneOf(cr.Status, "PASS", "FAIL"), "%s: status %q", where, cr.Status)
				failed = failed || cr.Status == "FAIL"
				p.need(!blank(cr.Evidence), "%s: evidence is empty", where)
				if cr.File != "" {
					checkLocation(&p, where, ctx, cr.File, cr.Line, true)
				}
			}
			for _, id := range ctx.CriteriaIDs {
				p.need(seen[id], "criterion %s is missing", id)
			}
			p.need(oneOf(c.Verdict, "PASS", "FAIL"), "verdict %q must be PASS or FAIL", c.Verdict)
			p.need((c.Verdict == "FAIL") == failed, "verdict %s disagrees with the per-criterion statuses", c.Verdict)
			return p
		},
		verdict: func(raw json.RawMessage) string {
			var c Criteria
			json.Unmarshal(raw, &c)
			return c.Verdict
		},
	})
}

// ---- documentation.v1: documentation "write" step ----

type Documentation struct {
	Status  string   `json:"status"`
	Files   []string `json:"files"`
	Summary string   `json:"summary"`
}

var requiredSections = []string{"overview", "usage", "limitations"}

var heading = regexp.MustCompile(`(?m)^#{1,6}\s+(.+?)\s*$`)

func init() {
	register(Kind{
		Name: "documentation.v1",
		Tool: provider.Tool{
			Name:        "submit_documentation",
			Description: "Submit after writing the docs with write_doc. List every file you wrote.",
			InputSchema: schemaJSON(`{"type":"object","properties":{
				"status":{"type":"string","enum":["COMPLETE","INCOMPLETE"]},
				"files":{"type":"array","items":{"type":"string"}},
				"summary":{"type":"string"}},
				"required":["status","files","summary"]}`),
		},
		validate: func(raw json.RawMessage, ctx Context) []string {
			var d Documentation
			if p := decode(raw, &d); p != nil {
				return p
			}
			var p problems
			p.need(oneOf(d.Status, "COMPLETE", "INCOMPLETE"), "status %q", d.Status)
			p.need(!blank(d.Summary), "summary is empty")
			p.need(d.Status != "COMPLETE" || len(d.Files) > 0, "COMPLETE with no files")
			for _, f := range d.Files {
				if !oneOf(f, ctx.DocsWritten...) {
					p.add("%s was not written with write_doc in this launch", f)
					continue
				}
				data, err := os.ReadFile(ctx.DocsOut + "/" + f)
				if err != nil {
					p.add("%s: %v", f, err)
					continue
				}
				found := map[string]bool{}
				for _, m := range heading.FindAllStringSubmatch(string(data), -1) {
					found[strings.ToLower(m[1])] = true
				}
				for _, s := range requiredSections {
					p.need(found[s], "%s: missing a %q heading", f, s)
				}
			}
			return p
		},
		verdict: func(raw json.RawMessage) string {
			var d Documentation
			json.Unmarshal(raw, &d)
			if d.Status == "COMPLETE" {
				return "PASS"
			}
			return "FAIL"
		},
	})
}

// ---- challenge.v1: adversarial review ----

type Challenge struct {
	Reviews []ArtifactReview `json:"reviews"`
}

type ArtifactReview struct {
	ArtifactID string      `json:"artifact_id"`
	Verdict    string      `json:"verdict"`
	Challenges []Objection `json:"challenges"`
}

type Objection struct {
	ID              string `json:"id"`
	Severity        string `json:"severity"`
	Claim           string `json:"claim"`
	CounterEvidence string `json:"counter_evidence"`
	File            string `json:"file"`
	Line            int    `json:"line"`
}

var challengeID = regexp.MustCompile(`^C\d+$`)

func init() {
	register(Kind{
		Name: "challenge.v1",
		Tool: provider.Tool{
			Name:        "submit_challenges",
			Description: "Submit one review per artifact. UPHELD means every claim you checked holds; CHALLENGED needs at least one challenge with counter-evidence from the repository.",
			InputSchema: schemaJSON(`{"type":"object","properties":{
				"reviews":{"type":"array","items":{"type":"object","properties":{
					"artifact_id":{"type":"string"},
					"verdict":{"type":"string","enum":["UPHELD","CHALLENGED"]},
					"challenges":{"type":"array","description":"The harness numbers challenges C1, C2, ... across the submission.","items":{"type":"object","properties":{
						"severity":` + severityEnum + `,
						"claim":{"type":"string","description":"the artifact's claim you dispute, quoted or paraphrased"},
						"counter_evidence":{"type":"string"},
						"file":{"type":"string"},"line":{"type":"integer"}},
						"required":["severity","claim","counter_evidence","file","line"]}}},
					"required":["artifact_id","verdict","challenges"]}}},
				"required":["reviews"]}`),
		},
		normalize: func(raw json.RawMessage) (json.RawMessage, error) {
			return numberItems(raw, "C", "reviews", "challenges")
		},
		validate: func(raw json.RawMessage, ctx Context) []string {
			var c Challenge
			if p := decode(raw, &c); p != nil {
				return p
			}
			var p problems
			covered := map[string]bool{}
			ids := map[string]bool{}
			for i, r := range c.Reviews {
				where := fmt.Sprintf("reviews[%d]", i)
				p.need(oneOf(r.ArtifactID, ctx.Reviewed...), "%s: %q was not assigned for review", where, r.ArtifactID)
				p.need(!covered[r.ArtifactID], "%s: %s reviewed twice", where, r.ArtifactID)
				covered[r.ArtifactID] = true
				p.need(oneOf(r.Verdict, "UPHELD", "CHALLENGED"), "%s: verdict %q", where, r.Verdict)
				p.need((r.Verdict == "CHALLENGED") == (len(r.Challenges) > 0), "%s: CHALLENGED requires challenges and UPHELD allows none", where)
				for j, o := range r.Challenges {
					w := fmt.Sprintf("%s.challenges[%d]", where, j)
					p.need(challengeID.MatchString(o.ID), "%s: id %q must look like C1", w, o.ID)
					p.need(!ids[o.ID], "%s: duplicate id %s", w, o.ID)
					ids[o.ID] = true
					p.need(oneOf(o.Severity, "critical", "major", "minor"), "%s: severity %q", w, o.Severity)
					p.need(!blank(o.Claim) && !blank(o.CounterEvidence), "%s: claim and counter_evidence are required", w)
					checkLocation(&p, w, ctx, o.File, o.Line, false)
				}
			}
			for _, id := range ctx.Reviewed {
				p.need(covered[id], "artifact %s has no review", id)
			}
			return p
		},
	})
}

// ---- final-review.v1: final review writer ----

type FinalReview struct {
	Summary  string    `json:"summary"`
	Sections []Section `json:"sections"`
}

type Section struct {
	ArtifactID string `json:"artifact_id"`
	Prose      string `json:"prose"`
}

var citedID = regexp.MustCompile(`\b[FC]\d+\b`)
var headingLine = regexp.MustCompile(`(?m)^\s{0,3}#`)

func init() {
	register(Kind{
		Name: "final-review.v1",
		Tool: provider.Tool{
			Name:        "submit_final_review",
			Description: "Submit the report prose: a short summary and one section per artifact. Plain paragraphs and lists only; the harness adds every heading.",
			InputSchema: schemaJSON(`{"type":"object","properties":{
				"summary":{"type":"string"},
				"sections":{"type":"array","items":{"type":"object","properties":{
					"artifact_id":{"type":"string"},"prose":{"type":"string"}},
					"required":["artifact_id","prose"]}}},
				"required":["summary","sections"]}`),
		},
		validate: func(raw json.RawMessage, ctx Context) []string {
			var f FinalReview
			if p := decode(raw, &f); p != nil {
				return p
			}
			var p problems
			p.need(!blank(f.Summary), "summary is empty")
			p.need(!headingLine.MatchString(f.Summary), "summary contains a markdown heading")
			for _, id := range citedID.FindAllString(f.Summary, -1) {
				p.need(knownAnywhere(ctx.KnownIDs, id), "summary cites %s, which no agent raised", id)
			}
			covered := map[string]bool{}
			for i, s := range f.Sections {
				where := fmt.Sprintf("sections[%d]", i)
				p.need(oneOf(s.ArtifactID, ctx.Reviewed...), "%s: unknown artifact %q", where, s.ArtifactID)
				p.need(!covered[s.ArtifactID], "%s: %s has two sections", where, s.ArtifactID)
				covered[s.ArtifactID] = true
				p.need(!blank(s.Prose), "%s: prose is empty", where)
				p.need(!headingLine.MatchString(s.Prose), "%s: prose contains a markdown heading", where)
				for _, id := range citedID.FindAllString(s.Prose, -1) {
					p.need(knownAnywhere(ctx.KnownIDs, id), "%s: cites %s, which no agent raised", where, id)
				}
			}
			for _, id := range ctx.Reviewed {
				p.need(covered[id], "artifact %s has no section", id)
			}
			return p
		},
	})
}

func knownAnywhere(known map[string][]string, id string) bool {
	for _, ids := range known {
		if oneOf(id, ids...) {
			return true
		}
	}
	return false
}
