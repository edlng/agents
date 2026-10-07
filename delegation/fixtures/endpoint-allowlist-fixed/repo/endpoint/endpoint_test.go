package endpoint

import "testing"

func TestValidateAcceptsAllowedHTTPSHost(t *testing.T) {
	got, err := Validate("https://API.example.com/v1?q=ok", []string{"api.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/v1" || got.RawQuery != "q=ok" {
		t.Fatalf("Validate() = %#v", got)
	}
}

func TestValidateRejectsUnsafeShapes(t *testing.T) {
	for _, raw := range []string{
		"http://api.example.com/v1",
		"https://user:pass@api.example.com/v1",
		"https://127.0.0.1/v1",
		"https://[::1]/v1",
		"//api.example.com/v1",
		"https://api.example.com.evil.test/v1",
		"https://evilapi.example.com/v1",
		"https://sub.api.example.com/v1",
		"https://\u212Aapi.example.com/v1",
	} {
		if _, err := Validate(raw, []string{"api.example.com", "kapi.example.com"}); err == nil {
			t.Errorf("Validate(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestValidateIgnoresEmptyAllowlistEntries(t *testing.T) {
	if _, err := Validate("https://api.example.com/v1", []string{""}); err == nil {
		t.Fatal("empty allowlist entry matched a host")
	}
}
