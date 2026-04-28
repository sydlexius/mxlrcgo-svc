package queue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

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

const (
	defaultBaseBackoff = time.Minute
	defaultMaxBackoff  = time.Hour
	timeFormat         = time.RFC3339
)

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
		baseBackoff: defaultBaseBackoff,
		maxBackoff:  defaultMaxBackoff,
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
             artist, title, artist_key, title_key, outdir, filename, output_paths, status, priority, next_attempt_at
         )
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
             output_paths = CASE
                 WHEN work_queue.status IN ('done', 'processing') THEN work_queue.output_paths
                 ELSE excluded.output_paths
             END,
             priority = max(work_queue.priority, excluded.priority),
             status = CASE
                 WHEN work_queue.status IN ('done', 'processing') THEN work_queue.status
                 ELSE 'pending'
             END,
             next_attempt_at = CASE
                 WHEN work_queue.status IN ('done', 'processing') THEN work_queue.next_attempt_at
                 ELSE excluded.next_attempt_at
             END,
             last_error = CASE
                 WHEN work_queue.status IN ('done', 'processing') THEN work_queue.last_error
                 ELSE ''
             END,
             completed_at = CASE
                 WHEN work_queue.status = 'done' THEN work_queue.completed_at
                 ELSE NULL
             END
         RETURNING id, artist, title, outdir, filename, status, priority, attempts,
                   next_attempt_at, last_error, created_at, updated_at, completed_at, output_paths`,
		inputs.Track.ArtistName,
		inputs.Track.TrackName,
		normalize.NormalizeKey(inputs.Track.ArtistName),
		normalize.NormalizeKey(inputs.Track.TrackName),
		inputs.Outdir,
		inputs.Filename,
		outputPaths,
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
         RETURNING id, artist, title, outdir, filename, status, priority, attempts,
                   next_attempt_at, last_error, created_at, updated_at, completed_at, output_paths`,
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

// Complete marks a processing item done.
func (q *DBQueue) Complete(ctx context.Context, id int64) error {
	now := formatTime(q.now())
	res, err := q.db.ExecContext(ctx,
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
	return requireAffected(res, "queue: complete")
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
	nextAttemptAt := formatTime(q.now().Add(q.backoff(nextAttempts)))
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
         RETURNING id, artist, title, outdir, filename, status, priority, attempts,
                   next_attempt_at, last_error, created_at, updated_at, completed_at, output_paths`,
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

func (q *DBQueue) backoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	if q.baseBackoff <= 0 || q.maxBackoff <= 0 {
		return 0
	}
	delay := q.baseBackoff
	for i := 1; i < attempts; i++ {
		if delay >= q.maxBackoff || delay > q.maxBackoff/2 {
			return q.maxBackoff
		}
		delay *= 2
	}
	if delay > q.maxBackoff {
		return q.maxBackoff
	}
	return delay
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanWorkItem(row rowScanner) (WorkItem, error) {
	var item WorkItem
	var nextAttemptAt, createdAt, updatedAt, outputPaths string
	var completedAt sql.NullString
	err := row.Scan(
		&item.ID,
		&item.Inputs.Track.ArtistName,
		&item.Inputs.Track.TrackName,
		&item.Inputs.Outdir,
		&item.Inputs.Filename,
		&item.Status,
		&item.Priority,
		&item.Attempts,
		&nextAttemptAt,
		&item.LastError,
		&createdAt,
		&updatedAt,
		&completedAt,
		&outputPaths,
	)
	if err != nil {
		return WorkItem{}, err
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
