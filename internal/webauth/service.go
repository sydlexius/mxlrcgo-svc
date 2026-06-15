package webauth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// DefaultSessionTTL is how long a new session stays valid (design default: 7 days).
const DefaultSessionTTL = 7 * 24 * time.Hour

// MinPasswordLength is the minimum admin password length enforced at the service
// boundary. The onboarding UI (a later lane) also checks length; enforcing it
// here keeps the core safe-by-default regardless of caller.
const MinPasswordLength = 8

var (
	// ErrInvalidCredentials is returned by Login for any failure (unknown user or
	// wrong password). It is deliberately identical for both so the caller cannot
	// distinguish them and enumerate usernames.
	ErrInvalidCredentials = errors.New("webauth: invalid credentials")
	// ErrInvalidSession is returned by ValidateSession for an unknown, expired, or
	// orphaned session token.
	ErrInvalidSession = errors.New("webauth: invalid session")
	// ErrPasswordTooShort is returned by Setup when the password is shorter than
	// MinPasswordLength.
	ErrPasswordTooShort = fmt.Errorf("webauth: password must be at least %d characters", MinPasswordLength)
)

// Service ties the user and session stores together into the browser-auth core:
// first-run setup, login, session validation, logout, and expiry cleanup. It is
// designed to be wrapped (e.g. by a per-IP rate limiter in a later lane); it does
// not implement lockout itself.
type Service struct {
	users      UserStore
	sessions   SessionStore
	sessionTTL time.Duration
	now        func() time.Time
}

// Option customizes a Service.
type Option func(*Service)

// WithSessionTTL overrides the session lifetime (default DefaultSessionTTL).
func WithSessionTTL(ttl time.Duration) Option {
	return func(s *Service) {
		if ttl > 0 {
			s.sessionTTL = ttl
		}
	}
}

// WithClock overrides the time source (for tests).
func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

// NewService returns a Service backed by the given stores.
func NewService(users UserStore, sessions SessionStore, opts ...Option) *Service {
	s := &Service{
		users:      users,
		sessions:   sessions,
		sessionTTL: DefaultSessionTTL,
		now:        time.Now,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Setup creates the first admin user. It rejects with ErrUserExists if any user
// already exists, so it can be called at most once. Atomicity is guaranteed by
// CreateFirstUser (a single conditional insert), not by the check below: the
// HasUsers call is only a fast path that avoids an expensive Argon2id hash when
// an admin already exists, and is safe to lose a race on.
func (s *Service) Setup(ctx context.Context, username, password string) (User, error) {
	if strings.TrimSpace(username) == "" {
		return User{}, fmt.Errorf("webauth: username must not be empty")
	}
	if len(password) < MinPasswordLength {
		return User{}, ErrPasswordTooShort
	}
	exists, err := s.users.HasUsers(ctx)
	if err != nil {
		return User{}, err
	}
	if exists {
		return User{}, ErrUserExists
	}
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, err
	}
	return s.users.CreateFirstUser(ctx, username, hash)
}

// Login verifies credentials and, on success, creates a session and returns its
// raw token. It is constant-time and enumeration-safe: an unknown username still
// runs an Argon2id verify against a dummy hash, and both the unknown-user and
// wrong-password paths return the identical ErrInvalidCredentials.
func (s *Service) Login(ctx context.Context, username, password string) (string, error) {
	user, ok, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		return "", err
	}
	if !ok {
		// Spend comparable time so a missing user is indistinguishable from a
		// wrong password by timing. The result is intentionally discarded.
		_, _ = VerifyPassword(dummyHash, password)
		return "", ErrInvalidCredentials
	}
	valid, err := VerifyPassword(user.PasswordHash, password)
	if err != nil {
		// A stored hash we cannot parse is a server-side defect, not a credential
		// problem; log it but do not leak detail to the caller.
		slog.Error("webauth: stored password hash is unparsable", "user_id", user.ID, "error", err)
		return "", ErrInvalidCredentials
	}
	if !valid {
		return "", ErrInvalidCredentials
	}
	expiresAt := s.now().Add(s.sessionTTL)
	token, err := s.sessions.CreateSession(ctx, user.ID, expiresAt)
	if err != nil {
		return "", err
	}
	return token, nil
}

// ValidateSession resolves a raw session token to its owning user. It returns
// ErrInvalidSession for an unknown, expired, or orphaned token.
func (s *Service) ValidateSession(ctx context.Context, rawToken string) (*User, error) {
	if rawToken == "" {
		return nil, ErrInvalidSession
	}
	sess, ok, err := s.sessions.GetSessionByToken(ctx, rawToken)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrInvalidSession
	}
	user, ok, err := s.users.GetByID(ctx, sess.UserID)
	if err != nil {
		return nil, err
	}
	if !ok {
		// Session references a user that no longer exists; treat as invalid.
		return nil, ErrInvalidSession
	}
	return &user, nil
}

// Logout revokes the session for rawToken by deleting it. Logging out an unknown
// token is a no-op (no error), so a double-logout is harmless.
func (s *Service) Logout(ctx context.Context, rawToken string) error {
	if rawToken == "" {
		return nil
	}
	return s.sessions.DeleteSession(ctx, rawToken)
}

// CleanExpiredSessions deletes expired sessions and returns the count removed.
// Intended to be called periodically by a background sweeper (a later lane).
func (s *Service) CleanExpiredSessions(ctx context.Context) (int64, error) {
	return s.sessions.CleanExpiredSessions(ctx)
}

// HasUsers reports whether an admin account exists (first-run detection).
func (s *Service) HasUsers(ctx context.Context) (bool, error) {
	return s.users.HasUsers(ctx)
}
