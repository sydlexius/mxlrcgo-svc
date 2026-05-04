package queue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/sydlexius/mxlrcgo-svc/internal/backoff"
	"github.com/sydlexius/mxlrcgo-svc/internal/models"
	"github.com/sydlexius/mxlrcgo-svc/internal/normalize"
)

const (
	// StatusPending marks queued work ready to be processed.
	StatusPending = "pending"
	// StatusProcessing marks queued work currently being processed.
	StatusProcessing = "processing"
	// StatusDone marks queued work completed successfully.
	StatusDone = "done"
	// StatusFailed marks queued work that failed and may be retried after backoff.
	StatusFailed = "failed"
)

const timeFormat = time.RFC3339

// InputsQueue is a FIFO queue for processing work items.
type InputsQueue struct {
	Queue []models.Inputs
}

// NewInputsQueue creates an empty InputsQueue.
func NewInputsQueue() *InputsQueue {
	return &InputsQueue{}
}

// Next returns the front item without removing it, or an error if the queue is empty.
func (q *InputsQueue) Next() (models.Inputs, error) {
	if q.Empty() {
		return models.Inputs{}, fmt.Errorf("queue is empty")
	}
	return q.Queue[0], nil
}

// Pop removes and returns the front item, or an error if the queue is empty.
func (q *InputsQueue) Pop() (models.Inputs, error) {
	if q.Empty() {
		return models.Inputs{}, fmt.Errorf("queue is empty")
	}
	tmp := q.Queue[0]
	q.Queue[0] = models.Inputs{} // clear reference to avoid memory leak
	q.Queue = q.Queue[1:]
	return tmp, nil
}

// Push adds an item to the back of the queue.
func (q *InputsQueue) Push(i models.Inputs) {
	q.Queue = append(q.Queue, i)
}

// Len returns the number of items in the queue.
func (q *InputsQueue) Len() int {
	return len(q.Queue)
}

// Empty returns true if the queue has no items.
func (q *InputsQueue) Empty() bool {
	return len(q.Queue) == 0
}

// WorkItem represents a persisted queue row.
type WorkItem struct {
	ID            int64
	Inputs        models.Inputs
	Status        string
	Priority      int
	Attempts      int
	NextAttemptAt time.Time
	LastError     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	CompletedAt   *time.Time
}

// DBQueue is a SQLite-backed queue for durable lyrics work.
type DBQueue struct {
	db          *sql.DB
	baseBackoff time.Duration
	maxBackoff  time.Duration
	now         func() time.Time
}

// NewDBQueue returns a durable queue backed by db.
func NewDBQueue(db *sql.DB) *DBQueue {
	return &DBQueue{
		db:          db,
		baseBackoff: backoff.DefaultBase,
		maxBackoff:  backoff.DefaultMax,
		now:         time.Now,
	}
}

// Enqueue atomically inserts a new work item or refreshes an existing retryable
// item with the same normalized artist/title key.
func (q *DBQueue) Enqueue(ctx context.Context, inputs models.Inputs, priority int) (WorkItem, error) {
	now := formatTime(q.now())
	outputPaths, err := marshalOutputPaths(inputs)
	if err != nil {
		return WorkItem{}, err
	}
	row := q.db.QueryRowContext(ctx,
		`INSERT INTO work_queue (
             artist, title, artist_key, title_key, outdir, filename, source_path, output_paths, scan_result_id, status, priority, next_attempt_at
         )
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
         ON CONFLICT(artist_key, title_key) DO UPDATE SET
             artist = CASE
                 WHEN work_queue.status IN ('done', 'processing') THEN work_queue.artist
                 ELSE excluded.artist
             END,
             title = CASE
                 WHEN work_queue.status IN ('done', 'processing') THEN work_queue.title
                 ELSE excluded.title
             END,
             outdir = CASE
                 WHEN work_queue.status IN ('done', 'processing') THEN work_queue.outdir
                 ELSE excluded.outdir
             END,
             filename = CASE
                 WHEN work_queue.status IN ('done', 'processing') THEN work_queue.filename
                 ELSE excluded.filename
             END,
             source_path = CASE
                 WHEN work_queue.status IN ('done', 'processing') THEN work_queue.source_path
                 ELSE excluded.source_path
             END,
             output_paths = CASE
                 WHEN work_queue.status IN ('done', 'processing') THEN work_queue.output_paths
                 ELSE excluded.output_paths
             END,
             scan_result_id = COALESCE(work_queue.scan_result_id, excluded.scan_result_id),
             priority = max(work_queue.priority, excluded.priority),
             status = CASE
                 WHEN work_queue.status IN ('done', 'processing', 'failed') THEN work_queue.status
                 ELSE 'pending'
             END,
             next_attempt_at = CASE
                 WHEN work_queue.status IN ('done', 'processing', 'failed') THEN work_queue.next_attempt_at
                 ELSE excluded.next_attempt_at
             END,
             last_error = CASE
                 WHEN work_queue.status IN ('done', 'processing', 'failed') THEN work_queue.last_error
                 ELSE ''
             END,
             completed_at = CASE
                 WHEN work_queue.status = 'done' THEN work_queue.completed_at
                 ELSE NULL
             END
         RETURNING id, artist, title, outdir, filename, source_path, status, priority, attempts,
                   next_attempt_at, last_error, created_at, updated_at, completed_at, output_paths, scan_result_id`,
		inputs.Track.ArtistName,
		inputs.Track.TrackName,
		normalize.NormalizeKey(inputs.Track.ArtistName),
		normalize.NormalizeKey(inputs.Track.TrackName),
		inputs.Outdir,
		inputs.Filename,
		inputs.SourcePath,
		outputPaths,
		nullableID(inputs.ScanResultID),
		StatusPending,
		priority,
		now,
	)
	item, err := scanWorkItem(row)
	if err != nil {
		return WorkItem{}, fmt.Errorf("queue: enqueue: %w", err)
	}
	return item, nil
}

