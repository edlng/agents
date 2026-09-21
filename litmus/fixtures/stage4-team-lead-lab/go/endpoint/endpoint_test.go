package endpoint

import "testing"

func TestValidateAcceptsOnlyExactHTTPSHosts(t *testing.T) {
	got, err := Validate("https://api.example.com/v1?q=ok", []string{"api.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Hostname() != "api.example.com" || got.Path != "/v1" {
		t.Fatalf("Validate() = %#v", got)
	}
}

func TestValidateRejectsSSRFAndCredentialShapes(t *testing.T) {
	for _, raw := range []string{
		"http://api.example.com/v1",
		"https://api.example.com.evil.test/v1",
		"https://user:pass@api.example.com/v1",
		"https://127.0.0.1/v1",
		"https://[::1]/v1",
		"//api.example.com/v1",
	} {
		if _, err := Validate(raw, []string{"api.example.com"}); err == nil {
			t.Fatalf("Validate(%q) unexpectedly succeeded", raw)
		}
	}
}
