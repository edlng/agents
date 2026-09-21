package retrydelay

import (
	"testing"
	"time"
)

func TestDelayUsesOneBasedAttemptsAndSaturates(t *testing.T) {
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 100 * time.Millisecond},
		{2, 200 * time.Millisecond},
		{4, 800 * time.Millisecond},
		{8, time.Second},
	}
	for _, test := range tests {
		got, err := Delay(test.attempt, 100*time.Millisecond, time.Second)
		if err != nil {
			t.Fatalf("Delay(%d) error: %v", test.attempt, err)
		}
		if got != test.want {
			t.Fatalf("Delay(%d) = %v, want %v", test.attempt, got, test.want)
		}
	}
}

func TestDelayRejectsInvalidInputs(t *testing.T) {
	for _, test := range []struct {
		attempt int
		base    time.Duration
		max     time.Duration
	}{{0, time.Second, time.Second}, {1, 0, time.Second}, {1, time.Second, 0}, {1, 2 * time.Second, time.Second}} {
		if _, err := Delay(test.attempt, test.base, test.max); err == nil {
			t.Fatalf("Delay(%d, %v, %v) unexpectedly succeeded", test.attempt, test.base, test.max)
		}
	}
}
