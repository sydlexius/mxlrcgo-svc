-- +goose Up
-- +goose StatementBegin
-- Per-lane provider outcome counters for the /metrics endpoint. Each row tracks
-- cumulative hits (lyrics found) and misses (benign no-result) for one provider
-- lane. Rows are upserted atomically by the worker write-path; the table starts
-- empty and self-populates on first use.
CREATE TABLE provider_outcomes (
    lane   TEXT    PRIMARY KEY,
    hits   INTEGER NOT NULL DEFAULT 0,
    misses INTEGER NOT NULL DEFAULT 0
);
-- +goose StatementEnd
-- +goose StatementBegin
-- Audio-detection result for a work_queue row. NULL = detection was not run for
-- this item; 0 = detection ran but track is not instrumental; 1 = audio-detected
-- as instrumental. Distinct from detect_instrumental (the request flag stamped at
-- enqueue time, migration 016). The gauge mxlrcgo_instrumental_tracks counts rows
-- with instrumental_result = 1.
ALTER TABLE work_queue ADD COLUMN instrumental_result INTEGER;
-- +goose StatementEnd
-- +goose StatementBegin
-- Winning provider lane for a completed work_queue row. NULL means the row has
-- not yet been completed, was retired without a match, or predates this column.
-- Written by the worker at completion time so per-track provider provenance is
-- queryable for debugging (separate from the aggregate counters in provider_outcomes).
ALTER TABLE work_queue ADD COLUMN provider_lane TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE work_queue DROP COLUMN provider_lane;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE work_queue DROP COLUMN instrumental_result;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS provider_outcomes;
-- +goose StatementEnd
