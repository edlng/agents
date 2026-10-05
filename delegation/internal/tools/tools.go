// Package tools implements the sub-agent tools. Every tool is rooted: paths are
// relative to the client repository, and a path that resolves outside it is
// refused. Tools that would change state work on copies under the run
// directory, never on the client repository.
package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/edlng/agents/litmus-eval/delegation/internal/provider"
)

const (
	maxReadBytes   = 200_000
	maxListEntries = 500
	maxMatches     = 200
	maxTestOutput  = 8_000
	maxDocBytes    = 64_000
	testTimeout    = 2 * time.Minute
)

// Env is what one sub-agent launch may touch.
type Env struct {
	Repo        string   // client repository, read-only
	TestCommand []string // the only command run_tests may execute
	DocsOut     string   // where write_doc puts files (inside the run directory)
}

// TestRun is the harness's own record of a run_tests call. Validators compare
// it with what the sub-agent reports.
type TestRun struct {
	Command  []string `json:"command"`
	ExitCode int      `json:"exit_code"`
	Output   string   `json:"output"`
}

// Set is the tool set for one launch. It records test runs and written docs.
type Set struct {
	env     Env
	tools   map[string]tool
	order   []string
	mu      sync.Mutex
	runs    []TestRun
	written []string
}

type tool struct {
	def provider.Tool
	run func(ctx context.Context, input json.RawMessage) (string, error)
}

// Build returns a set holding exactly the named tools.
func Build(names []string, env Env) (*Set, error) {
	repo, err := filepath.EvalSymlinks(env.Repo)
	if err != nil {
		return nil, fmt.Errorf("repo root: %w", err)
	}
	env.Repo = repo
	s := &Set{env: env, tools: map[string]tool{}}
	all := map[string]tool{
		"read_file":  {def: readFileDef, run: s.readFile},
		"list_files": {def: listFilesDef, run: s.listFiles},
		"search":     {def: searchDef, run: s.search},
		"run_tests":  {def: runTestsDef, run: s.runTests},
		"write_doc":  {def: writeDocDef, run: s.writeDoc},
	}
	for _, name := range names {
		t, ok := all[name]
		if !ok {
			return nil, fmt.Errorf("unknown tool %q", name)
		}
		if name == "run_tests" && len(env.TestCommand) == 0 {
			return nil, fmt.Errorf("run_tests needs a test command")
		}
		if name == "write_doc" && env.DocsOut == "" {
			return nil, fmt.Errorf("write_doc needs an output directory")
		}
		s.tools[name] = t
		s.order = append(s.order, name)
	}
	return s, nil
}

// Defs returns the tool definitions in manifest order.
func (s *Set) Defs() []provider.Tool {
	var defs []provider.Tool
	for _, name := range s.order {
		defs = append(defs, s.tools[name].def)
	}
	return defs
}

func (s *Set) Has(name string) bool { _, ok := s.tools[name]; return ok }

// Call runs a tool. The error is shown to the model as an is_error result.
func (s *Set) Call(ctx context.Context, name string, input json.RawMessage) (string, error) {
	t, ok := s.tools[name]
	if !ok {
		return "", fmt.Errorf("tool %q is not available to this agent", name)
	}
	return t.run(ctx, input)
}

func (s *Set) TestRuns() []TestRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]TestRun(nil), s.runs...)
}

func (s *Set) Written() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.written...)
}

// Resolve maps a repository-relative path to an absolute path inside the repo.
func Resolve(repo, rel string) (string, error) {
	if rel == "" {
		rel = "."
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("path %q must be relative to the repository", rel)
	}
	repo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		return "", fmt.Errorf("repository root: %w", stripPath(err))
	}
	clean := filepath.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q is outside the repository", rel)
	}
	abs := filepath.Join(repo, clean)
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("path %q: %w", rel, stripPath(err))
	}
	if real != repo && !strings.HasPrefix(real, repo+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q is outside the repository", rel)
	}
	return real, nil
}

// stripPath drops the absolute path from filesystem errors shown to the model.
func stripPath(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}

func skipDir(name string) bool {
	return name == ".git" || name == "node_modules" || name == "__pycache__"
}

var readFileDef = provider.Tool{
	Name:        "read_file",
	Description: "Read a text file from the repository. Lines are numbered. Optionally pass start_line and end_line (1-based, inclusive).",
	InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"start_line":{"type":"integer"},"end_line":{"type":"integer"}},"required":["path"]}`),
}

func (s *Set) readFile(_ context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Path      string `json:"path"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("input: %w", err)
	}
	abs, err := Resolve(s.env.Repo, in.Path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", in.Path, stripPath(err))
	}
	if len(data) > maxReadBytes {
		return "", fmt.Errorf("%s is %d bytes; read a line range", in.Path, len(data))
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	start, end := 1, len(lines)
	if in.StartLine > 0 {
		start = in.StartLine
	}
	if in.EndLine > 0 && in.EndLine < end {
		end = in.EndLine
	}
	if start > len(lines) {
		return "", fmt.Errorf("%s has %d lines", in.Path, len(lines))
	}
	var b strings.Builder
	for i := start; i <= end; i++ {
		fmt.Fprintf(&b, "%d\t%s\n", i, lines[i-1])
	}
	return b.String(), nil
}

