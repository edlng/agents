package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repo(t *testing.T) string {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "src", "a.go"), []byte("package a\n\nfunc A() int { return 1 }\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# demo\n"), 0o644)
	return dir
}

func call(t *testing.T, s *Set, name, input string) (string, error) {
	t.Helper()
	return s.Call(context.Background(), name, json.RawMessage(input))
}

func TestOnlyDeclaredToolsExist(t *testing.T) {
	s, err := Build([]string{"read_file"}, Env{Repo: repo(t)})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Defs()) != 1 || s.Defs()[0].Name != "read_file" {
		t.Fatalf("defs = %v", s.Defs())
	}
	if _, err := call(t, s, "list_files", `{}`); err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("undeclared tool ran: %v", err)
	}
	if _, err := Build([]string{"bash"}, Env{Repo: repo(t)}); err == nil {
		t.Fatal("unknown tool accepted")
	}
}

func TestReadFileStaysInRepo(t *testing.T) {
	dir := repo(t)
	outside := filepath.Join(t.TempDir(), "secret.txt")
	os.WriteFile(outside, []byte("secret"), 0o644)
	os.Symlink(outside, filepath.Join(dir, "link.txt"))
	s, _ := Build([]string{"read_file"}, Env{Repo: dir})

	got, err := call(t, s, "read_file", `{"path":"src/a.go","start_line":3,"end_line":3}`)
	if err != nil || got != "3\tfunc A() int { return 1 }\n" {
		t.Fatalf("read = %q, %v", got, err)
	}
	for _, path := range []string{"../secret.txt", outside, "link.txt", "src/../../x"} {
		if out, err := call(t, s, "read_file", `{"path":`+quote(path)+`}`); err == nil {
			t.Errorf("read %s succeeded: %q", path, out)
		} else if strings.Contains(err.Error(), dir) {
			t.Errorf("error leaks absolute path: %v", err)
		}
	}
}

func TestListAndSearch(t *testing.T) {
	s, _ := Build([]string{"list_files", "search"}, Env{Repo: repo(t)})
	got, _ := call(t, s, "list_files", `{}`)
	if got != "README.md\nsrc/a.go" {
		t.Fatalf("list = %q", got)
	}
	got, _ = call(t, s, "search", `{"pattern":"func \\w+\\("}`)
	if got != "src/a.go:3: func A() int { return 1 }" {
		t.Fatalf("search = %q", got)
	}
}

func TestRunTestsUsesFixedCommandOnACopy(t *testing.T) {
	dir := repo(t)
	cmd := []string{"sh", "-c", "touch created; echo boom; exit 3"}
	s, _ := Build([]string{"run_tests"}, Env{Repo: dir, TestCommand: cmd})
	got, err := call(t, s, "run_tests", `{"command":"rm -rf /"}`)
	if err != nil || !strings.Contains(got, "exit_code: 3") || !strings.Contains(got, "boom") {
		t.Fatalf("run = %q, %v", got, err)
	}
	if runs := s.TestRuns(); len(runs) != 1 || runs[0].ExitCode != 3 {
		t.Fatalf("runs = %+v", runs)
	}
	if _, err := os.Stat(filepath.Join(dir, "created")); err == nil {
		t.Fatal("test command wrote into the client repository")
	}
}

// With a run scratch directory, each call shares its cache and removes its
// repository copy, so only the cache stays until the run ends.
func TestRunTestsSharesRunScratch(t *testing.T) {
	scratch := t.TempDir()
	cmd := []string{"sh", "-c", `mkdir -p "$GOCACHE"; touch "$GOCACHE/$$"`}
	s, _ := Build([]string{"run_tests"}, Env{Repo: repo(t), TestCommand: cmd, Scratch: scratch})
	for i := 0; i < 2; i++ {
		if got, err := call(t, s, "run_tests", `{}`); err != nil || !strings.Contains(got, "exit_code: 0") {
			t.Fatalf("run %d = %q, %v", i, got, err)
		}
	}
	entries, _ := os.ReadDir(scratch)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != "gocache" {
		t.Fatalf("scratch holds %v, want only gocache", names)
	}
	if cached, _ := os.ReadDir(filepath.Join(scratch, "gocache")); len(cached) != 2 {
		t.Fatalf("gocache holds %d entries, want 2 (one per run)", len(cached))
	}
}

func TestWriteDocOnlyUnderDocs(t *testing.T) {
	out := t.TempDir()
	s, _ := Build([]string{"write_doc"}, Env{Repo: repo(t), DocsOut: out})
	if _, err := call(t, s, "write_doc", `{"path":"docs/guide.md","content":"# Guide"}`); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(out, "docs", "guide.md")); string(data) != "# Guide" {
		t.Fatalf("doc = %q", data)
	}
	for _, path := range []string{"src/a.go", "docs/../x.md", "docs/x.sh", "/docs/x.md"} {
		if _, err := call(t, s, "write_doc", `{"path":`+quote(path)+`,"content":"x"}`); err == nil {
			t.Errorf("write %s succeeded", path)
		}
	}
	if w := s.Written(); len(w) != 1 || w[0] != "docs/guide.md" {
		t.Fatalf("written = %v", w)
	}
}

func quote(s string) string { b, _ := json.Marshal(s); return string(b) }