// Dequeue atomically claims the next ready item and marks it processing.
func (q *DBQueue) Dequeue(ctx context.Context) (WorkItem, error) {
	now := formatTime(q.now())
	row := q.db.QueryRowContext(ctx,
		`UPDATE work_queue
         SET status = 'processing'
         WHERE id = (
             SELECT id
             FROM work_queue
             WHERE status IN ('pending', 'failed')
               AND next_attempt_at <= ?
             ORDER BY priority DESC, created_at ASC, id ASC
             LIMIT 1
         )
         RETURNING id, artist, title, outdir, filename, source_path, status, priority, attempts,
                   next_attempt_at, last_error, created_at, updated_at, completed_at, output_paths, scan_result_id`,
		now,
	)
	item, err := scanWorkItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return WorkItem{}, sql.ErrNoRows
	}
	if err != nil {
		return WorkItem{}, fmt.Errorf("queue: dequeue: %w", err)
	}
	return item, nil
}

// Complete marks a processing item done. If the work_queue row carries a
// scan_result_id, the linked scan_results row is flipped to 'done' inside the
// same transaction, so a successful Complete guarantees both ledgers agree.
// Crash or partial-write between the two updates is impossible: SQLite either
// commits the whole transaction or rolls back.
func (q *DBQueue) Complete(ctx context.Context, id int64) error {
	now := formatTime(q.now())
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("queue: begin complete tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		`UPDATE work_queue
         SET status = 'done',
             completed_at = ?,
             last_error = ''
         WHERE id = ?
           AND status = 'processing'`,
		now,
		id,
	)
	if err != nil {
		return fmt.Errorf("queue: complete: %w", err)
	}
	if err := requireAffected(res, "queue: complete"); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE scan_results
         SET status = 'done'
         WHERE id = (SELECT scan_result_id FROM work_queue WHERE id = ?)
           AND status != 'done'`,
		id,
	); err != nil {
		return fmt.Errorf("queue: complete scan_results writeback: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("queue: commit complete tx: %w", err)
	}
	return nil
}

// Cleanup removes retryable queued work for the same normalized artist/title.
// Processing and completed rows are preserved to avoid racing active workers or
// losing history for work that has already finished.
func (q *DBQueue) Cleanup(ctx context.Context, inputs models.Inputs) (int64, error) {
	res, err := q.db.ExecContext(ctx,
		`DELETE FROM work_queue
         WHERE artist_key = ?
           AND title_key = ?
           AND status IN ('pending', 'failed')`,
		normalize.NormalizeKey(inputs.Track.ArtistName),
		normalize.NormalizeKey(inputs.Track.TrackName),
	)
	if err != nil {
		return 0, fmt.Errorf("queue: cleanup: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("queue: cleanup rows affected: %w", err)
	}
	return n, nil
}

// Fail records a failed attempt and schedules the item after geometric backoff.
func (q *DBQueue) Fail(ctx context.Context, id int64, cause error) (WorkItem, error) {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return WorkItem{}, fmt.Errorf("queue: begin fail tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var attempts int
	if err := tx.QueryRowContext(ctx,
		`SELECT attempts FROM work_queue WHERE id = ? AND status = 'processing'`,
		id,
	).Scan(&attempts); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return WorkItem{}, sql.ErrNoRows
		}
		return WorkItem{}, fmt.Errorf("queue: load attempts: %w", err)
	}

	nextAttempts := attempts + 1
	nextAttemptAt := formatTime(q.now().Add(backoff.Geometric(nextAttempts, q.baseBackoff, q.maxBackoff)))
	lastError := ""
	if cause != nil {
		lastError = cause.Error()
	}
	row := tx.QueryRowContext(ctx,
		`UPDATE work_queue
         SET status = 'failed',
             attempts = ?,
             next_attempt_at = ?,
             last_error = ?
         WHERE id = ?
           AND status = 'processing'
         RETURNING id, artist, title, outdir, filename, source_path, status, priority, attempts,
                   next_attempt_at, last_error, created_at, updated_at, completed_at, output_paths, scan_result_id`,
		nextAttempts,
		nextAttemptAt,
		lastError,
		id,
	)
	item, err := scanWorkItem(row)
	if err != nil {
		return WorkItem{}, fmt.Errorf("queue: fail: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return WorkItem{}, fmt.Errorf("queue: commit fail tx: %w", err)
	}
	return item, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanWorkItem(row rowScanner) (WorkItem, error) {
	var item WorkItem
	var nextAttemptAt, createdAt, updatedAt, outputPaths string
	var completedAt sql.NullString
	var scanResultID sql.NullInt64
	err := row.Scan(
		&item.ID,
		&item.Inputs.Track.ArtistName,
		&item.Inputs.Track.TrackName,
		&item.Inputs.Outdir,
		&item.Inputs.Filename,
		&item.Inputs.SourcePath,
		&item.Status,
		&item.Priority,
		&item.Attempts,
		&nextAttemptAt,
		&item.LastError,
		&createdAt,
		&updatedAt,
		&completedAt,
		&outputPaths,
		&scanResultID,
	)
	if err != nil {
		return WorkItem{}, err
	}
	if scanResultID.Valid {
		item.Inputs.ScanResultID = scanResultID.Int64
	}
	item.NextAttemptAt, err = parseTime(nextAttemptAt)
	if err != nil {
		return WorkItem{}, err
	}
	item.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return WorkItem{}, err
	}
	item.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return WorkItem{}, err
	}
	if completedAt.Valid {
		t, err := parseTime(completedAt.String)
		if err != nil {
			return WorkItem{}, err
		}
		item.CompletedAt = &t
	}
	item.Inputs.OutputPaths, err = unmarshalOutputPaths(outputPaths, item.Inputs.Outdir, item.Inputs.Filename)
	if err != nil {
		return WorkItem{}, err
	}
	return item, nil
}

func marshalOutputPaths(inputs models.Inputs) (string, error) {
	paths := inputs.OutputPaths
	if len(paths) == 0 {
		paths = []models.OutputPath{{
			Outdir:   inputs.Outdir,
			Filename: inputs.Filename,
		}}
	}
	b, err := json.Marshal(paths)
	if err != nil {
		return "", fmt.Errorf("queue: marshal output paths: %w", err)
	}
	return string(b), nil
}

func unmarshalOutputPaths(s string, outdir string, filename string) ([]models.OutputPath, error) {
	if s == "" {
		return []models.OutputPath{{
			Outdir:   outdir,
			Filename: filename,
		}}, nil
	}
	var paths []models.OutputPath
	if err := json.Unmarshal([]byte(s), &paths); err != nil {
		return nil, fmt.Errorf("queue: unmarshal output paths: %w", err)
	}
	if len(paths) == 0 {
		paths = append(paths, models.OutputPath{Outdir: outdir, Filename: filename})
	}
	return paths, nil
}

func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(timeFormat, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse time %q: %w", s, err)
	}
	return t, nil
}

func formatTime(t time.Time) string {
	return t.UTC().Format(timeFormat)
}

func nullableID(id int64) any {
	if id <= 0 {
		return nil
	}
	return id
}

func requireAffected(res sql.Result, op string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s rows affected: %w", op, err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
