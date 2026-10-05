package retry

import "time"

// Delay returns how long to wait before retry attempt n (1-based): base for
// the first retry, doubling each attempt, never more than max.
func Delay(n int, base, max time.Duration) time.Duration {
	if n < 1 {
		return 0
	}
	d := base << n
	if d > max || d <= 0 {
		return max
	}
	return d
}
