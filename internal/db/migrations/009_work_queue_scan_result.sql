-- +goose Up
-- +goose StatementBegin
ALTER TABLE work_queue ADD COLUMN scan_result_id INTEGER REFERENCES scan_results(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_work_queue_scan_result
    ON work_queue(scan_result_id) WHERE scan_result_id IS NOT NULL;

-- Recovery: rows pinned to 'processing' before the worker writeback existed
-- would otherwise stay there forever. Reset them so the next scan re-enqueues.
UPDATE scan_results SET status = 'pending' WHERE status = 'processing';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_work_queue_scan_result;
ALTER TABLE work_queue DROP COLUMN scan_result_id;
-- +goose StatementEnd
