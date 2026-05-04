-- +goose Up
-- +goose StatementBegin
-- Junction table tracking every scan_results row that collapsed into a single
-- deduped work_queue row. The work_queue dedupe key is (artist_key, title_key);
-- multiple files with identical normalized metadata produce one queue row but
-- multiple scan_results rows, all of which must be written back to 'done' on
-- successful completion. The pre-existing scalar work_queue.scan_result_id
-- could only carry the first link, leaving the rest stuck in 'processing'.
CREATE TABLE IF NOT EXISTS work_queue_scan_results (
    work_queue_id  INTEGER NOT NULL REFERENCES work_queue(id) ON DELETE CASCADE,
    scan_result_id INTEGER NOT NULL REFERENCES scan_results(id) ON DELETE CASCADE,
    PRIMARY KEY (work_queue_id, scan_result_id)
);

CREATE INDEX IF NOT EXISTS idx_work_queue_scan_results_scan
    ON work_queue_scan_results(scan_result_id);

INSERT OR IGNORE INTO work_queue_scan_results (work_queue_id, scan_result_id)
SELECT id, scan_result_id FROM work_queue WHERE scan_result_id IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_work_queue_scan_results_scan;
DROP TABLE IF EXISTS work_queue_scan_results;
-- +goose StatementEnd
