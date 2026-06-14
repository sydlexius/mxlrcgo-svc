-- +goose Up
-- +goose StatementBegin
-- Encrypted-at-rest secret store. Each row holds one secret as an AES-256-GCM
-- blob: nonce(12) || ciphertext || tag(16). The encryption key is NOT stored
-- here (see docs/design/2026-06-13-223-secrets-encryption.md). General store:
-- `name` is a stable identifier; v1 uses 'musixmatch_token' and 'webhook_api_key'.
CREATE TABLE secrets (
    name       TEXT PRIMARY KEY,
    ciphertext BLOB NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS secrets;
-- +goose StatementEnd
