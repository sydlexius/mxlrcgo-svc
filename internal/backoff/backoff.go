// Package backoff provides shared retry-delay formulas used by the worker,
// the durable queue, and the legacy fetch loop. Keeping the formula in one
// place ensures all three surfaces present the same cadence to operators
// (1m, 2m, 4m, ..., capped at 1h with the defaults).
package backoff

import "time"

// Default base and cap durations for geometric backoff. Callers may override
// these via Geometric's parameters when they need a different cadence.
const (
	DefaultBase = time.Minute
	DefaultMax  = time.Hour
)

// Geometric returns the wait duration for the given 1-indexed attempt count,
// doubling from base each step and capping at max. Returns 0 if base or max
// is non-positive. Attempts < 1 are treated as 1.
func Geometric(attempts int, base, max time.Duration) time.Duration {
	if base <= 0 || max <= 0 {
		return 0
	}
	if attempts < 1 {
		attempts = 1
	}
	delay := base
	for i := 1; i < attempts; i++ {
		if delay >= max || delay > max/2 {
			return max
		}
		delay *= 2
	}
	if delay > max {
		return max
	}
	return delay
}
