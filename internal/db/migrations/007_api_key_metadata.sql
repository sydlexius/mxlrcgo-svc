-- +goose Up
-- +goose StatementBegin
CREATE TABLE api_key_metadata (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL DEFAULT '',
    hash       TEXT NOT NULL UNIQUE,
    scopes     TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    revoked_at DATETIME
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS api_key_metadata;
-- +goose StatementEnd
