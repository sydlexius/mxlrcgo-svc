-- +goose Up
-- +goose StatementBegin
-- Files that consistently fail audio metadata read (malformed tag frames, e.g.
-- "compression without data length indicator"). The scanner records a row here
-- on a metadata-read failure and skips re-reading the file on later scans while
-- mtime_unix and size_bytes are unchanged, so a permanently-malformed file is
-- not re-read (and re-warned about) on every scan. The row is keyed by absolute
-- file_path; an upsert overwrites it when the file changes and fails again. A
-- file that later reads cleanly simply stops matching (its mtime/size moved on),
-- so the stale row is inert and harmless. See issue #376.
CREATE TABLE scanner_metadata_failures (
    file_path  TEXT    PRIMARY KEY,
    mtime_unix INTEGER NOT NULL,
    size_bytes INTEGER NOT NULL,
    error_text TEXT    NOT NULL DEFAULT ''
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS scanner_metadata_failures;
-- +goose StatementEnd
