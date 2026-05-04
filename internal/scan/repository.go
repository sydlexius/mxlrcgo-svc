package scan

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/sydlexius/mxlrcgo-svc/internal/models"
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
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("scan: begin upsert tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, res := range results {
		insertStatus := res.Status
		if insertStatus == "" {
			insertStatus = StatusPending
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO scan_results (library_id, file_path, artist, title, outdir, filename, status)
             VALUES (?, ?, ?, ?, ?, ?, ?)
             ON CONFLICT(library_id, file_path) DO UPDATE SET
                 artist = excluded.artist,
                 title = excluded.title,
                 outdir = excluded.outdir,
                 filename = excluded.filename`,
			libraryID,
			res.FilePath,
			res.Track.ArtistName,
			res.Track.TrackName,
			res.Outdir,
			res.Filename,
			insertStatus,
		)
		if err != nil {
			return fmt.Errorf("scan: upsert %s: %w", res.FilePath, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("scan: commit upsert tx: %w", err)
	}
	return nil
}

// ListByLibrary returns persisted scan results for a library in stable ID order.
func (r *Repo) ListByLibrary(ctx context.Context, libraryID int64) (results []models.ScanResult, retErr error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, library_id, file_path, artist, title, outdir, filename, status, created_at
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

	results, err = scanResultRows(rows)
	if err != nil {
		return nil, fmt.Errorf("scan: list rows: %w", err)
	}
	return results, nil
}

// ListPendingByLibrary returns pending scan results for a library in stable ID order.
func (r *Repo) ListPendingByLibrary(ctx context.Context, libraryID int64) (results []models.ScanResult, retErr error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, library_id, file_path, artist, title, outdir, filename, status, created_at
         FROM scan_results
         WHERE library_id = ?
           AND status = ?
         ORDER BY id ASC`,
		libraryID,
		StatusPending,
	)
	if err != nil {
		return nil, fmt.Errorf("scan: list pending by library: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil && retErr == nil {
			retErr = fmt.Errorf("scan: close pending rows: %w", err)
		}
	}()

	results, err = scanResultRows(rows)
	if err != nil {
		return nil, fmt.Errorf("scan: list pending rows: %w", err)
	}
	return results, nil
}

func scanResultRows(rows *sql.Rows) ([]models.ScanResult, error) {
	var results []models.ScanResult
	for rows.Next() {
		var res models.ScanResult
		if err := rows.Scan(
			&res.ID,
			&res.LibraryID,
			&res.FilePath,
			&res.Track.ArtistName,
			&res.Track.TrackName,
			&res.Outdir,
			&res.Filename,
			&res.Status,
			&res.CreatedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, res)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

// SetStatus updates scan result status for each id.
func (r *Repo) SetStatus(ctx context.Context, ids []int64, status string) error {
	if len(ids) == 0 {
		return nil
	}
	switch status {
	case StatusPending, StatusProcessing, StatusDone, StatusFailed:
	default:
		return fmt.Errorf("scan: unsupported status %q", status)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("scan: begin set status tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, id := range ids {
		res, err := tx.ExecContext(ctx,
			`UPDATE scan_results SET status = ? WHERE id = ?`,
			status,
			id,
		)
		if err != nil {
			return fmt.Errorf("scan: set status %d: %w", id, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("scan: set status rows affected: %w", err)
		}
		if n == 0 {
			return sql.ErrNoRows
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("scan: commit set status tx: %w", err)
	}
	return nil
}
