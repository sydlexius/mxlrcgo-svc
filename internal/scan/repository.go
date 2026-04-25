package scan

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/sydlexius/mxlrcsvc-go/internal/models"
)

const (
	// StatusPending marks a discovered file that has not been queued yet.
	StatusPending = "pending"
	// StatusProcessing marks a discovered file currently being processed.
	StatusProcessing = "processing"
	// StatusDone marks a discovered file whose lyrics work completed.
	StatusDone = "done"
	// StatusFailed marks a discovered file whose lyrics work failed.
	StatusFailed = "failed"
)

// Repo provides persistence for library scan results.
type Repo struct {
	db *sql.DB
}

// New creates a scan result repository backed by db.
func New(db *sql.DB) *Repo {
	return &Repo{db: db}
}

// Upsert stores scan results for a library, keyed by library_id and file_path.
func (r *Repo) Upsert(ctx context.Context, libraryID int64, results []models.ScanResult) error {
	for _, res := range results {
		status := res.Status
		if status == "" {
			status = StatusPending
		}
		_, err := r.db.ExecContext(ctx,
			`INSERT INTO scan_results (library_id, file_path, artist, title, status)
             VALUES (?, ?, ?, ?, ?)
             ON CONFLICT(library_id, file_path) DO UPDATE SET
                 artist = excluded.artist,
                 title = excluded.title,
                 status = excluded.status`,
			libraryID,
			res.FilePath,
			res.Track.ArtistName,
			res.Track.TrackName,
			status,
		)
		if err != nil {
			return fmt.Errorf("scan: upsert %s: %w", res.FilePath, err)
		}
	}
	return nil
}

// ListByLibrary returns persisted scan results for a library in stable ID order.
func (r *Repo) ListByLibrary(ctx context.Context, libraryID int64) (results []models.ScanResult, retErr error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, library_id, file_path, artist, title, status, created_at
         FROM scan_results
         WHERE library_id = ?
         ORDER BY id ASC`,
		libraryID,
	)
	if err != nil {
		return nil, fmt.Errorf("scan: list by library: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil && retErr == nil {
			retErr = fmt.Errorf("scan: close list rows: %w", err)
		}
	}()

	for rows.Next() {
		var res models.ScanResult
		if err := rows.Scan(
			&res.ID,
			&res.LibraryID,
			&res.FilePath,
			&res.Track.ArtistName,
			&res.Track.TrackName,
			&res.Status,
			&res.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan: list scan: %w", err)
		}
		results = append(results, res)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scan: list rows: %w", err)
	}
	return results, nil
}
