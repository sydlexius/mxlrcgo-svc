-- +goose Up
-- +goose StatementBegin
ALTER TABLE work_queue ADD COLUMN source_path TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE work_queue DROP COLUMN source_path;
-- +goose StatementEnd
