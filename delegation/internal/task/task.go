// Package task loads a change-review task: the post-change repository, the
// patch, the acceptance criteria, and the one test command run_tests may run.
package task

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Criterion struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type Task struct {
	ID                 string      `json:"id"`
	Title              string      `json:"title"`
	Description        string      `json:"description"`
	AcceptanceCriteria []Criterion `json:"acceptance_criteria"`
	TestCommand        []string    `json:"test_command"` // optional; run_tests is unavailable without it
	Repo               string      `json:"repo"`         // relative to the task file
	Patch              string      `json:"patch"`        // relative to the task file

	RepoDir   string `json:"-"`
	PatchText string `json:"-"`
}

// Load reads <dir>/task.json and resolves the repository and patch.
func Load(dir string) (*Task, error) {
	data, err := os.ReadFile(filepath.Join(dir, "task.json"))
	if err != nil {
		return nil, err
	}
	var t Task
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("task.json: %w", err)
	}
	switch {
	case t.ID == "" || t.Title == "" || t.Description == "":
		return nil, fmt.Errorf("task.json: id, title, and description are required")
	case len(t.AcceptanceCriteria) == 0:
		return nil, fmt.Errorf("task.json: no acceptance criteria")
	}
	ids := map[string]bool{}
	for _, c := range t.AcceptanceCriteria {
		if c.ID == "" || ids[c.ID] {
			return nil, fmt.Errorf("task.json: criterion ID %q is empty or duplicated", c.ID)
		}
		ids[c.ID] = true
	}
	if t.RepoDir, err = filepath.Abs(filepath.Join(dir, t.Repo)); err != nil {
		return nil, err
	}
	if info, err := os.Stat(t.RepoDir); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("task.json: repo %s is not a directory", t.Repo)
	}
	patch, err := os.ReadFile(filepath.Join(dir, t.Patch))
	if err != nil {
		return nil, fmt.Errorf("task.json: patch: %w", err)
	}
	t.PatchText = string(patch)
	return &t, nil
}

func (t *Task) CriteriaIDs() []string {
	var ids []string
	for _, c := range t.AcceptanceCriteria {
		ids = append(ids, c.ID)
	}
	return ids
}

// Brief renders the task for a sub-agent's first message.
func (t *Task) Brief() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Task %s: %s\n\n%s\n\n## Acceptance criteria\n", t.ID, t.Title, t.Description)
	for _, c := range t.AcceptanceCriteria {
		fmt.Fprintf(&b, "- %s: %s\n", c.ID, c.Text)
	}
	if len(t.TestCommand) > 0 {
		fmt.Fprintf(&b, "\n## Test command\n%s\n", strings.Join(t.TestCommand, " "))
	} else {
		b.WriteString("\n## Test command\nNone. This task has no runnable tests.\n")
	}
	fmt.Fprintf(&b, "\n## Patch\n```diff\n%s```\n", t.PatchText)
	return b.String()
}
