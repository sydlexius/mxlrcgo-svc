-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS libraries (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    path       TEXT    NOT NULL UNIQUE,
    name       TEXT    NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE TABLE IF NOT EXISTS scan_results (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    file_path  TEXT    NOT NULL,
    artist     TEXT    NOT NULL DEFAULT '',
    title      TEXT    NOT NULL DEFAULT '',
    status     TEXT    NOT NULL DEFAULT 'pending'
                       CHECK(status IN ('pending', 'processing', 'done', 'failed')),
    created_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE TABLE IF NOT EXISTS lyrics_cache (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    artist     TEXT    NOT NULL,
    title      TEXT    NOT NULL,
    lyrics     TEXT    NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    UNIQUE(artist, title)
);

CREATE TABLE IF NOT EXISTS work_queue (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    artist     TEXT    NOT NULL,
    title      TEXT    NOT NULL,
    outdir     TEXT    NOT NULL DEFAULT '',
    status     TEXT    NOT NULL DEFAULT 'pending'
                       CHECK(status IN ('pending', 'processing', 'done', 'failed')),
    priority   INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_work_queue_pending
    ON work_queue(artist, title) WHERE status = 'pending';

CREATE TABLE IF NOT EXISTS api_keys (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    service    TEXT    NOT NULL UNIQUE,
    token      TEXT    NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE TRIGGER IF NOT EXISTS update_libraries_updated_at
AFTER UPDATE ON libraries
BEGIN
    UPDATE libraries SET updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
    WHERE id = NEW.id;
END;

CREATE TRIGGER IF NOT EXISTS update_lyrics_cache_updated_at
AFTER UPDATE ON lyrics_cache
BEGIN
    UPDATE lyrics_cache SET updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
    WHERE id = NEW.id;
END;

CREATE TRIGGER IF NOT EXISTS update_work_queue_updated_at
AFTER UPDATE ON work_queue
BEGIN
    UPDATE work_queue SET updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
    WHERE id = NEW.id;
END;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS update_work_queue_updated_at;
DROP TRIGGER IF EXISTS update_lyrics_cache_updated_at;
DROP TRIGGER IF EXISTS update_libraries_updated_at;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS work_queue;
DROP TABLE IF EXISTS lyrics_cache;
DROP TABLE IF EXISTS scan_results;
DROP TABLE IF EXISTS libraries;
-- +goose StatementEnd
