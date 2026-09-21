package approval

import (
	"strings"
	"testing"
	"time"
)

func TestApprovalRoundTripAndBinding(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	token, err := Sign([]byte("0123456789abcdef0123456789abcdef"), "task-7", "alice", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	actor, err := Verify([]byte("0123456789abcdef0123456789abcdef"), token, "task-7", now)
	if err != nil || actor != "alice" {
		t.Fatalf("Verify() = %q, %v", actor, err)
	}
	if _, err := Verify([]byte("0123456789abcdef0123456789abcdef"), token, "task-8", now); err == nil {
		t.Fatal("Verify() accepted a token for another task")
	}
}

func TestApprovalRejectsExpiredTamperedAndWeakInputs(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	if _, err := Sign([]byte("short"), "task", "alice", now.Add(time.Minute)); err == nil {
		t.Fatal("Sign() accepted a weak secret")
	}
	token, err := Sign([]byte("0123456789abcdef0123456789abcdef"), "task", "alice", now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Verify([]byte("short"), token, "task", now); err == nil {
		t.Fatal("Verify() accepted a weak secret")
	}
	if _, err := Verify([]byte("0123456789abcdef0123456789abcdef"), token, "task", now.Add(2*time.Second)); err == nil {
		t.Fatal("Verify() accepted an expired token")
	}
	if _, err := Verify([]byte("0123456789abcdef0123456789abcdef"), token, "task", now.Add(time.Second)); err == nil {
		t.Fatal("Verify() accepted a token at its expiry instant")
	}
	replacement := "A"
	if strings.HasSuffix(token, replacement) {
		replacement = "B"
	}
	tampered := token[:len(token)-1] + replacement
	if _, err := Verify([]byte("0123456789abcdef0123456789abcdef"), tampered, "task", now); err == nil {
		t.Fatal("Verify() accepted a tampered token")
	}
}

func TestApprovalRoundTripsDelimiterCharacters(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	secret := []byte("0123456789abcdef0123456789abcdef")
	token, err := Sign(secret, "task|release", "alice|ops", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	actor, err := Verify(secret, token, "task|release", now)
	if err != nil || actor != "alice|ops" {
		t.Fatalf("Verify() = %q, %v", actor, err)
	}
}
