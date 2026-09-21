package redactheaders

import (
	"reflect"
	"testing"
)

func TestRedactCopiesAndRedactsSensitiveHeadersCaseInsensitively(t *testing.T) {
	input := map[string][]string{
		"Authorization":       {"Bearer secret"},
		"proxy-authorization": {"Basic secret"},
		"x-API-key":           {"key"},
		"Cookie":              {"session=secret"},
		"Set-Cookie":          {"session=secret"},
		"Content-Type":        {"application/json"},
	}
	got := Redact(input)
	want := map[string][]string{
		"Authorization":       {"[REDACTED]"},
		"proxy-authorization": {"[REDACTED]"},
		"x-API-key":           {"[REDACTED]"},
		"Cookie":              {"[REDACTED]"},
		"Set-Cookie":          {"[REDACTED]"},
		"Content-Type":        {"application/json"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Redact() = %#v, want %#v", got, want)
	}
	got["Content-Type"][0] = "changed"
	if input["Content-Type"][0] != "application/json" {
		t.Fatal("Redact() aliased input slices")
	}
}

func TestRedactHandlesNil(t *testing.T) {
	if Redact(nil) != nil {
		t.Fatal("Redact(nil) should return nil")
	}
}
