// Package dispatch turns a coordinator decision into a sub-agent launch. It
// builds each launch's input from the task and the artifacts the coordinator
// chose to pass, enforces launch limits, and keeps the latest valid artifact
// per workflow step. It is the only path from the coordinator to sub-agents.
package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/edlng/agents/litmus-eval/delegation/internal/audit"
	"github.com/edlng/agents/litmus-eval/delegation/internal/catalog"
	"github.com/edlng/agents/litmus-eval/delegation/internal/provider"
	"github.com/edlng/agents/litmus-eval/delegation/internal/schema"
	"github.com/edlng/agents/litmus-eval/delegation/internal/subagent"
	"github.com/edlng/agents/litmus-eval/delegation/internal/task"
	"github.com/edlng/agents/litmus-eval/delegation/internal/tools"
)

const (
	ReviewerID = "adversarial-review"
	WriterID   = "final-review"
)

type Limits struct {
	PerArtifact int     // launches allowed per workflow step
	PerRun      int     // launches allowed in total
	BudgetUSD   float64 // spend at which further launches are refused
}

var DefaultLimits = Limits{PerArtifact: 3, PerRun: 14, BudgetUSD: 5}

type Runner struct {
	Provider provider.Provider
	Log      *audit.Log
	Catalog  *catalog.Catalog
	Task     *task.Task
	Limits   Limits

	mu         sync.Mutex
	artifacts  map[string]subagent.Result         // latest valid artifact per artifact ID
	reviews    map[string]schema.ArtifactReview   // latest review per artifact
	history    map[string][]schema.ArtifactReview // every review round per artifact
	reviewedAt map[string]int                     // artifact version when last reviewed
	versions   map[string]int
	launches   map[string]int
	total      int
	spent      float64
	final      *subagent.Result
	dispose    []Disposition
}

func New(p provider.Provider, log *audit.Log, c *catalog.Catalog, t *task.Task, limits Limits) *Runner {
	return &Runner{
		Provider: p, Log: log, Catalog: c, Task: t, Limits: limits,
		artifacts: map[string]subagent.Result{}, reviews: map[string]schema.ArtifactReview{}, history: map[string][]schema.ArtifactReview{},
		reviewedAt: map[string]int{}, versions: map[string]int{}, launches: map[string]int{},
	}
}

// ArtifactID names the artifact a workflow step produces.
func ArtifactID(workflow, step string) string { return workflow + "/" + step }

func refused(id, code, msg string) subagent.Result {
	return subagent.Result{ArtifactID: id, Error: &subagent.Error{Code: code, Message: msg}}
}

// reserve checks the limits and counts a launch before it runs.
func (r *Runner) reserve(id string) *subagent.Result {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch {
	case r.total >= r.Limits.PerRun:
		res := refused(id, "launch_limit", fmt.Sprintf("run limit of %d launches reached", r.Limits.PerRun))
		return &res
	case r.launches[id] >= r.Limits.PerArtifact:
		res := refused(id, "launch_limit", fmt.Sprintf("%s already launched %d times", id, r.launches[id]))
		return &res
	case r.spent >= r.Limits.BudgetUSD:
		res := refused(id, "budget_exhausted", fmt.Sprintf("spent $%.4f of the $%.2f run budget", r.spent, r.Limits.BudgetUSD))
		return &res
	}
	r.launches[id]++
	r.total++
	return nil
}

func (r *Runner) record(res subagent.Result) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.spent += res.CostUSD
	if res.OK {
		r.artifacts[res.ArtifactID] = res
		r.versions[res.ArtifactID]++
	}
}

// LaunchStep runs one step of a client workflow. contextIDs names other
// artifacts the coordinator wants the agent to see; challenges carries prior
// adversarial challenges against this step's artifact.
func (r *Runner) LaunchStep(ctx context.Context, workflowID, stepID string, contextIDs []string, challenges json.RawMessage, parent string) subagent.Result {
	id := ArtifactID(workflowID, stepID)
	w := r.Catalog.Workflows[workflowID]
	if w == nil || w.Role != "" || workflowID == WriterID {
		return refused(id, "unknown_workflow", fmt.Sprintf("%q is not a client workflow", workflowID))
	}
	step, err := w.Step(stepID)
	if err != nil {
		return refused(id, "unknown_step", err.Error())
	}
	spec, err := w.Spec(stepID)
	if err != nil {
		return refused(id, "config_error", err.Error())
	}
	var b strings.Builder
	b.WriteString(r.Task.Brief())
	for _, in := range step.Inputs {
		dep := ArtifactID(workflowID, in)
		a, ok := r.Artifact(dep)
		if !ok {
			return refused(id, "missing_input", fmt.Sprintf("step %s needs a valid %s artifact first", stepID, dep))
		}
		writeArtifact(&b, "Input artifact", a)
	}
	for _, cid := range contextIDs {
		a, ok := r.Artifact(cid)
		if !ok {
			return refused(id, "missing_input", fmt.Sprintf("context artifact %s does not exist or is not valid", cid))
		}
		writeArtifact(&b, "Context artifact", a)
	}
	if len(challenges) > 0 && string(challenges) != "null" {
		fmt.Fprintf(&b, "\n## Prior challenges against your artifact\n```json\n%s\n```\n", challenges)
	}
	if res := r.reserve(id); res != nil {
		return *res
	}
	res := subagent.Run(ctx, r.Provider, r.Log, subagent.Launch{
		Spec: spec, ArtifactID: id, Input: b.String(), ParentSpan: parent,
		Env: tools.Env{
			Repo: r.Task.RepoDir, TestCommand: r.Task.TestCommand,
			DocsOut: filepath.Join(r.Log.Dir(), "artifacts", workflowID, stepID+"-out"),
		},
		Validation: schema.Context{CriteriaIDs: r.Task.CriteriaIDs()},
	})
	r.record(res)
	return res
}

