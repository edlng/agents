package retry

import (
	"testing"
	"time"
)

func TestDelayDoublesFromBase(t *testing.T) {
	base, max := 100*time.Millisecond, 5*time.Second
	for n, want := range map[int]time.Duration{1: 100 * time.Millisecond, 2: 200 * time.Millisecond, 3: 400 * time.Millisecond} {
		if got := Delay(n, base, max); got != want {
			t.Errorf("Delay(%d) = %v, want %v", n, got, want)
		}
	}
}

func TestDelayCapsAtMax(t *testing.T) {
	if got := Delay(20, 100*time.Millisecond, 5*time.Second); got != 5*time.Second {
		t.Errorf("Delay(20) = %v", got)
	}
}
