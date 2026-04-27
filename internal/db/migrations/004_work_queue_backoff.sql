-- +goose Up
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_work_queue_pending;

ALTER TABLE work_queue ADD COLUMN artist_key TEXT NOT NULL DEFAULT '';
ALTER TABLE work_queue ADD COLUMN title_key TEXT NOT NULL DEFAULT '';
ALTER TABLE work_queue ADD COLUMN filename TEXT NOT NULL DEFAULT '';
ALTER TABLE work_queue ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE work_queue ADD COLUMN next_attempt_at DATETIME NOT NULL DEFAULT '1970-01-01T00:00:00Z';
ALTER TABLE work_queue ADD COLUMN last_error TEXT NOT NULL DEFAULT '';
ALTER TABLE work_queue ADD COLUMN completed_at DATETIME;

UPDATE work_queue
SET artist_key = lower(trim(artist)),
    title_key = lower(trim(title)),
    next_attempt_at = CASE
        WHEN next_attempt_at = '' THEN strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
        ELSE next_attempt_at
    END
WHERE artist_key = '' OR title_key = '' OR next_attempt_at = '';

DELETE FROM work_queue
WHERE id IN (
    SELECT id
    FROM (
        SELECT
            id,
            ROW_NUMBER() OVER (
                PARTITION BY artist_key, title_key
                ORDER BY
                    CASE WHEN status IN ('pending', 'processing') THEN 0 ELSE 1 END,
                    id ASC
            ) AS rn
        FROM work_queue
    )
    WHERE rn > 1
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_work_queue_artist_title_key
    ON work_queue(artist_key, title_key);

CREATE INDEX IF NOT EXISTS idx_work_queue_dequeue
    ON work_queue(status, next_attempt_at, priority, created_at, id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_work_queue_dequeue;
DROP INDEX IF EXISTS idx_work_queue_artist_title_key;

CREATE UNIQUE INDEX IF NOT EXISTS idx_work_queue_pending
    ON work_queue(artist, title) WHERE status = 'pending';
-- +goose StatementEnd
