// Command corpushash fetches each corpus case once, writes its diff hash into
// the manifest, and checks that every labeled defect points at a real line of
// the reviewed commit. Each checkout is removed before the next fetch.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/edlng/agents/litmus-eval/delegation/evals/corpus"
)

func main() {
	cases, err := corpus.Load("delegation/corpus")
	if err != nil {
		fail(err)
	}
	cache := corpus.CacheRoot()
	for _, c := range cases {
		dir, cleanup, err := corpus.Materialize(withoutHash(c), cache)
		if err != nil {
			cleanup()
			fail(fmt.Errorf("%s: %w", c.ID, err))
		}
		for _, d := range c.Expected.Defects {
			data, err := os.ReadFile(filepath.Join(dir, "repo", d.File))
			if err != nil {
				cleanup()
				fail(fmt.Errorf("%s: %s: %w", c.ID, d.File, err))
			}
			if n := strings.Count(string(data), "\n") + 1; d.Line < 1 || d.Line > n {
				cleanup()
				fail(fmt.Errorf("%s: %s:%d is outside the file (%d lines)", c.ID, d.File, d.Line, n))
			}
		}
		patch, _ := os.ReadFile(filepath.Join(dir, "change.patch"))
		cleanup()
		hash, err := corpus.DiffHash(withoutHash(c), cache)
		if err != nil {
			fail(err)
		}
		path := filepath.Join("delegation/corpus", c.ID+".json")
		raw, _ := os.ReadFile(path)
		var m map[string]any
		json.Unmarshal(raw, &m)
		m["diff_sha256"] = hash
		out, _ := json.MarshalIndent(orderLike(raw, m), "", "  ")
		os.WriteFile(path, append(out, '\n'), 0o644)
		fmt.Printf("%-16s diff %6d bytes  %d labeled defects  %s\n", c.ID, len(patch), len(c.Expected.Defects), hash[:12])
	}
	os.Remove(cache)
}

func withoutHash(c corpus.Case) corpus.Case { c.DiffSHA256 = ""; return c }

// orderLike keeps the manifest's key order by re-decoding into the case type.
func orderLike(_ []byte, m map[string]any) any {
	b, _ := json.Marshal(m)
	var c corpus.Case
	json.Unmarshal(b, &c)
	return c
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
