// Package report assembles the final review. The structure (title, headings,
// verdict labels, finding index, disposition, cost, and human decision) comes
// from validated artifacts and harness state; the final review writer supplies
// only the prose placed under each heading.
package report

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/edlng/agents/litmus-eval/delegation/internal/schema"
)

type Artifact struct {
	ID          string
	Verdict     string // PASS | FAIL from schema.Kind.Verdict
	Raw         json.RawMessage
	Review      schema.ArtifactReview
	Decision    string // accepted | revised | unresolved
	Reason      string
	Workflow    string
	Substance   string
	Prose       string
	OpenCritics int // critical challenges in the latest review
}

type Input struct {
	TaskID        string
	Title         string
	CorrelationID string
	Status        string // PENDING_HUMAN | ELEVATED | COMPLETE | REJECTED_HUMAN
	Summary       string
	Artifacts     []Artifact
	CostUSD       float64
	Calls         int
	Tokens        int64
	Substance     [][2]string // workflow, substance, from the gate assessment
	Elevation     []string    // substance gate reasons; empty when not elevated
	Requests      []string    // request_human_decision reasons from the coordinator
}

// Heading is the label printed for an artifact. An open critical challenge
// overrides the artifact's own verdict.
func Heading(a Artifact) string {
	if a.Decision == "unresolved" && a.OpenCritics > 0 {
		return "UNRESOLVED"
	}
	return a.Verdict
}

// Build renders the report. It fails if the prose is missing for an artifact,
// so a report can never ship with a heading and no explanation.
func Build(in Input) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "# Change review: %s\n\n", in.Title)
	fmt.Fprintf(&b, "Task `%s` · correlation ID `%s` · status **%s**\n\n", in.TaskID, in.CorrelationID, in.Status)
	if strings.TrimSpace(in.Summary) == "" {
		return "", fmt.Errorf("report: summary is empty")
	}
	b.WriteString(strings.TrimSpace(in.Summary) + "\n\n")

	b.WriteString("| Artifact | Result | Adversarial review | Disposition |\n|---|---|---|---|\n")
	for _, a := range in.Artifacts {
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", a.ID, Heading(a), reviewLabel(a.Review), a.Decision)
	}
	b.WriteString("\n")

	for _, a := range in.Artifacts {
		if strings.TrimSpace(a.Prose) == "" {
			return "", fmt.Errorf("report: no prose for %s", a.ID)
		}
		fmt.Fprintf(&b, "## %s: %s\n\n", a.ID, Heading(a))
		fmt.Fprintf(&b, "Adversarial review: %s. Coordinator disposition: %s (%s).\n\n", reviewLabel(a.Review), a.Decision, strings.TrimRight(strings.TrimSpace(a.Reason), "."))
		b.WriteString(strings.TrimSpace(a.Prose) + "\n\n")
		if rows := findingRows(a.Raw); rows != "" {
			b.WriteString("| ID | Severity | Location | Title |\n|---|---|---|---|\n" + rows + "\n")
		}
		if rows := challengeRows(a.Review); rows != "" {
			b.WriteString("| Challenge | Severity | Location | Claim disputed |\n|---|---|---|---|\n" + rows + "\n")
		}
	}

	b.WriteString("## Workflow substance\n\n| Workflow | Substance |\n|---|---|\n")
	for _, row := range in.Substance {
		fmt.Fprintf(&b, "| %s | %s |\n", row[0], row[1])
	}
	fmt.Fprintf(&b, "\n## Run cost\n\n%d model calls, %d tokens, $%.4f.\n\n", in.Calls, in.Tokens, in.CostUSD)
	fmt.Fprintf(&b, "## Human decision\n\n%s\n", in.Status)
	if len(in.Elevation) > 0 {
		b.WriteString("\nSubstance gate elevated this review. A reviewer must record continue or reject:\n")
		for _, r := range in.Elevation {
			b.WriteString("- " + r + "\n")
		}
	}
	if len(in.Requests) > 0 {
		b.WriteString("\nThe coordinator asked for a human decision:\n")
		for _, r := range in.Requests {
			b.WriteString("- " + cell(r) + "\n")
		}
	}
	return b.String(), nil
}

func reviewLabel(r schema.ArtifactReview) string {
	if r.Verdict == "" {
		return "not reviewed"
	}
	if r.Verdict == "UPHELD" {
		return "UPHELD"
	}
	counts := map[string]int{}
	for _, c := range r.Challenges {
		counts[c.Severity]++
	}
	var parts []string
	for _, s := range []string{"critical", "major", "minor"} {
		if counts[s] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[s], s))
		}
	}
	return "CHALLENGED (" + strings.Join(parts, ", ") + ")"
}

func cell(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "|", `\|`), "\n", " ") }

func findingRows(raw json.RawMessage) string {
	var v struct {
		Findings []schema.Finding `json:"findings"`
	}
	json.Unmarshal(raw, &v)
	var b strings.Builder
	for _, f := range v.Findings {
		fmt.Fprintf(&b, "| %s | %s | `%s:%d` | %s |\n", f.ID, f.Severity, f.File, f.Line, cell(f.Title))
	}
	return b.String()
}

func challengeRows(r schema.ArtifactReview) string {
	var b strings.Builder
	for _, c := range r.Challenges {
		fmt.Fprintf(&b, "| %s | %s | `%s:%d` | %s |\n", c.ID, c.Severity, c.File, c.Line, cell(c.Claim))
	}
	return b.String()
}
