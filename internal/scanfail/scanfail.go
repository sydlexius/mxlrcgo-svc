// Package scanfail persists files that consistently fail audio metadata read so
// the scanner can skip re-reading (and re-warning about) malformed files until
// they change on disk. See issue #376.
package scanfail

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Store records and queries metadata-read failures in the
// scanner_metadata_failures table. It is safe for concurrent use because the
// underlying *sql.DB is.
type Store struct {
	db *sql.DB
}

// New returns a Store backed by db.
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// ShouldSkip reports whether path previously failed metadata read at the same
// mtime and size, meaning a re-read would fail identically and can be skipped.
// mtimeNano is ModTime().UnixNano() so a same-second rewrite to the same size
// still reads as changed.
func (s *Store) ShouldSkip(ctx context.Context, path string, mtimeNano, size int64) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM scanner_metadata_failures WHERE file_path=? AND mtime_nsec=? AND size_bytes=? LIMIT 1`,
		path, mtimeNano, size,
	).Scan(&one)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, fmt.Errorf("scanfail: lookup %q: %w", path, err)
}

// RecordFailure remembers that path failed metadata read at the given mtime and
// size, so subsequent scans skip it until the file changes. mtimeNano is
// ModTime().UnixNano().
func (s *Store) RecordFailure(ctx context.Context, path string, mtimeNano, size int64, readErr error) error {
	var errText string
	if readErr != nil {
		errText = readErr.Error()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO scanner_metadata_failures (file_path, mtime_nsec, size_bytes, error_text)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(file_path) DO UPDATE SET
		     mtime_nsec = excluded.mtime_nsec,
		     size_bytes = excluded.size_bytes,
		     error_text = excluded.error_text`,
		path, mtimeNano, size, errText,
	)
	if err != nil {
		return fmt.Errorf("scanfail: record %q: %w", path, err)
	}
	return nil
}
