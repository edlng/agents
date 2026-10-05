package task

import (
	"strings"
	"testing"
)

func TestLoadFixture(t *testing.T) {
	tk, err := Load("../../fixtures/endpoint-allowlist")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(tk.CriteriaIDs(), ",") != "AC1,AC2,AC3,AC4,AC5" || tk.TestCommand[0] != "go" {
		t.Fatalf("task = %+v", tk)
	}
	brief := tk.Brief()
	for _, want := range []string{"AC2: The hostname must match", "go test ./...", "+\t\tif strings.HasSuffix(host"} {
		if !strings.Contains(brief, want) {
			t.Errorf("brief missing %q", want)
		}
	}
}
