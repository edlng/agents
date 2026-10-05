package coordinator

import (
	"os/exec"
	"strings"
	"testing"
)

// The coordinator must not be able to reach anything that does real work.
func TestCoordinatorCannotReachWorkTools(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatal(err)
	}
	forbidden := []string{"os/exec", "net/http", "net", "/delegation/internal/tools", "/delegation/internal/subagent",
		"/delegation/internal/dispatch", "/delegation/internal/provider/anthropic", "/delegation/evals"}
	for _, dep := range strings.Fields(string(out)) {
		for _, f := range forbidden {
			if dep == f || (strings.HasPrefix(f, "/") && strings.HasSuffix(dep, f)) {
				t.Errorf("coordinator depends on %s", dep)
			}
		}
	}
}

func TestShippedManifestIsDispatchOnly(t *testing.T) {
	m, err := LoadManifest("../../coordinator")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Tools) != 6 {
		t.Fatalf("tools = %v", m.Tools)
	}
	for _, d := range m.ToolDefs() {
		if !IsDispatchVerb(d.Name) {
			t.Errorf("%s is not a dispatch tool", d.Name)
		}
	}
	for name := range definitions {
		if !IsDispatchVerb(name) {
			t.Errorf("package defines non-dispatch tool %s", name)
		}
	}
}
