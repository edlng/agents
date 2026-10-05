package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/edlng/agents/litmus-eval/delegation/internal/schema"
	"github.com/edlng/agents/litmus-eval/delegation/internal/subagent"
)

// lineOf returns the 1-based line of the first occurrence of needle in a
// fixture file, so checks follow the fixture if it changes.
func lineOf(repo, file, needle string) int {
	f, err := os.Open(filepath.Join(repo, file))
	if err != nil {
		panic(err)
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for n := 1; s.Scan(); n++ {
		if strings.Contains(s.Text(), needle) {
			return n
		}
	}
	panic(fmt.Sprintf("%q not found in %s", needle, file))
}

func near(got, want, tol int) bool { return got >= want-tol && got <= want+tol }

func review(res subagent.Result) (schema.Review, bool) {
	var r schema.Review
	if !res.OK || json.Unmarshal(res.Artifact, &r) != nil {
		return r, false
	}
	return r, true
}

// blocksAt reports whether a valid review BLOCKs with a critical or major
// finding at file:line (within two lines).
func blocksAt(res subagent.Result, file string, line int) bool {
	r, ok := review(res)
	if !ok || r.Verdict != "BLOCK" {
		return false
	}
	for _, f := range r.Findings {
		if f.File == file && near(f.Line, line, 2) && (f.Severity == "critical" || f.Severity == "major") {
			return true
		}
	}
	return false
}

func approvesClean(res subagent.Result) bool {
	r, ok := review(res)
	return ok && r.Verdict == "APPROVE"
}

func criteria(res subagent.Result) (map[string]string, string, bool) {
	var c schema.Criteria
	if !res.OK || json.Unmarshal(res.Artifact, &c) != nil {
		return nil, "", false
	}
	m := map[string]string{}
	for _, cr := range c.Criteria {
		m[cr.ID] = cr.Status
	}
	return m, c.Verdict, true
}

// treeHash hashes every file under dir, to prove an agent changed nothing.
func treeHash(dir string) string {
	h := sha256.New()
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		fmt.Fprintf(h, "%s\x00%x\x00", rel, sha256.Sum256(data))
		return nil
	})
	return hex.EncodeToString(h.Sum(nil))
}

var (
	goDecl    = regexp.MustCompile(`(?m)^(?:func (?:\([^)]*\) )?|type )([A-Za-z_]\w*)`)
	backticks = regexp.MustCompile("`([^`\n]+)`")
	qualified = regexp.MustCompile(`\b([a-z]\w*)\.([A-Z]\w*)\b`)
)

// inventedAPIs lists identifiers the doc attributes to the repository's own
// packages (pkg.Name inside backticks or code blocks) that the code does not
// declare.
func inventedAPIs(repo, doc string) []string {
	declared := map[string]bool{}
	pkgs := map[string]bool{}
	filepath.WalkDir(repo, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && p != repo {
			pkgs[d.Name()] = true
		}
		if strings.HasSuffix(p, ".go") {
			data, _ := os.ReadFile(p)
			for _, m := range goDecl.FindAllStringSubmatch(string(data), -1) {
				declared[m[1]] = true
			}
		}
		return nil
	})
	var spans []string
	for _, m := range backticks.FindAllStringSubmatch(doc, -1) {
		spans = append(spans, m[1])
	}
	for _, block := range strings.Split(doc, "```")[1:] {
		spans = append(spans, block)
	}
	seen := map[string]bool{}
	var invented []string
	for _, s := range spans {
		for _, m := range qualified.FindAllStringSubmatch(s, -1) {
			id := m[1] + "." + m[2]
			if pkgs[m[1]] && !declared[m[2]] && !seen[id] {
				seen[id] = true
				invented = append(invented, id)
			}
		}
	}
	return invented
}

func errorText(res subagent.Result) string {
	if res.Error == nil {
		return ""
	}
	return res.Error.Code + ": " + res.Error.Message + " " + strings.Join(res.Error.Details, "; ")
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
