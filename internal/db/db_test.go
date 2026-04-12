package db

import (
	"context"
	"path/filepath"
	"testing"
)

// TestOpen_CreatesDatabaseAndAppliesMigrations verifies that Open succeeds,
// returns a usable *sql.DB, and has run the initial migrations (all expected
// tables exist).
func TestOpen_CreatesDatabaseAndAppliesMigrations(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.db")

	sqlDB, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer sqlDB.Close() //nolint:errcheck

	// Verify expected tables were created by the migration.
	tables := []string{"libraries", "scan_results", "lyrics_cache", "work_queue", "api_keys"}
	for _, tbl := range tables {
		var count int
		row := sqlDB.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", tbl)
		if err := row.Scan(&count); err != nil {
			t.Errorf("query table %q: %v", tbl, err)
			continue
		}
		if count != 1 {
			t.Errorf("table %q not found after migration", tbl)
		}
	}
}

// TestOpen_WALModeEnabled verifies that the journal_mode PRAGMA was applied.
func TestOpen_WALModeEnabled(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "wal.db")

	sqlDB, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer sqlDB.Close() //nolint:errcheck

	var mode string
	if err := sqlDB.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q; want %q", mode, "wal")
	}
}

// TestOpen_ForeignKeysEnabled verifies that PRAGMA foreign_keys=ON was applied.
func TestOpen_ForeignKeysEnabled(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "fk.db")

	sqlDB, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer sqlDB.Close() //nolint:errcheck

	var enabled int
	if err := sqlDB.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&enabled); err != nil {
		t.Fatalf("query foreign_keys: %v", err)
	}
	if enabled != 1 {
		t.Errorf("foreign_keys = %d; want 1 (ON)", enabled)
	}
}

// TestOpen_EmptyPathReturnsError verifies that an empty path is rejected.
func TestOpen_EmptyPathReturnsError(t *testing.T) {
	ctx := context.Background()
	_, err := Open(ctx, "")
	if err == nil {
		t.Fatal("Open(\"\") returned nil error; want an error")
	}
}

// TestOpen_IdempotentMigrations verifies that opening the same DB a second time
// does not fail (goose Up is idempotent).
func TestOpen_IdempotentMigrations(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "idempotent.db")

	db1, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	db1.Close() //nolint:errcheck

	db2, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("second Open (idempotency check): %v", err)
	}
	db2.Close() //nolint:errcheck
}
