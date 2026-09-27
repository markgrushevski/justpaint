-- +goose Up
-- The drawing a practice run scored. Null on runs recorded before it was kept.
alter table practice_runs add column document jsonb;

-- +goose Down
alter table practice_runs drop column document;