// LaunchReview runs the adversarial reviewer in a fresh context over the
// named artifacts.
func (r *Runner) LaunchReview(ctx context.Context, artifactIDs []string, parent string) subagent.Result {
	id := fmt.Sprintf("%s/round-%d", ReviewerID, r.launchCount(ReviewerID)+1)
	if len(artifactIDs) == 0 {
		return refused(id, "missing_input", "no artifacts to review")
	}
	w := r.Catalog.Workflows[ReviewerID]
	spec, err := w.Spec("review")
	if err != nil {
		return refused(id, "config_error", err.Error())
	}
	var b strings.Builder
	b.WriteString(r.Task.Brief())
	b.WriteString("\n## Artifacts to review\n")
	versions := map[string]int{}
	extra := map[string]string{}
	for _, aid := range artifactIDs {
		a, ok := r.Artifact(aid)
		if !ok {
			return refused(id, "missing_input", fmt.Sprintf("artifact %s does not exist or is not valid", aid))
		}
		writeArtifact(&b, "Artifact", a)
		versions[aid] = r.version(aid)
		for path, content := range a.Files {
			extra[path] = content
		}
	}
	if res := r.reserve(ReviewerID); res != nil {
		res.ArtifactID = id
		return *res
	}
	res := subagent.Run(ctx, r.Provider, r.Log, subagent.Launch{
		Spec: spec, ArtifactID: id, Input: b.String(), ParentSpan: parent,
		Env:        tools.Env{Repo: r.Task.RepoDir},
		Validation: schema.Context{Reviewed: artifactIDs, ExtraFiles: extra},
	})
	r.record(res)
	if res.OK {
		var c schema.Challenge
		json.Unmarshal(res.Artifact, &c)
		r.mu.Lock()
		for _, rv := range c.Reviews {
			r.reviews[rv.ArtifactID] = rv
			r.history[rv.ArtifactID] = append(r.history[rv.ArtifactID], rv)
			r.reviewedAt[rv.ArtifactID] = versions[rv.ArtifactID]
		}
		r.mu.Unlock()
	}
	return res
}

// Unreviewed lists valid artifacts whose current version has no adversarial
// review. The final review cannot run while any remain.
func (r *Runner) Unreviewed() []string {
	ids := r.ArtifactIDs()
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, id := range ids {
		if r.reviewedAt[id] != r.versions[id] {
			out = append(out, id)
		}
	}
	return out
}

// Disposition is the coordinator's decision about one artifact.
type Disposition struct {
	ArtifactID string `json:"artifact_id"`
	Decision   string `json:"decision"` // accepted | revised | unresolved
	Reason     string `json:"reason"`
}

