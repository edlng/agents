// Package corpus turns links to real pull-request review rounds into tasks.
// The repository stores only a small manifest per case: the PR link, pinned
// base and reviewed commits, the diff hash, and the defects human reviewers
// flagged, each linked to its review comment. Code is fetched on demand into
// a cache that is removed after each case, so at most one checkout exists.
package corpus

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// MaxRepoKB refuses to fetch repositories larger than this (GitHub's size).
const MaxRepoKB = 50_000

type Defect struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Severity string `json:"severity"` // critical | major (minor review comments are not labels)
	Summary  string `json:"summary"`
	Source   string `json:"source"` // link to the review comment
}

// Dispute records a defect the system reported on human-approved code that
// the human reviewer could not confirm or reject. A case with disputes is
// left out of verdict scoring.
type Dispute struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Summary string `json:"summary"`
	Ruling  string `json:"ruling"` // unsure
	RuledBy string `json:"ruled_by"`
	Date    string `json:"date"`
}

type Criterion struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type Case struct {
	ID string `json:"id"`
	// Split is "tune" for cases used while changing prompts and "heldout"
	// for cases labeled before any results were seen and never tuned against.
	Split              string      `json:"split"`
	PR                 string      `json:"pr"`   // https://github.com/owner/repo/pull/N
	Repo               string      `json:"repo"` // owner/repo
	BaseSHA            string      `json:"base_sha"`
	HeadSHA            string      `json:"head_sha"` // the commit the reviewers saw
	DiffSHA256         string      `json:"diff_sha256"`
	Title              string      `json:"title"`
	Description        string      `json:"description"`
	AcceptanceCriteria []Criterion `json:"acceptance_criteria"`
	TestCommand        []string    `json:"test_command,omitempty"`
	Expected           struct {
		Verdict  string    `json:"verdict"` // BLOCK | APPROVE
		Defects  []Defect  `json:"defects"`
		Disputed []Dispute `json:"disputed,omitempty"`
	} `json:"expected"`
	LabelNotes string `json:"label_notes,omitempty"`
}

func Load(dir string) ([]Case, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var cases []Case
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		var c Case
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&c); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		if c.ID+".json" != filepath.Base(p) {
			return nil, fmt.Errorf("%s: id %q does not match the file name", p, c.ID)
		}
		if c.Split != "tune" && c.Split != "heldout" {
			return nil, fmt.Errorf("%s: split must be tune or heldout", p)
		}
		if c.Expected.Verdict != "BLOCK" && c.Expected.Verdict != "APPROVE" {
			return nil, fmt.Errorf("%s: expected verdict must be BLOCK or APPROVE", p)
		}
		if (c.Expected.Verdict == "BLOCK") != (len(c.Expected.Defects) > 0) {
			return nil, fmt.Errorf("%s: BLOCK needs labeled defects and APPROVE allows none", p)
		}
		cases = append(cases, c)
	}
	return cases, nil
}

// CacheRoot is this process's own cache directory, so concurrent runs never
// share or delete each other's checkouts.
func CacheRoot() string {
	return filepath.Join("delegation", "runs", "corpus-cache", fmt.Sprintf("pid-%d", os.Getpid()))
}

// Materialize fetches the two pinned commits into root/<id>, checks the diff
// against the manifest hash, and writes a task directory. cleanup removes
// everything it created.
func Materialize(c Case, root string) (taskDir string, cleanup func(), err error) {
	dir := filepath.Join(root, c.ID)
	cleanup = func() { os.RemoveAll(dir) }
	if err := checkSize(c.Repo); err != nil {
		return "", cleanup, err
	}
	gitDir := filepath.Join(dir, "git")
	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		return "", cleanup, err
	}
	steps := [][]string{
		{"init", "-q", "--bare", gitDir},
		{"--git-dir", gitDir, "fetch", "-q", "--depth", "1", "--no-tags", "https://github.com/" + c.Repo, c.BaseSHA, c.HeadSHA},
		{"--git-dir", gitDir, "--work-tree", repo, "checkout", "-q", c.HeadSHA, "--", "."},
	}
	for _, args := range steps {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			return "", cleanup, fmt.Errorf("git %s: %v: %s", args[len(args)-1], err, strings.TrimSpace(string(out)))
		}
	}
	diff, err := exec.Command("git", "--git-dir", gitDir, "diff", c.BaseSHA, c.HeadSHA).Output()
	if err != nil {
		return "", cleanup, fmt.Errorf("git diff: %w", err)
	}
	sum := sha256.Sum256(diff)
	if got := hex.EncodeToString(sum[:]); c.DiffSHA256 != "" && got != c.DiffSHA256 {
		return "", cleanup, fmt.Errorf("%s: diff hash %s does not match the manifest %s", c.ID, got, c.DiffSHA256)
	}
	os.RemoveAll(gitDir) // the checkout and patch are all a case needs
	if err := os.WriteFile(filepath.Join(dir, "change.patch"), diff, 0o644); err != nil {
		return "", cleanup, err
	}
	task := map[string]any{
		"id": c.ID, "title": c.Title,
		"description":         c.Description + "\n\nSource: " + c.PR,
		"acceptance_criteria": c.AcceptanceCriteria,
		"test_command":        c.TestCommand,
		"repo":                "repo", "patch": "change.patch",
	}
	b, _ := json.MarshalIndent(task, "", "  ")
	return dir, cleanup, os.WriteFile(filepath.Join(dir, "task.json"), b, 0o644)
}

// DiffHash fetches a case and returns its diff hash, for writing manifests.
func DiffHash(c Case, root string) (string, error) {
	c.DiffSHA256 = ""
	dir, cleanup, err := Materialize(c, root)
	defer cleanup()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(dir, "change.patch"))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func checkSize(repo string) error {
	out, err := exec.Command("gh", "api", "repos/"+repo, "-q", ".size").Output()
	if err != nil {
		return fmt.Errorf("repo size for %s: %w", repo, err)
	}
	var kb int
	fmt.Sscan(strings.TrimSpace(string(out)), &kb)
	if kb == 0 || kb > MaxRepoKB {
		return fmt.Errorf("%s is %d KB; the corpus fetches only repositories under %d KB", repo, kb, MaxRepoKB)
	}
	return nil
}
