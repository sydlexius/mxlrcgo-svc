package web

import (
	"testing"
	"time"
)

// fixedClock is a manually advanced clock for deterministic lockout tests.
type fixedClock struct{ t time.Time }

func (c *fixedClock) Now() time.Time          { return c.t }
func (c *fixedClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestLimiter(clk *fixedClock) *loginLimiter {
	l := newLoginLimiter()
	l.now = clk.Now
	return l
}

// TestLimiterBackoffProgression checks the documented backoff schedule
// (0, 1s, 2s, 4s, 8s, capped at 8s) for consecutive failures from one IP.
func TestLimiterBackoffProgression(t *testing.T) {
	clk := &fixedClock{t: time.Unix(0, 0)}
	l := newTestLimiter(clk)
	const ip = "192.0.2.1"

	want := []time.Duration{
		0,
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		8 * time.Second, // capped
	}
	for i, w := range want {
		if ok, _ := l.allow(ip); !ok {
			t.Fatalf("failure %d: allow() = false before lockout threshold", i+1)
		}
		got := l.fail(ip)
		if got != w {
			t.Errorf("failure %d: backoff = %v, want %v", i+1, got, w)
		}
	}
}

// TestLimiterHardLockout verifies that after maxFailures consecutive failures the
// IP is refused (allow=false) with a positive Retry-After until the cool-down
// elapses, then is allowed again.
func TestLimiterHardLockout(t *testing.T) {
	clk := &fixedClock{t: time.Unix(0, 0)}
	l := newTestLimiter(clk)
	const ip = "192.0.2.2"

	for i := 0; i < defaultMaxFailures; i++ {
		if ok, _ := l.allow(ip); !ok {
			t.Fatalf("attempt %d unexpectedly locked before threshold", i+1)
		}
		l.fail(ip)
	}

	ok, retryAfter := l.allow(ip)
	if ok {
		t.Fatalf("allow() = true after %d failures, want hard lockout", defaultMaxFailures)
	}
	if retryAfter <= 0 || retryAfter > defaultLoginLockout {
		t.Errorf("retryAfter = %v, want within (0, %v]", retryAfter, defaultLoginLockout)
	}

	// Still locked partway through the window.
	clk.advance(defaultLoginLockout - time.Second)
	if ok, _ := l.allow(ip); ok {
		t.Error("allow() = true before lockout window elapsed")
	}

	// Window elapsed: state resets and the IP is allowed afresh.
	clk.advance(2 * time.Second)
	if ok, _ := l.allow(ip); !ok {
		t.Error("allow() = false after lockout window elapsed, want reset")
	}
	if got := l.fail(ip); got != 0 {
		t.Errorf("post-reset first failure backoff = %v, want 0 (counter did not reset)", got)
	}
}

// TestLimiterSuccessResets confirms a successful login clears the IP's counter so
// the next failure starts the backoff schedule from zero.
func TestLimiterSuccessResets(t *testing.T) {
	clk := &fixedClock{t: time.Unix(0, 0)}
	l := newTestLimiter(clk)
	const ip = "192.0.2.3"

	l.fail(ip)
	l.fail(ip) // backoff now escalated
	l.success(ip)

	if got := l.fail(ip); got != 0 {
		t.Errorf("after success, first failure backoff = %v, want 0", got)
	}
}

// TestLimiterPerIPIsolation verifies failures from one IP never affect another.
func TestLimiterPerIPIsolation(t *testing.T) {
	clk := &fixedClock{t: time.Unix(0, 0)}
	l := newTestLimiter(clk)

	for i := 0; i < defaultMaxFailures; i++ {
		l.fail("192.0.2.10")
	}
	if ok, _ := l.allow("192.0.2.10"); ok {
		t.Fatal("attacker IP not locked out")
	}
	if ok, _ := l.allow("192.0.2.11"); !ok {
		t.Error("a different IP was locked out by the attacker's failures")
	}
}
