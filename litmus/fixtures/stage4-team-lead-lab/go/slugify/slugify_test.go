package slugify

import "testing"

func TestSlugifyNormalizesAndCollapsesSeparators(t *testing.T) {
	got, err := Slugify("  Stage 4: Audit_READY -- Now  ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "stage-4-audit-ready-now" {
		t.Fatalf("Slugify() = %q", got)
	}
}

func TestSlugifyRejectsEmptyAndTruncatesOnWordBoundary(t *testing.T) {
	if _, err := Slugify("!!!"); err == nil {
		t.Fatal("Slugify() accepted an empty normalized value")
	}
	got, err := Slugify("this is a deliberately long workflow identifier that should be shortened safely")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) > 48 || got[len(got)-1] == '-' {
		t.Fatalf("Slugify() returned invalid bounded slug %q", got)
	}
}
