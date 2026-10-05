package main

import (
	"os/exec"
	"strings"
	"testing"
)

// The graded runtime must not be able to reach the CLI-backed dev provider.
func TestRuntimeCannotReachCLIProvider(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, dep := range strings.Fields(string(out)) {
		if strings.Contains(dep, "/delegation/evals") {
			t.Fatalf("delegate depends on %s", dep)
		}
	}
}
