package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShippedCatalog(t *testing.T) {
	c, err := Load("../../workflows")
	if err != nil {
		t.Fatal(err)
	}
	var ids, substance []string
	for _, w := range c.Client() {
		ids = append(ids, w.ID)
		substance = append(substance, w.Substance)
	}
	if strings.Join(ids, ",") != "code-review,documentation,spec-validation" {
		t.Fatalf("client workflows = %v", ids)
	}
	if strings.Join(substance, ",") != "core,peripheral,core" {
		t.Fatalf("substance = %v", substance)
	}
	spec, err := c.Workflows["spec-validation"].Spec("tests")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(spec.Tools, ",") != "read_file,run_tests" || !strings.Contains(spec.System, "## Trust model") {
		t.Fatalf("spec = %+v", spec)
	}
	if _, err := c.Workflows["code-review"].Spec("style"); err == nil {
		t.Fatal("unknown step accepted")
	}
}

func TestRejectsStepToolOutsideWorkflow(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "trust.md"), []byte("trust"), 0o644)
	w := filepath.Join(dir, "demo")
	os.MkdirAll(w, 0o755)
	os.WriteFile(filepath.Join(w, "prompt.md"), []byte("p"), 0o644)
	os.WriteFile(filepath.Join(w, "s.md"), []byte("s"), 0o644)
	os.WriteFile(filepath.Join(w, "manifest.json"), []byte(`{"id":"demo","agent":"a","purpose":"p","substance":"core","substance_rationale":"r",
		"model":"m","effort":"low","max_turns":2,"max_tokens":100,"isolated_context":true,"tools":["read_file"],
		"steps":[{"id":"s","prompt":"s.md","output":"review.v1","tools":["read_file","run_tests"],"description":"d"}],
		"evaluation":{"criteria":["a","b","c"],"results":"r.json"}}`), 0o644)
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "uses run_tests, which the workflow does not declare") {
		t.Fatalf("err = %v", err)
	}
}
