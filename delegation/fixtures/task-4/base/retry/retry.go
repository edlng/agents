package retry

import "time"

// Delay returns how long to wait before retry attempt n.
func Delay(n int, base, max time.Duration) time.Duration {
	return base
}
