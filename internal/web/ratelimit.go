package web

import (
	"sync"
	"time"
)

// Login rate-limiting defaults (issue #204, lane 3). These throttle online
// password guessing without permanently locking out a legitimate admin:
//
//   - Consecutive failures from one IP incur an exponential backoff applied
//     before the response (0, 1s, 2s, 4s, 8s, capped at maxLoginBackoff).
//   - After maxLoginFailures consecutive failures the IP is hard-locked for
//     loginLockout, during which every attempt is refused with 429.
//   - A successful login clears that IP's counter.
//
// State is in-memory and keyed on the resolved client IP (trustnet.ClientIP),
// so it resets on restart - acceptable for a single-admin daemon - and composes
// with reverse-proxy handling rather than locking out the proxy's own address.
//
// Memory bounding (issue #260): entries idle for defaultIdleEvictAfter are
// swept when a new IP would exceed defaultMaxEntries, keeping heap growth
// bounded under a flood of rotating source IPs. Active hard-lockouts are never
// evicted; when idle eviction frees no space the oldest unlocked entry by
// lastSeen is dropped instead. If every entry holds an active lockout so no
// eviction is possible, new IPs are refused without being stored (maximum
// backoff returned) -- len(attempts) never exceeds maxEntries.
const (
	defaultBaseBackoff    = 1 * time.Second
	defaultMaxBackoff     = 8 * time.Second
	defaultMaxFailures    = 10
	defaultLoginLockout   = 15 * time.Minute
	defaultIdleEvictAfter = 24 * time.Hour
	defaultMaxEntries     = 10_000
)

// attemptState tracks consecutive failed logins for one client IP.
type attemptState struct {
	failures    int
	lockedUntil time.Time // zero when not locked
	lastSeen    time.Time // set on every fail(); used for idle eviction
}

// loginLimiter is a thread-safe, in-memory per-IP failed-login tracker that
// implements exponential backoff plus a hard lockout. It is safe for concurrent
// use by multiple request goroutines.
type loginLimiter struct {
	mu             sync.Mutex
	attempts       map[string]*attemptState
	baseBackoff    time.Duration
	maxBackoff     time.Duration
	maxFailures    int
	lockout        time.Duration
	idleEvictAfter time.Duration
	maxEntries     int
	now            func() time.Time
}

// newLoginLimiter builds a limiter with the package defaults.
func newLoginLimiter() *loginLimiter {
	return &loginLimiter{
		attempts:       make(map[string]*attemptState),
		baseBackoff:    defaultBaseBackoff,
		maxBackoff:     defaultMaxBackoff,
		maxFailures:    defaultMaxFailures,
		lockout:        defaultLoginLockout,
		idleEvictAfter: defaultIdleEvictAfter,
		maxEntries:     defaultMaxEntries,
		now:            time.Now,
	}
}

// allow reports whether an attempt from ip may proceed. When the IP is hard-
// locked it returns false plus the remaining cool-down; once the lockout window
// has elapsed the IP's state is reset and the attempt is allowed afresh.
func (l *loginLimiter) allow(ip string) (ok bool, retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	st := l.attempts[ip]
	if st == nil || st.lockedUntil.IsZero() {
		return true, 0
	}
	if remaining := st.lockedUntil.Sub(l.now()); remaining > 0 {
		return false, remaining
	}
	// Lockout expired: forget this IP so it starts clean.
	delete(l.attempts, ip)
	return true, 0
}

// fail records a failed attempt from ip and returns the backoff the caller must
// sleep before responding. When the failure count reaches maxFailures the IP is
// hard-locked for the cool-down window.
//
// When a new IP would exceed maxEntries, idle entries are swept first; if the
// map is still full after the sweep the oldest unlocked entry is dropped
// (hard cap). If all entries hold active lockouts and no eviction is possible,
// the new IP is refused without being stored and maxBackoff is returned,
// keeping len(attempts) <= maxEntries at all times.
func (l *loginLimiter) fail(ip string) (backoff time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	st := l.attempts[ip]
	if st == nil {
		if len(l.attempts) >= l.maxEntries {
			l.evictIdleLocked()
			if len(l.attempts) >= l.maxEntries {
				l.evictOldestLocked()
			}
			// All remaining entries hold active lockouts; refuse to store
			// state for this unknown IP to preserve the hard bound.
			if len(l.attempts) >= l.maxEntries {
				return l.maxBackoff
			}
		}
		st = &attemptState{}
		l.attempts[ip] = st
	}
	st.failures++
	st.lastSeen = l.now()
	if st.failures >= l.maxFailures {
		st.lockedUntil = l.now().Add(l.lockout)
	}
	return l.backoffFor(st.failures)
}

// evictIdleLocked removes entries idle for longer than idleEvictAfter. Entries
// with an unexpired hard lockout are preserved regardless of lastSeen. Must be
// called with l.mu held.
func (l *loginLimiter) evictIdleLocked() {
	now := l.now()
	cutoff := now.Add(-l.idleEvictAfter)
	for ip, st := range l.attempts {
		if !st.lockedUntil.IsZero() && st.lockedUntil.After(now) {
			continue // active lockout: preserve
		}
		if st.lastSeen.Before(cutoff) {
			delete(l.attempts, ip)
		}
	}
}

// evictOldestLocked removes the single non-locked entry with the oldest
// lastSeen as a hard-cap fallback when idle eviction cannot free space. Active
// hard-lockouts are skipped; if every entry has an unexpired lockout nothing is
// removed (fail() handles this case by refusing to store the new IP). Must be
// called with l.mu held.
func (l *loginLimiter) evictOldestLocked() {
	now := l.now()
	var oldestIP string
	var oldest time.Time
	for ip, st := range l.attempts {
		if !st.lockedUntil.IsZero() && st.lockedUntil.After(now) {
			continue // active lockout: preserve
		}
		if oldestIP == "" || st.lastSeen.Before(oldest) {
			oldestIP = ip
			oldest = st.lastSeen
		}
	}
	if oldestIP != "" {
		delete(l.attempts, oldestIP)
	}
}

// success clears any failure state for ip after a successful login.
func (l *loginLimiter) success(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, ip)
}

// backoffFor returns the delay for the nth consecutive failure: 0 for the first,
// then baseBackoff doubling each step, capped at maxBackoff. The shift is
// guarded so a large failure count cannot overflow into a negative duration.
func (l *loginLimiter) backoffFor(failures int) time.Duration {
	if failures <= 1 {
		return 0
	}
	shift := failures - 2
	if shift >= 62 { // 1s << 62 already overflows; clamp well before
		return l.maxBackoff
	}
	d := l.baseBackoff << uint(shift)
	if d <= 0 || d > l.maxBackoff {
		return l.maxBackoff
	}
	return d
}
