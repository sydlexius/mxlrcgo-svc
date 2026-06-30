-- +goose Up
-- +goose StatementBegin
-- Five nullable telemetry columns added to work_queue so each instrumental
-- detection decision is persisted alongside instrumental_result (issue #404).
-- NULL semantics: NULL means detection did not run for this row (disabled,
-- source_path absent, or a pre-telemetry row created before this migration).
-- When detection ran, all five columns are stamped atomically with
-- instrumental_result in a single UPDATE by SetInstrumentalResult.
--
-- music_sum    - summed instrumental-class MEAN probability (the music gate score).
-- vocal_peak   - peak (max-over-frames) of the winning vocal class (sung-vocal gate score).
-- speech_mean  - summed frame-MEAN of speech classes (speech gate score).
-- vocal_class  - name of the vocal class that produced vocal_peak; empty when no
--                vocal class scored or when the sidecar returned no max map.
-- detector_version - app version string at detection time (internal/version.Version),
--                    sourced from the running binary via Config.Version.
ALTER TABLE work_queue ADD COLUMN music_sum REAL;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE work_queue ADD COLUMN vocal_peak REAL;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE work_queue ADD COLUMN speech_mean REAL;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE work_queue ADD COLUMN vocal_class TEXT;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE work_queue ADD COLUMN detector_version TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE work_queue DROP COLUMN music_sum;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE work_queue DROP COLUMN vocal_peak;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE work_queue DROP COLUMN speech_mean;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE work_queue DROP COLUMN vocal_class;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE work_queue DROP COLUMN detector_version;
-- +goose StatementEnd
