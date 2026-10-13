package archive

import "testing"

func TestEntryPathJoinsInsideRoot(t *testing.T) {
	got, err := EntryPath("/srv/exports", "reports/q3.csv")
	if err != nil || got != "/srv/exports/reports/q3.csv" {
		t.Fatalf("EntryPath() = %q, %v", got, err)
	}
}

func TestEntryPathRejectsEscapes(t *testing.T) {
	for _, name := range []string{"../etc/passwd", "a/../../etc/passwd", "/etc/passwd", ""} {
		if _, err := EntryPath("/srv/exports", name); err == nil {
			t.Errorf("EntryPath(%q) unexpectedly succeeded", name)
		}
	}
}
