package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

//go:embed migrations/*.sql
var migrations embed.FS

// Open opens (or creates) the SQLite database at path, applies pragmas,
// and runs any pending goose migrations. Returns a ready-to-use *sql.DB.
// The caller must close the returned DB when done.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	if path == "" {
		return nil, fmt.Errorf("db: path must not be empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return nil, fmt.Errorf("db: create data dir: %w", err)
	}

	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("db: open %s: %w", path, err)
	}

	// Limit to one connection so per-connection PRAGMAs are reliable.
	sqlDB.SetMaxOpenConns(1)

	// PRAGMA journal_mode returns the mode actually applied, not just acknowledged,
	// so read it back and warn if WAL was not enabled (e.g. on a read-only FS).
	var journalMode string
	if err := sqlDB.QueryRowContext(ctx, "PRAGMA journal_mode=WAL").Scan(&journalMode); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("db: set WAL mode: %w", err)
	}
	if journalMode != "wal" {
		slog.Warn("db: WAL mode not enabled; running in fallback mode", "actual_mode", journalMode)
	}

	// Apply remaining pragmas. These do not require read-back verification;
	// any application failure surfaces as a non-nil error from ExecContext.
	pragmas := []string{
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
	}
	for _, p := range pragmas {
		if _, err := sqlDB.ExecContext(ctx, p); err != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("db: pragma %q: %w", p, err)
		}
	}

	// Run embedded migrations via goose NewProvider (thread-safe, non-global API).
	// fs.Sub roots the FS at the migrations/ subdirectory so goose can find *.sql at root.
	migFS, err := fs.Sub(migrations, "migrations")
	if err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("db: sub migrations fs: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, sqlDB, migFS)
	if err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("db: migration provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("db: migrate: %w", err)
	}

	return sqlDB, nil
}
