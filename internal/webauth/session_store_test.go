package webauth

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"
)

// newUserForSession creates a user row so session FKs resolve, returning its id.
func newUserForSession(t *testing.T, sqlDB *sql.DB) string {
	t.Helper()
	u, err := NewSQLUserStore(sqlDB).CreateUser(context.Background(), "admin", "$argon2id$hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return u.ID
}

func TestSessionStoreCreateStoresHashNotRawToken(t *testing.T) {
	ctx := context.Background()
	sqlDB := newTestDB(t)
	userID := newUserForSession(t, sqlDB)
	store := NewSQLSessionStore(sqlDB)

	raw, err := store.CreateSession(ctx, userID, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if raw == "" {
		t.Fatal("CreateSession returned an empty raw token")
	}

	// The DB must hold the SHA-256 hash, never the raw bearer token.
	var stored string
	if err := sqlDB.QueryRowContext(ctx, `SELECT token_hash FROM sessions`).Scan(&stored); err != nil {
		t.Fatalf("read token_hash: %v", err)
	}
	if stored == raw {
		t.Fatal("sessions table stored the RAW token, not its hash")
	}
	if stored != hashToken(raw) {
		t.Fatalf("stored token_hash = %q, want sha256 hex of raw token", stored)
	}
	if len(stored) != 64 {
		t.Fatalf("token_hash length = %d, want 64 hex chars", len(stored))
	}
	if strings.Contains(stored, raw) {
		t.Fatal("raw token leaked into the stored hash")
	}
}

func TestSessionStoreGetByToken(t *testing.T) {
	ctx := context.Background()
	sqlDB := newTestDB(t)
	userID := newUserForSession(t, sqlDB)
	store := NewSQLSessionStore(sqlDB)

	raw, err := store.CreateSession(ctx, userID, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	sess, ok, err := store.GetSessionByToken(ctx, raw)
	if err != nil {
		t.Fatalf("GetSessionByToken: %v", err)
	}
	if !ok {
		t.Fatal("GetSessionByToken found nothing for a valid token")
	}
	if sess.UserID != userID {
		t.Fatalf("session user id = %q, want %q", sess.UserID, userID)
	}
	if sess.TokenHash != hashToken(raw) {
		t.Fatal("session token hash mismatch")
	}

	// Unknown token resolves to ok=false, not an error.
	if _, ok, err := store.GetSessionByToken(ctx, "not-a-real-token"); err != nil || ok {
		t.Fatalf("GetSessionByToken(unknown) = (ok=%v, err=%v), want (false, nil)", ok, err)
	}
}

func TestSessionStoreExpired(t *testing.T) {
	ctx := context.Background()
	sqlDB := newTestDB(t)
	userID := newUserForSession(t, sqlDB)
	store := NewSQLSessionStore(sqlDB)

	// Expiry already in the past: GetSessionByToken must treat it as gone.
	raw, err := store.CreateSession(ctx, userID, time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if _, ok, err := store.GetSessionByToken(ctx, raw); err != nil || ok {
		t.Fatalf("GetSessionByToken(expired) = (ok=%v, err=%v), want (false, nil)", ok, err)
	}
}

func TestSessionStoreDelete(t *testing.T) {
	ctx := context.Background()
	sqlDB := newTestDB(t)
	userID := newUserForSession(t, sqlDB)
	store := NewSQLSessionStore(sqlDB)

	raw, err := store.CreateSession(ctx, userID, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := store.DeleteSession(ctx, raw); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, ok, err := store.GetSessionByToken(ctx, raw); err != nil || ok {
		t.Fatalf("session still present after delete (ok=%v, err=%v)", ok, err)
	}
	// Deleting an unknown token is a no-op, not an error.
	if err := store.DeleteSession(ctx, "unknown"); err != nil {
		t.Fatalf("DeleteSession(unknown): %v", err)
	}
}

func TestSessionStoreCleanExpired(t *testing.T) {
	ctx := context.Background()
	sqlDB := newTestDB(t)
	userID := newUserForSession(t, sqlDB)
	store := NewSQLSessionStore(sqlDB)

	validRaw, err := store.CreateSession(ctx, userID, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("CreateSession valid: %v", err)
	}
	if _, err := store.CreateSession(ctx, userID, time.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("CreateSession expired: %v", err)
	}

	removed, err := store.CleanExpiredSessions(ctx)
	if err != nil {
		t.Fatalf("CleanExpiredSessions: %v", err)
	}
	if removed != 1 {
		t.Fatalf("CleanExpiredSessions removed %d, want 1", removed)
	}

	// The valid session survives the sweep.
	if _, ok, err := store.GetSessionByToken(ctx, validRaw); err != nil || !ok {
		t.Fatalf("valid session removed by sweep (ok=%v, err=%v)", ok, err)
	}
	var count int
	if err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions`).Scan(&count); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if count != 1 {
		t.Fatalf("sessions remaining = %d, want 1", count)
	}
}

func TestSessionStoreTokensAreUnique(t *testing.T) {
	ctx := context.Background()
	sqlDB := newTestDB(t)
	userID := newUserForSession(t, sqlDB)
	store := NewSQLSessionStore(sqlDB)

	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		raw, err := store.CreateSession(ctx, userID, time.Now().Add(time.Hour))
		if err != nil {
			t.Fatalf("CreateSession #%d: %v", i, err)
		}
		if seen[raw] {
			t.Fatalf("duplicate raw token generated: %q", raw)
		}
		seen[raw] = true
	}
}
