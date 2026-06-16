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
const (
	defaultBaseBackoff  = 1 * time.Second
	defaultMaxBackoff   = 8 * time.Second
	defaultMaxFailures  = 10
	defaultLoginLockout = 15 * time.Minute
)

// attemptState tracks consecutive failed logins for one client IP.
type attemptState struct {
	failures    int
	lockedUntil time.Time // zero when not locked
}

// loginLimiter is a thread-safe, in-memory per-IP failed-login tracker that
// implements exponential backoff plus a hard lockout. It is safe for concurrent
// use by multiple request goroutines.
type loginLimiter struct {
	mu          sync.Mutex
	attempts    map[string]*attemptState
	baseBackoff time.Duration
	maxBackoff  time.Duration
	maxFailures int
	lockout     time.Duration
	now         func() time.Time
}

// newLoginLimiter builds a limiter with the package defaults.
func newLoginLimiter() *loginLimiter {
	return &loginLimiter{
		attempts:    make(map[string]*attemptState),
		baseBackoff: defaultBaseBackoff,
		maxBackoff:  defaultMaxBackoff,
		maxFailures: defaultMaxFailures,
		lockout:     defaultLoginLockout,
		now:         time.Now,
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
func (l *loginLimiter) fail(ip string) (backoff time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	st := l.attempts[ip]
	if st == nil {
		st = &attemptState{}
		l.attempts[ip] = st
	}
	st.failures++
	if st.failures >= l.maxFailures {
		st.lockedUntil = l.now().Add(l.lockout)
	}
	return l.backoffFor(st.failures)
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
