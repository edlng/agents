// Package catalog loads the workflow manifests under delegation/workflows and
// turns a workflow step into a sub-agent spec. The manifests are the declared
// tool scope of every sub-agent; Load rejects a step that asks for a tool its
// workflow does not declare.
package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/edlng/agents/litmus-eval/delegation/internal/schema"
	"github.com/edlng/agents/litmus-eval/delegation/internal/subagent"
)

type Step struct {
	ID          string   `json:"id"`
	Prompt      string   `json:"prompt"`
	Output      string   `json:"output"`
	Tools       []string `json:"tools,omitempty"`  // defaults to the workflow's tools
	Inputs      []string `json:"inputs,omitempty"` // earlier steps of this workflow whose artifacts this step reads
	Description string   `json:"description"`
}

type Evaluation struct {
	Criteria []string `json:"criteria"`
	Results  string   `json:"results"`
}

type Workflow struct {
	ID                 string     `json:"id"`
	Agent              string     `json:"agent"`
	Role               string     `json:"role,omitempty"` // "" for client workflows, "reviewer" for the adversarial reviewer
	Purpose            string     `json:"purpose"`
	Substance          string     `json:"substance"` // core | peripheral | toy | not-assessed
	SubstanceRationale string     `json:"substance_rationale"`
	Source             string     `json:"source,omitempty"`
	Model              string     `json:"model"`
	Effort             string     `json:"effort"`
	MaxTurns           int        `json:"max_turns"`
	MaxTokens          int        `json:"max_tokens"`
	IsolatedContext    bool       `json:"isolated_context"`
	Tools              []string   `json:"tools"`
	Steps              []Step     `json:"steps"`
	Evaluation         Evaluation `json:"evaluation"`

	dir   string
	trust string
}

type Catalog struct {
	Workflows map[string]*Workflow
}

// Load reads every <dir>/<id>/manifest.json.
func Load(dir string) (*Catalog, error) {
	trust, err := os.ReadFile(filepath.Join(dir, "trust.md"))
	if err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*", "manifest.json"))
	if err != nil {
		return nil, err
	}
	c := &Catalog{Workflows: map[string]*Workflow{}}
	for _, path := range paths {
		w, err := load(path, string(trust))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if c.Workflows[w.ID] != nil {
			return nil, fmt.Errorf("%s: duplicate workflow %s", path, w.ID)
		}
		c.Workflows[w.ID] = w
	}
	if len(c.Workflows) == 0 {
		return nil, fmt.Errorf("no workflow manifests under %s", dir)
	}
	return c, nil
}

func load(path, trust string) (*Workflow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var w Workflow
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&w); err != nil {
		return nil, err
	}
	w.dir, w.trust = filepath.Dir(path), trust
	if w.ID != filepath.Base(w.dir) {
		return nil, fmt.Errorf("id %q does not match directory %q", w.ID, filepath.Base(w.dir))
	}
	switch {
	case w.Substance == "not-assessed" && w.Role != "reviewer":
		return nil, fmt.Errorf("only the reviewer role may skip the substance assessment")
	case w.Substance != "not-assessed" && w.Substance != "core" && w.Substance != "peripheral" && w.Substance != "toy":
		return nil, fmt.Errorf("substance %q must be core, peripheral, or toy", w.Substance)
	case !w.IsolatedContext:
		return nil, fmt.Errorf("isolated_context must be true")
	case w.Model == "" || w.MaxTurns < 1 || w.MaxTokens < 1:
		return nil, fmt.Errorf("model, max_turns, and max_tokens are required")
	case len(w.Steps) == 0:
		return nil, fmt.Errorf("no steps")
	case len(w.Evaluation.Criteria) < 3:
		return nil, fmt.Errorf("needs at least 3 evaluation criteria, has %d", len(w.Evaluation.Criteria))
	}
	if _, err := os.Stat(filepath.Join(w.dir, "prompt.md")); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, s := range w.Steps {
		if seen[s.ID] {
			return nil, fmt.Errorf("duplicate step %s", s.ID)
		}
		for _, in := range s.Inputs {
			if !seen[in] {
				return nil, fmt.Errorf("step %s reads %s, which is not an earlier step", s.ID, in)
			}
		}
		seen[s.ID] = true
		if _, err := schema.Lookup(s.Output); err != nil {
			return nil, fmt.Errorf("step %s: %w", s.ID, err)
		}
		for _, t := range s.Tools {
			if !contains(w.Tools, t) {
				return nil, fmt.Errorf("step %s uses %s, which the workflow does not declare", s.ID, t)
			}
		}
		if _, err := os.Stat(filepath.Join(w.dir, s.Prompt)); err != nil {
			return nil, fmt.Errorf("step %s: %w", s.ID, err)
		}
	}
	return &w, nil
}

// Client returns the client workflows (every workflow except the reviewer and
// the final review writer), sorted by ID.
func (c *Catalog) Client() []*Workflow {
	var out []*Workflow
	for _, w := range c.Workflows {
		if w.Role == "" && w.ID != "final-review" {
			out = append(out, w)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (w *Workflow) Step(id string) (Step, error) {
	for _, s := range w.Steps {
		if s.ID == id {
			return s, nil
		}
	}
	var ids []string
	for _, s := range w.Steps {
		ids = append(ids, s.ID)
	}
	return Step{}, fmt.Errorf("workflow %s has no step %q (steps: %s)", w.ID, id, strings.Join(ids, ", "))
}

// Spec builds the sub-agent spec for one step. The system prompt is the
// workflow prompt, the step prompt, and the shared trust model.
func (w *Workflow) Spec(stepID string) (subagent.Spec, error) {
	s, err := w.Step(stepID)
	if err != nil {
		return subagent.Spec{}, err
	}
	role, err := os.ReadFile(filepath.Join(w.dir, "prompt.md"))
	if err != nil {
		return subagent.Spec{}, err
	}
	step, err := os.ReadFile(filepath.Join(w.dir, s.Prompt))
	if err != nil {
		return subagent.Spec{}, err
	}
	tools := s.Tools
	if tools == nil {
		tools = w.Tools
	}
	return subagent.Spec{
		Workflow: w.ID, Step: s.ID, Agent: w.Agent,
		Model: w.Model, Effort: w.Effort,
		System:   strings.TrimSpace(string(role)) + "\n\n" + strings.TrimSpace(string(step)) + "\n\n" + strings.TrimSpace(w.trust),
		Tools:    tools,
		Output:   s.Output,
		MaxTurns: w.MaxTurns, MaxTokens: w.MaxTokens,
	}, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