// LaunchFinal runs the final review writer over every artifact, review, and
// disposition. It refuses while an artifact lacks a current review.
func (r *Runner) LaunchFinal(ctx context.Context, dispositions []Disposition, parent string) subagent.Result {
	id := WriterID + "/write"
	if pending := r.Unreviewed(); len(pending) > 0 {
		return refused(id, "review_pending", "adversarial review is missing or stale for: "+strings.Join(pending, ", "))
	}
	ids := r.ArtifactIDs()
	if len(ids) == 0 {
		return refused(id, "missing_input", "no valid artifacts to report on")
	}
	byID := map[string]Disposition{}
	for _, d := range dispositions {
		if d.Decision != "accepted" && d.Decision != "revised" && d.Decision != "unresolved" {
			return refused(id, "invalid_disposition", fmt.Sprintf("%s: decision %q must be accepted, revised, or unresolved", d.ArtifactID, d.Decision))
		}
		byID[d.ArtifactID] = d
	}
	for _, aid := range ids {
		if _, ok := byID[aid]; !ok {
			return refused(id, "invalid_disposition", "no disposition for "+aid)
		}
	}
	spec, err := r.Catalog.Workflows[WriterID].Spec("write")
	if err != nil {
		return refused(id, "config_error", err.Error())
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Task %s: %s\n\n%s\n", r.Task.ID, r.Task.Title, r.Task.Description)
	known := map[string][]string{}
	for _, aid := range ids {
		a, _ := r.Artifact(aid)
		writeArtifact(&b, "Artifact", a)
		rounds := r.History(aid)
		rvJSON, _ := json.MarshalIndent(rounds, "", "  ")
		dJSON, _ := json.MarshalIndent(byID[aid], "", "  ")
		fmt.Fprintf(&b, "Adversarial review rounds for %s, oldest first (the last round reviewed the current version):\n```json\n%s\n```\nCoordinator disposition:\n```json\n%s\n```\n", aid, rvJSON, dJSON)
		known[aid] = findingIDs(a.Artifact)
		for _, rv := range rounds {
			known[aid] = append(known[aid], challengeIDs(rv)...)
		}
	}
	if res := r.reserve(WriterID); res != nil {
		res.ArtifactID = id
		return *res
	}
	res := subagent.Run(ctx, r.Provider, r.Log, subagent.Launch{
		Spec: spec, ArtifactID: id, Input: b.String(), ParentSpan: parent,
		Env:        tools.Env{Repo: r.Task.RepoDir},
		Validation: schema.Context{Reviewed: ids, KnownIDs: known},
	})
	r.record(res)
	if res.OK {
		r.mu.Lock()
		r.final = &res
		r.dispose = dispositions
		r.mu.Unlock()
	}
	return res
}

// Dispositions returns the dispositions the final review was written with.
func (r *Runner) Dispositions() []Disposition {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Disposition(nil), r.dispose...)
}

// Seed installs a pre-built valid artifact. Evaluations use it to give the
// reviewer and writer planted artifacts; the coordinator cannot call it.
func (r *Runner) Seed(res subagent.Result) {
	res.OK = true
	r.record(res)
}

// SeedReview installs an adversarial review round for the current version of
// an artifact, for writer evaluations.
func (r *Runner) SeedReview(rv schema.ArtifactReview) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reviews[rv.ArtifactID] = rv
	r.history[rv.ArtifactID] = append(r.history[rv.ArtifactID], rv)
	r.reviewedAt[rv.ArtifactID] = r.versions[rv.ArtifactID]
}

func (r *Runner) Artifact(id string) (subagent.Result, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.artifacts[id]
	return a, ok
}

// ArtifactIDs lists the client artifacts with a valid version, sorted.
func (r *Runner) ArtifactIDs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var ids []string
	for id := range r.artifacts {
		if !strings.HasPrefix(id, ReviewerID+"/") && !strings.HasPrefix(id, WriterID+"/") {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func (r *Runner) Review(id string) schema.ArtifactReview {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reviews[id]
}

// History returns every adversarial review round of an artifact, oldest first.
func (r *Runner) History(id string) []schema.ArtifactReview {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]schema.ArtifactReview(nil), r.history[id]...)
}

func (r *Runner) Final() *subagent.Result {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.final
}

func (r *Runner) Spent() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.spent
}

func (r *Runner) version(id string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.versions[id]
}

func (r *Runner) launchCount(id string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.launches[id]
}

func writeArtifact(b *strings.Builder, label string, a subagent.Result) {
	pretty, _ := json.MarshalIndent(json.RawMessage(a.Artifact), "", "  ")
	fmt.Fprintf(b, "\n### %s %s (verdict %s)\n```json\n%s\n```\n", label, a.ArtifactID, a.Verdict, pretty)
	paths := make([]string, 0, len(a.Files))
	for path := range a.Files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		fmt.Fprintf(b, "\nFile %s written by this artifact (in the review output, not the repository):\n````markdown\n%s\n````\n", path, numbered(a.Files[path]))
	}
}

func numbered(s string) string {
	lines := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
	for i := range lines {
		lines[i] = fmt.Sprintf("%d\t%s", i+1, lines[i])
	}
	return strings.Join(lines, "\n")
}

func findingIDs(raw json.RawMessage) []string {
	var v struct {
		Findings []struct {
			ID string `json:"id"`
		} `json:"findings"`
	}
	json.Unmarshal(raw, &v)
	var ids []string
	for _, f := range v.Findings {
		ids = append(ids, f.ID)
	}
	return ids
}

func challengeIDs(rv schema.ArtifactReview) []string {
	var ids []string
	for _, c := range rv.Challenges {
		ids = append(ids, c.ID)
	}
	return ids
}
