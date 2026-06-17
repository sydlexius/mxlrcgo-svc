package web

import (
	"fmt"
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

// TestLimiterIdleEviction verifies that idle entries are swept when a new IP
// would push the map past maxEntries, and that an IP with recent activity
// survives the sweep.
func TestLimiterIdleEviction(t *testing.T) {
	clk := &fixedClock{t: time.Unix(0, 0)}
	l := newTestLimiter(clk)
	l.maxEntries = 3
	l.idleEvictAfter = time.Hour

	// Fill the map to capacity.
	for i := range 3 {
		l.fail(fmt.Sprintf("10.0.0.%d", i))
	}

	// Advance past the idle window; refresh only the third IP.
	clk.advance(90 * time.Minute)
	l.fail("10.0.0.2") // updates lastSeen; still within idleEvictAfter from now

	// A fourth IP triggers eviction; the two stale entries should be removed.
	l.fail("10.1.0.1")

	l.mu.Lock()
	size := len(l.attempts)
	_, ip0ok := l.attempts["10.0.0.0"]
	_, ip1ok := l.attempts["10.0.0.1"]
	_, ip2ok := l.attempts["10.0.0.2"]
	_, newok := l.attempts["10.1.0.1"]
	l.mu.Unlock()

	if size > l.maxEntries {
		t.Errorf("map size = %d after eviction, want <= %d", size, l.maxEntries)
	}
	if ip0ok || ip1ok {
		t.Error("idle IPs were not evicted")
	}
	if !ip2ok {
		t.Error("recently active IP was evicted")
	}
	if !newok {
		t.Error("newly added IP is missing from the map")
	}
}

// TestLimiterHardCapWhenAllActive verifies the hard-cap fallback: when no entry
// qualifies as idle, the oldest entry by lastSeen is dropped to enforce maxEntries.
func TestLimiterHardCapWhenAllActive(t *testing.T) {
	clk := &fixedClock{t: time.Unix(0, 0)}
	l := newTestLimiter(clk)
	l.maxEntries = 2
	l.idleEvictAfter = time.Hour

	l.fail("10.0.0.1")
	l.fail("10.0.0.2")

	// Both entries are within idleEvictAfter (no time advance).
	// Adding a third IP must still respect the hard cap.
	l.fail("10.0.0.3")

	l.mu.Lock()
	size := len(l.attempts)
	l.mu.Unlock()

	if size > l.maxEntries {
		t.Errorf("map size = %d, want <= %d (hard cap not enforced)", size, l.maxEntries)
	}
}

// TestLimiterSaturatedAllLockedRefusesNewIPs verifies the memory hard-bound:
// when the map is at capacity and every entry holds an active lockout (so
// neither idle nor oldest-entry eviction can free space), new IPs are refused
// without being stored and maxBackoff is returned. len(attempts) must never
// exceed maxEntries. Already-tracked IPs still update their state normally.
func TestLimiterSaturatedAllLockedRefusesNewIPs(t *testing.T) {
	clk := &fixedClock{t: time.Unix(0, 0)}
	l := newTestLimiter(clk)
	l.maxEntries = 3
	l.maxFailures = 2
	l.idleEvictAfter = time.Hour

	// Fill the map with hard-locked entries.
	lockedIPs := make([]string, l.maxEntries)
	for i := range l.maxEntries {
		ip := fmt.Sprintf("10.0.0.%d", i)
		lockedIPs[i] = ip
		for range l.maxFailures {
			l.fail(ip)
		}
		if ok, _ := l.allow(ip); ok {
			t.Fatalf("setup: expected %s to be hard-locked", ip)
		}
	}

	l.mu.Lock()
	if got := len(l.attempts); got != l.maxEntries {
		l.mu.Unlock()
		t.Fatalf("setup: map size = %d, want %d", got, l.maxEntries)
	}
	l.mu.Unlock()

	// New IPs must be refused without being stored; fail() returns maxBackoff.
	for i := range 5 {
		newIP := fmt.Sprintf("10.1.0.%d", i)
		backoff := l.fail(newIP)

		l.mu.Lock()
		size := len(l.attempts)
		_, stored := l.attempts[newIP]
		l.mu.Unlock()

		if size > l.maxEntries {
			t.Errorf("new IP %d: map size %d exceeds maxEntries %d", i, size, l.maxEntries)
		}
		if stored {
			t.Errorf("new IP %s was stored despite saturated map", newIP)
		}
		if backoff != l.maxBackoff {
			t.Errorf("refused IP %s: backoff = %v, want maxBackoff %v", newIP, backoff, l.maxBackoff)
		}
	}

	// An already-tracked IP still has its state updated normally even during
	// saturation (it is in the map, so the refusal branch is not reached).
	const knownIP = "10.0.0.0"
	l.fail(knownIP)
	l.mu.Lock()
	st, present := l.attempts[knownIP]
	failures := 0
	if st != nil {
		failures = st.failures
	}
	l.mu.Unlock()

	if !present {
		t.Error("tracked IP removed from map during saturation")
	}
	if failures <= l.maxFailures {
		t.Errorf("tracked IP failures = %d, want > %d after extra fail()", failures, l.maxFailures)
	}
}

// TestLimiterLockedOutEntryPreserved verifies that an IP with an unexpired hard
// lockout is never evicted by the idle sweep, even when the map is full.
func TestLimiterLockedOutEntryPreserved(t *testing.T) {
	clk := &fixedClock{t: time.Unix(0, 0)}
	l := newTestLimiter(clk)
	l.maxEntries = 2
	l.idleEvictAfter = time.Minute

	const lockedIP = "192.0.2.100"

	// Lock out lockedIP.
	for range defaultMaxFailures {
		l.fail(lockedIP)
	}
	if ok, _ := l.allow(lockedIP); ok {
		t.Fatal("expected lockedIP to be hard-locked")
	}

	// Advance well past idleEvictAfter so lockedIP would normally be swept -
	// but its lockout window has not expired yet.
	clk.advance(10 * time.Minute)

	// A second IP fills the remaining slot; a third forces eviction.
	l.fail("192.0.2.101")
	l.fail("192.0.2.102") // triggers eviction

	l.mu.Lock()
	_, lockedPresent := l.attempts[lockedIP]
	l.mu.Unlock()

	if !lockedPresent {
		t.Error("actively locked-out IP was evicted; want it preserved")
	}
}
