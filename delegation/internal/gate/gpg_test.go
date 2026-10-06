package gate

import "testing"

func TestStatusField(t *testing.T) {
	status := "[GNUPG:] BEGIN_SIGNING H10\n[GNUPG:] SIG_CREATED D 22 10 00 1791301590 32B5AFC15DDCFCD20B98F1F904DEEFF6DBB20F74\n"
	if got := statusField(status, "SIG_CREATED", 5); got != "32B5AFC15DDCFCD20B98F1F904DEEFF6DBB20F74" {
		t.Fatalf("SIG_CREATED fingerprint = %q", got)
	}
	verify := "[GNUPG:] GOODSIG 04DEEFF6DBB20F74 Edward\n[GNUPG:] VALIDSIG 32B5AFC15DDCFCD20B98F1F904DEEFF6DBB20F74 2026-10-05 1791301590\n"
	if got := statusField(verify, "VALIDSIG", 0); got != "32B5AFC15DDCFCD20B98F1F904DEEFF6DBB20F74" {
		t.Fatalf("VALIDSIG fingerprint = %q", got)
	}
}
