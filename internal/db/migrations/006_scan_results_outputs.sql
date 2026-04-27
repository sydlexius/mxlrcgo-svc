-- +goose Up
-- +goose StatementBegin
ALTER TABLE scan_results ADD COLUMN outdir TEXT NOT NULL DEFAULT '';
ALTER TABLE scan_results ADD COLUMN filename TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE scan_results DROP COLUMN filename;
ALTER TABLE scan_results DROP COLUMN outdir;
-- +goose StatementEnd