var listFilesDef = provider.Tool{
	Name:        "list_files",
	Description: "List files under a repository directory, recursively. path defaults to the repository root.",
	InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
}

func (s *Set) listFiles(_ context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("input: %w", err)
	}
	abs, err := Resolve(s.env.Repo, in.Path)
	if err != nil {
		return "", err
	}
	var files []string
	err = filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && skipDir(d.Name()) {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(s.env.Repo, p)
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("list %s: %w", in.Path, stripPath(err))
	}
	sort.Strings(files)
	if len(files) > maxListEntries {
		return strings.Join(files[:maxListEntries], "\n") + fmt.Sprintf("\n... %d more", len(files)-maxListEntries), nil
	}
	return strings.Join(files, "\n"), nil
}

var searchDef = provider.Tool{
	Name:        "search",
	Description: "Search repository files for a Go regular expression. Returns path:line: text for each match. path limits the search to a directory or file.",
	InputSchema: json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string"},"path":{"type":"string"}},"required":["pattern"]}`),
}

func (s *Set) search(_ context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("input: %w", err)
	}
	re, err := regexp.Compile(in.Pattern)
	if err != nil {
		return "", fmt.Errorf("pattern: %w", err)
	}
	abs, err := Resolve(s.env.Repo, in.Path)
	if err != nil {
		return "", err
	}
	var matches []string
	err = filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if len(matches) >= maxMatches {
			return fs.SkipAll
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		rel, _ := filepath.Rel(s.env.Repo, p)
		scanner := bufio.NewScanner(f)
		for n := 1; scanner.Scan() && len(matches) < maxMatches; n++ {
			if re.Match(scanner.Bytes()) {
				matches = append(matches, fmt.Sprintf("%s:%d: %s", filepath.ToSlash(rel), n, scanner.Text()))
			}
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("search: %w", stripPath(err))
	}
	if len(matches) == 0 {
		return "no matches", nil
	}
	return strings.Join(matches, "\n"), nil
}

var runTestsDef = provider.Tool{
	Name:        "run_tests",
	Description: "Run the task's test command on a scratch copy of the repository. Takes no arguments; the command is fixed by the task specification. Returns the exit code and the tail of the output.",
	InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
}

func (s *Set) runTests(ctx context.Context, _ json.RawMessage) (string, error) {
	scratch, err := os.MkdirTemp("", "delegate-tests-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(scratch)
	work := filepath.Join(scratch, "repo")
	if err := copyTree(s.env.Repo, work); err != nil {
		return "", fmt.Errorf("copy repository: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, testTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.env.TestCommand[0], s.env.TestCommand[1:]...)
	cmd.Dir = work
	home := filepath.Join(scratch, "home")
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"TMPDIR=" + scratch,
		"GOCACHE=" + filepath.Join(scratch, "gocache"),
		"GOPATH=" + filepath.Join(scratch, "gopath"),
		"GOFLAGS=-mod=mod",
		"GOTOOLCHAIN=local",
	}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	runErr := cmd.Run()
	exit := 0
	if runErr != nil {
		var ee *exec.ExitError
		if !errors.As(runErr, &ee) {
			return "", fmt.Errorf("run tests: %w", runErr)
		}
		exit = ee.ExitCode()
	}
	text := out.String()
	if len(text) > maxTestOutput {
		text = "...\n" + text[len(text)-maxTestOutput:]
	}
	s.mu.Lock()
	s.runs = append(s.runs, TestRun{Command: s.env.TestCommand, ExitCode: exit, Output: text})
	s.mu.Unlock()
	return fmt.Sprintf("command: %s\nexit_code: %d\n%s", strings.Join(s.env.TestCommand, " "), exit, text), nil
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}

var writeDocDef = provider.Tool{
	Name:        "write_doc",
	Description: "Write a Markdown documentation file. path must start with docs/ and end with .md. Files go to the review output, not the client repository.",
	InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"]}`),
}

func (s *Set) writeDoc(_ context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("input: %w", err)
	}
	clean := filepath.ToSlash(filepath.Clean(in.Path))
	if !strings.HasPrefix(clean, "docs/") || !strings.HasSuffix(clean, ".md") || strings.Contains(clean, "..") {
		return "", fmt.Errorf("path %q must be docs/<name>.md", in.Path)
	}
	if len(in.Content) > maxDocBytes {
		return "", fmt.Errorf("content is %d bytes, limit %d", len(in.Content), maxDocBytes)
	}
	target := filepath.Join(s.env.DocsOut, filepath.FromSlash(clean))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(target, []byte(in.Content), 0o644); err != nil {
		return "", err
	}
	s.mu.Lock()
	if !contains(s.written, clean) {
		s.written = append(s.written, clean)
	}
	s.mu.Unlock()
	return fmt.Sprintf("wrote %s (%d bytes)", clean, len(in.Content)), nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
