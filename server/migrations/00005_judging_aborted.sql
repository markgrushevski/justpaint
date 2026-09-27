-- +goose Up

-- A third terminal resolution, 'aborted': judging exhausted its retries with no
-- verdict, so the round closes as `done` with no winner and no rating change
-- instead of wedging in `judging` forever (docs/DECISIONS.md 2026-09-18). Postgres
-- can't widen a check constraint in place, so 00004's is dropped and re-added,
-- named explicitly so a mismatch fails loudly rather than leaving it silently
-- unwidened.
alter table matches
    drop constraint matches_resolution_check,
    add constraint matches_resolution_check
        check (resolution in ('judged', 'forfeit', 'aborted'));

-- Backfills the judging_started_at that 00004 left null: `null <= now() -
-- interval` is null, so an already-wedged row matched neither watchdog query and
-- was immortal. Stamping it now lets the watchdog re-fire it normally.
update matches set judging_started_at = now()
    where status = 'judging' and judging_started_at is null;

-- +goose Down

-- Rows already resolved 'aborted' would violate the narrowed constraint, so fold
-- them back to `abandoned`, the closest pre-00005 no-winner/no-Elo terminal —
-- losing the 'aborted' label, not the fact of ending.
update matches set status = 'abandoned', resolution = null, updated_at = now()
    where resolution = 'aborted';
alter table matches
    drop constraint matches_resolution_check,
    add constraint matches_resolution_check
        check (resolution in ('judged', 'forfeit'));
